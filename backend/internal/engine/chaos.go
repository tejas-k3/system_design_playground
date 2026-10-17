package engine

import "math"

// Chaos: scheduled and manual crashes, slow-downs and network partitions.

// ChaosRecord is a chaos event that fired.
type ChaosRecord struct {
	T      float64 `json:"t"`
	Type   string  `json:"type"`
	NodeID string  `json:"nodeId"`
	Label  string  `json:"label"`
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
	t.slow = math.Max(1, ev.Factor)
	t.slowUntil = s.now + ev.DurationSec*1000
	until := t.slowUntil
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
