package engine

import "testing"

func fanoutDesign(kind string) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 40.0, "readPct": 0.0, "keySpace": 1000.0, "timeoutMs": 3000.0})
	b.node("a", KAppServer, Params{"threads": 32.0})
	b.node("q", kind, Params{"capacity": 50000.0})
	b.edge("u", "a").edge("a", "q")
	for _, w := range []string{"email", "push", "sms"} {
		b.node(w, KWorker, Params{"concurrency": 4.0, "processMs": 30.0})
		b.edge("q", w)
	}
	d, _ := Sanitize(b.d)
	return d
}

// A notification has to reach three channels. A queue hands each message to one worker; a topic gives every
// subscriber its own copy.
func TestDeliveriesPerMessageSeparatesAQueueFromATopic(t *testing.T) {
	min := 2.9
	g := Goals{MinSuccessRate: 0.9, MinDeliveriesPerMessage: &min}
	check := func(kind string) Check {
		d := fanoutDesign(kind)
		sc := Evaluate(g, d, run(d, Options{Seed: 5}, 40).RunSnapshot(), 1)
		for _, c := range sc.Checks {
			if c.Label == "Deliveries per message" {
				return c
			}
		}
		t.Fatal("no deliveries check in the score")
		return Check{}
	}
	q, tp := check(KQueue), check(KTopic)
	t.Logf("queue: %s (%v), topic: %s (%v)", q.Actual, q.Pass, tp.Actual, tp.Pass)
	if q.Pass {
		t.Errorf("a queue delivers each message once, not to three channels: %+v", q)
	}
	if !tp.Pass {
		t.Errorf("a topic delivers each message to all three subscribers: %+v", tp)
	}
}
