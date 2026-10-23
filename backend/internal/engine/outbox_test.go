package engine

import "testing"

// A service that saves an order to its database and tells everyone else by publishing an event to a queue. Two
// separate steps: if the queue is unreachable after the database has the order, the event is gone for good (a dual
// write). With an outbox the event is written with the order, atomically, and a relay publishes it until it is
// accepted.
func eventRun(mode string, chaos []ChaosEvent) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 0.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 3000.0})
	ap := Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 100.0}
	if mode != "" {
		ap["eventPublish"] = mode
	}
	b.node("a", KAppServer, ap)
	b.node("db", KDatabase, Params{"connections": 64.0})
	b.node("q", KQueue, Params{"capacity": 50000.0})
	b.node("w", KWorker, Params{"concurrency": 16.0})
	b.edge("u", "a").edge("a", "db").edge("a", "q").edge("q", "w").edge("w", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 21, Chaos: chaos})
	s.Advance(45000)
	return s
}

var queueOutage = []ChaosEvent{{Type: "crash", NodeID: "q", AtSec: 10, DownSec: 10}}

func TestADualWriteLosesTheEventsOfAQueueOutage(t *testing.T) {
	s := eventRun("dualWrite", queueOutage)
	r := s.RunSnapshot()
	committed := len(s.committed)
	t.Logf("dual write: %d orders saved, %d events lost", committed, r.EventsLost)
	// 10 s of 100 orders a second; the clients keep getting 'saved' while the queue is down
	if r.EventsLost < 700 || r.EventsLost > 1100 {
		t.Errorf("about ten seconds of orders lose their event, got %d", r.EventsLost)
	}
}

func TestAnOutboxLosesNothingAndCatchesUpAfterTheOutage(t *testing.T) {
	s := eventRun("outbox", queueOutage)
	r := s.RunSnapshot()
	st := s.Snapshot().Nodes["a"].Extra
	t.Logf("outbox: %d orders saved, %d events lost, %.0f still waiting in the outbox", len(s.committed), r.EventsLost, st["outboxBacklog"])
	if r.EventsLost != 0 {
		t.Errorf("an outbox cannot lose an event: %d", r.EventsLost)
	}
	if st["outboxBacklog"] != 0 {
		t.Errorf("the relay should have delivered everything by the end: %.0f waiting", st["outboxBacklog"])
	}
}

func TestWithoutAQueueOutageNeitherModeLosesAnything(t *testing.T) {
	for _, m := range []string{"dualWrite", "outbox"} {
		if r := eventRun(m, nil).RunSnapshot(); r.EventsLost != 0 {
			t.Errorf("%s with a healthy queue lost %d", m, r.EventsLost)
		}
	}
}

func TestEventPublishOffKeepsTheOldBehaviour(t *testing.T) {
	a := eventRun("", queueOutage).RunSnapshot()
	b := eventRun("none", queueOutage).RunSnapshot()
	if a.OK != b.OK || a.EventsLost != 0 {
		t.Errorf("none must equal the default: %d vs %d", a.OK, b.OK)
	}
}
