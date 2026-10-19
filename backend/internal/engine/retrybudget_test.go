package engine

import (
	"reflect"
	"testing"
)

func storm(retries, budgetPct float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 0.0, "timeoutMs": 3000.0, "retries": retries, "backoff": "none", "retryBudgetPct": budgetPct}).
		node("a", KAppServer, Params{"threads": 64.0, "queueCap": 500.0}).
		node("v", KExternalAPI, Params{"latencyMs": 40.0, "errorPct": 50.0}).
		node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "v").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

// vendorLoad is the mean number of calls the vendor receives per second over the last 40 seconds of a 60 s run.
func vendorLoad(d Diagram) (offered, arrivals float64) {
	s := New(d, Options{Seed: 4})
	s.Advance(20000)
	var sum float64
	for sec := 0; sec < 40; sec++ {
		s.Advance(1000)
		sum += s.Snapshot().Nodes["v"].Arr
	}
	return 50, sum / 40
}

// A flaky vendor (half the calls fail) and clients that retry three times: every failure becomes more load.
func TestRetriesMultiplyTheLoadOnAFailingDependency(t *testing.T) {
	_, none := vendorLoad(storm(0, 0))
	_, storming := vendorLoad(storm(3, 0))
	t.Logf("vendor arrivals per second: no retries %.1f, 3 retries %.1f", none, storming)
	// 1 + 0.5 + 0.25 + 0.125 = 1.875 attempts per request
	if r := storming / none; r < 1.6 || r > 2.1 {
		t.Fatalf("three retries against a 50%% failure rate should send about 1.9x the calls: %.2fx", r)
	}
}

func TestARetryBudgetCapsTheAmplification(t *testing.T) {
	_, none := vendorLoad(storm(0, 0))
	_, budget := vendorLoad(storm(3, 10))
	t.Logf("vendor arrivals per second with a 10%% retry budget: %.1f against %.1f for no retries", budget, none)
	if r := budget / none; r < 1.0 || r > 1.2 {
		t.Fatalf("a 10%% budget allows at most about 10%% extra calls: %.2fx", r)
	}
}

func TestRetryBudgetOffChangesNothing(t *testing.T) {
	if !reflect.DeepEqual(run(storm(3, 0), Options{Seed: 4}, 20).RunSnapshot(), run(stormWithoutBudgetKey(), Options{Seed: 4}, 20).RunSnapshot()) {
		t.Fatal("retryBudgetPct 0 must equal a design that never mentions it")
	}
}

func stormWithoutBudgetKey() Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 0.0, "timeoutMs": 3000.0, "retries": 3.0, "backoff": "none"}).
		node("a", KAppServer, Params{"threads": 64.0, "queueCap": 500.0}).
		node("v", KExternalAPI, Params{"latencyMs": 40.0, "errorPct": 50.0}).
		node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "v").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}
