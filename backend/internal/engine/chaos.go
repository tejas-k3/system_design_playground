package engine

import "math"

// Chaos: scheduled and manual crashes, slow-downs and network partitions.

// ChaosRecord is a chaos event that fired.
type ChaosRecord struct {
	T      float64 `json:"t"`
	Type   string  `json:"type"`
	NodeID string  `json:"nodeId"`
	Label  string  `json:"label"`
	Lost   int     `json:"lost,omitempty"` // promotion: acknowledged writes that did not survive
}

func (s *Sim) fireChaos(ev ChaosEvent) {
	var t *node
	if len(ev.NodeID) > 7 && ev.NodeID[:7] == "random:" {
		kind := ev.NodeID[7:]
		var c []*node
		for _, n := range s.order {
			if n.kind == kind && !n.down && !n.removed {
				c = append(c, n)
			}
		}
		if len(c) == 0 {
			return
		}
		t = c[int(s.chaosRng.Next()*float64(len(c)))]
	} else {
		t = s.nodes[ev.NodeID]
	}
	if t == nil || t.removed {
		return
	}
	s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: ev.Type, NodeID: t.id, Label: t.label})
	switch ev.Type {
	case "crash":
		s.CrashNode(t.id, ev.DownSec)
		return
	case "partition":
		s.partition(t, ev.DurationSec)
		return
	}
	factor := math.Max(1, ev.Factor)
	t.slow = factor
	t.slowUntil = s.now + ev.DurationSec*1000
	until := t.slowUntil
	// intermittent: slow for PulseSec, normal for PulseSec, ... (the last step to normal is the end below)
	for at, on := s.now+ev.PulseSec*1000, false; ev.PulseSec > 0 && at < until; at, on = at+ev.PulseSec*1000, !on {
		slow := factor
		if !on {
			slow = 1
		}
		s.at(at, func() {
			if t.slowUntil == until && s.now < until {
				t.slow = slow
			}
		})
	}
	s.at(until, func() {
		if s.now >= t.slowUntil && t.slowUntil == until {
			t.slow = 1
		}
	})
}

// partition cuts a node off the network: calls to it get no answer (callers only escape through their own
// timeouts, unlike a crash that refuses at once) until it heals, when the waiting calls are delivered.
func (s *Sim) partition(n *node, durationSec float64) {
	n.cut = true
	s.after(durationSec*1000, func() {
		n.cut = false
		held := n.held
		n.held = nil
		for _, deliver := range held {
			deliver()
		}
	})
}

// scheduleFailover promotes a replica once the failure has been noticed and the replica has taken over, unless
// the primary came back first (a blip shorter than the detection time is not a failover).
func (s *Sim) scheduleFailover(db *node) {
	ep := db.epoch
	s.after((db.f("detectSec")+db.f("promoteSec"))*1000, func() {
		if db.down && db.epoch == ep {
			s.promote(db)
		}
	})
}

// promote makes the first healthy replica the primary. Writes the old primary acknowledged less than the replica's
// replication lag before it crashed never reached the replica, so they are gone: that is the price of asynchronous replication.
func (s *Sim) promote(db *node) {
	var r *node
	for _, n := range s.order {
		if n.kind == KReplica && !n.removed && !n.down && !n.promoted {
			r = n
			break
		}
	}
	if r == nil {
		return // nothing to promote: the database stays down
	}
	lost := s.truncateVersions(db.downAt - r.f("replicationLagMs"))
	s.runLost += lost
	r.promoted = true
	db.successor = r
	s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: "promotion", NodeID: r.id, Label: r.label, Lost: lost})
}

// CrashNode takes a node down; if downSec > 0 it restarts automatically.
func (s *Sim) CrashNode(id string, downSec float64) {
	n := s.nodes[id]
	if n == nil || n.down {
		return
	}
	n.down, n.downAt = true, s.now
	n.epoch++
	s.failQueued(n, RNodeDown)
	s.closePools(n)
	s.dropConns(n)
	if n.kind == KRedis {
		s.loseRedisData(n)
	}
	if n.kind == KDatabase && n.s("onFailure") == "promote" {
		s.scheduleFailover(n)
	}
	if downSec > 0 {
		ep := n.epoch
		s.after(downSec*1000, func() {
			if n.down && n.epoch == ep {
				s.RestartNode(n.id)
			}
		})
	}
}

// RestartNode brings a crashed node back.
func (s *Sim) RestartNode(id string) {
	n := s.nodes[id]
	if n == nil || !n.down {
		return
	}
	n.down = false
	n.slow = 1
	if n.kind == KClient {
		s.startClient(n)
	}
	s.startNext(n)
	if n.kind == KQueue {
		s.dispatch(n)
	}
}

// ChaosLog returns chaos events that have fired so far.
func (s *Sim) ChaosLog() []ChaosRecord { return s.chaosLog }
