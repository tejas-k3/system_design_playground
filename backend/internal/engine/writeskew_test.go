package engine

import (
	"reflect"
	"testing"
)

// Two on-call doctors share a pair of rows. Each request reads both rows and, if the other is still on call, takes
// its own row off call (a write to its own row only). Run at once, both see the other on call and both leave: the
// rule "someone must stay" is broken although neither wrote the other's row.
func oncallDesign(isolation string) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 80.0, "workload": "oncall", "keySpace": 8.0, "keyDist": "uniform", "timeoutMs": 30000.0, "retries": 0.0}).
		node("a", KAppServer, Params{"threads": 256.0, "queueCap": 4000.0}).
		node("db", KDatabase, Params{"connections": 64.0, "isolation": isolation, "queueCap": 4000.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func oncall(iso string) RunStats { return run(oncallDesign(iso), Options{Seed: 3}, 30).RunSnapshot() }

func TestWriteSkewSlipsThroughReadCommittedAndSnapshotIsolation(t *testing.T) {
	rc, rr := oncall("readCommitted"), oncall("repeatableRead")
	t.Logf("write skew: read committed %d, repeatable read %d", rc.WriteSkew, rr.WriteSkew)
	if rc.WriteSkew < 20 || rr.WriteSkew < 20 {
		t.Fatalf("overlapping doctors on 4 pairs at 80 req/s should break the rule many times: %d and %d", rc.WriteSkew, rr.WriteSkew)
	}
	// snapshot isolation does abort two requests that write the SAME row (first committer wins) but not two that write
	// the two different rows of a pair: those are the skew
	if rr.TxAborts == 0 {
		t.Fatal("two doctors writing the same row still conflict under snapshot isolation")
	}
	if rc.TxAborts != 0 {
		t.Fatalf("read committed never aborts: %d", rc.TxAborts)
	}
}

func TestSerializableStopsWriteSkewAtTheCostOfWaiting(t *testing.T) {
	ser, rr := oncall("serializable"), oncall("repeatableRead")
	if ser.WriteSkew != 0 {
		t.Fatalf("locking both rows makes the doctors take turns: %d", ser.WriteSkew)
	}
	if ser.Mean <= rr.Mean {
		t.Fatalf("taking turns costs latency: serializable %.0f ms, repeatable read %.0f ms", ser.Mean, rr.Mean)
	}
}

func TestOnCallIsOffForOtherWorkloads(t *testing.T) {
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 60.0}).node("a", KAppServer, Params{"threads": 16.0}).node("db", KDatabase, Params{"isolation": "serializable"}).edge("u", "a").edge("a", "db")
	d, _ := Sanitize(plain.d)
	r := run(d, Options{Seed: 3}, 20).RunSnapshot()
	if r.WriteSkew != 0 {
		t.Fatalf("no skew without the oncall workload: %d", r.WriteSkew)
	}
	if !reflect.DeepEqual(r, run(d, Options{Seed: 3}, 20).RunSnapshot()) {
		t.Fatal("deterministic")
	}
}
