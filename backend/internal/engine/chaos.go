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
	s.logEvent("slow", t, 0)
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
			s.logEvent("slowEnd", t, 0)
		}
	})
}

// partition cuts a node off the network: calls to it get no answer (callers only escape through their own
// timeouts, unlike a crash that refuses at once) until it heals, when the waiting calls are delivered.
func (s *Sim) partition(n *node, durationSec float64) {
	n.cut = true
	s.logEvent("partition", n, 0)
	s.after(durationSec*1000, func() {
		n.cut = false
		s.logEvent("heal", n, 0)
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
	s.after(db.f("detectSec")*1000, func() {
		if db.down && db.epoch == ep {
			s.logEvent("failoverDetected", db, 0)
		}
	})
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
	lost := s.truncateVersions(db.downAt - s.replicaLag(r))
	s.runLost += lost
	r.promoted = true
	db.successor = r
	s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: "promotion", NodeID: r.id, Label: r.label, Lost: lost})
	s.logEvent("promotion", r, lost)
}

// CrashNode takes a node down; if downSec > 0 it restarts automatically.
func (s *Sim) CrashNode(id string, downSec float64) {
	n := s.nodes[id]
	if n == nil || n.down {
		return
	}
	n.down, n.downAt = true, s.now
	s.logEvent("crash", n, 0)
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
	s.logEvent("restart", n, 0)
	if n.kind == KDatabase && n.successor != nil && n.successor.promoted && !n.b("fencing") {
		s.rejoinUnfenced(n)
	}
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

// replicaLag is how far behind a replica effectively runs: none when it replicates synchronously.
func (s *Sim) replicaLag(r *node) float64 {
	if r.s("replication") == "sync" {
		return 0
	}
	return r.f("replicationLagMs")
}

// syncReplicaCost is the extra time a write to this primary spends waiting for its synchronous replicas
// (their write time plus a network round trip), and whether one of them is down, which blocks the write.
func (s *Sim) syncReplicaCost(db *node) (wait float64, down bool) {
	for _, r := range s.order {
		if r.kind != KReplica || r.removed || r.promoted || r.s("replication") != "sync" {
			continue
		}
		if r.down {
			return 0, true
		}
		wait = math.Max(wait, r.f("writeMs")+10)
	}
	return wait, false
}

// rejoinUnfenced is the split-brain ending: with no fencing the old primary comes back believing it is still the
// primary, clients return to it, and every write the promoted replica accepted in the meantime exists only there: lost.
func (s *Sim) rejoinUnfenced(db *node) {
	r := db.successor
	lost := r.sincePromotion
	s.runLost += lost
	r.promoted, r.sincePromotion = false, 0
	db.successor = nil
	s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: "splitbrain", NodeID: db.id, Label: db.label, Lost: lost})
	s.logEvent("splitbrain", db, lost)
}

// scheduleGC arranges the next stop-the-world pause of an app server: exponentially distributed gaps with a mean of
// gcEverySec, drawn from the chaos random stream (so servers with no GC setting draw nothing, and the request
// stream is untouched). While paused nothing on the node starts or finishes; queued work waits and the pause ends
// by starting it.
func (s *Sim) scheduleGC(n *node) {
	gap := -math.Log(1-s.chaosRng.Next()) * n.f("gcEverySec") * 1000
	s.after(gap, func() {
		if n.removed {
			return
		}
		pause := n.f("gcPauseMs")
		if pause <= 0 {
			return
		}
		n.pausedUntil = s.now + pause
		s.logEvent("gc", n, 0)
		s.after(pause, func() {
			if n.pausedUntil <= s.now {
				n.pausedUntil = 0
			}
			s.startNext(n)
		})
		s.scheduleGC(n)
	})
}
