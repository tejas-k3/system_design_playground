package engine

import (
	"math"
	"testing"
)

// Checkout is three steps in three services: reserve stock (a database), charge the card (an external API that
// sometimes declines) and book shipping (another external API). If a later step fails, what the earlier ones did
// either stays (naive: the order is half done) or is undone by compensating actions (a saga).
func workflowRun(mode string, errPct float64) (*Sim, RunStats) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 0.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 4000.0, "retries": 0.0})
	ap := Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 100.0}
	if mode != "" {
		ap["workflow"] = mode
	}
	b.node("a", KAppServer, ap)
	b.node("stock", KDatabase, Params{"connections": 32.0, "writeMs": 10.0})
	b.node("pay", KExternalAPI, Params{"latencyMs": 40.0, "errorPct": errPct})
	b.node("ship", KExternalAPI, Params{"latencyMs": 30.0, "errorPct": 0.0})
	b.edge("u", "a").edge("a", "stock").edge("a", "pay").edge("a", "ship")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 22})
	s.Advance(40000)
	return s, s.RunSnapshot()
}

func TestWithoutCompensationAFailedStepLeavesHalfAnOrder(t *testing.T) {
	_, naive := workflowRun("naive", 20)
	rate := float64(naive.Inconsistent) / float64(naive.Resolved)
	t.Logf("naive: %.1f%% of orders left half done (card declines 20%% of the time)", rate*100)
	if math.Abs(rate-0.2) > 0.04 {
		t.Errorf("each declined card leaves the reserved stock behind: expected about 20%%, got %.3f", rate)
	}
}

func TestASagaUndoesWhatEarlierStepsDid(t *testing.T) {
	_, saga := workflowRun("saga", 20)
	_, naive := workflowRun("naive", 20)
	t.Logf("saga: %d inconsistent, %d compensated, %d stuck; failed orders take %.0f ms (naive %.0f)", saga.Inconsistent, saga.Compensated, saga.Stuck, saga.FailP50, naive.FailP50)
	if saga.Inconsistent != 0 || saga.Stuck != 0 {
		t.Errorf("compensations succeed here, so nothing is left half done: %d, %d", saga.Inconsistent, saga.Stuck)
	}
	if rate := float64(saga.Compensated) / float64(saga.Resolved); math.Abs(rate-0.2) > 0.04 {
		t.Errorf("about a fifth of the orders are rolled back: %.3f", rate)
	}
	if saga.FailP50 <= naive.FailP50+5 {
		t.Errorf("rolling back takes time: failed orders are slower under a saga: %.0f vs %.0f", saga.FailP50, naive.FailP50)
	}
}

func TestWhenTheUndoFailsToo(t *testing.T) {
	// the card service answers half of its calls with an error, and shipping always fails: an order gets through the
	// card step half the time, shipping fails, and the refund (the compensation) has the same even chance of failing
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 0.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 4000.0, "retries": 0.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 100.0, "workflow": "saga"})
	b.node("stock", KDatabase, Params{"connections": 32.0, "writeMs": 10.0})
	b.node("pay", KExternalAPI, Params{"latencyMs": 40.0, "errorPct": 50.0})
	b.node("ship", KExternalAPI, Params{"latencyMs": 30.0, "errorPct": 100.0})
	b.edge("u", "a").edge("a", "stock").edge("a", "pay").edge("a", "ship")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 22})
	s.Advance(40000)
	r := s.RunSnapshot()
	rate := float64(r.Stuck) / float64(r.Resolved)
	t.Logf("%d stuck (%.1f%% of orders, theory 25%%), %d compensated, %d inconsistent", r.Stuck, rate*100, r.Compensated, r.Inconsistent)
	if math.Abs(rate-0.25) > 0.04 {
		t.Errorf("half reach shipping and half of those cannot be refunded: expected about 25%%, got %.3f", rate)
	}
	if r.Inconsistent != r.Stuck {
		t.Errorf("a stuck order is a half-done order: %d vs %d", r.Inconsistent, r.Stuck)
	}
}

func TestWorkflowOffKeepsTheOldWritePath(t *testing.T) {
	_, a := workflowRun("", 20)
	_, b := workflowRun("none", 20)
	if a.OK != b.OK || a.Inconsistent != 0 {
		t.Errorf("none must equal the default: %d vs %d", a.OK, b.OK)
	}
}
