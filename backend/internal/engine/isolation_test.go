package engine

import (
	"math"
	"reflect"
	"testing"
)

func transferDesign(isolation string, keySpace, rps float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "workload": "transfer", "keySpace": keySpace, "keyDist": "uniform", "timeoutMs": 30000.0, "retries": 0.0})
	b.node("a", KAppServer, Params{"threads": 256.0, "queueCap": 4000.0}).node("db", KDatabase, Params{"connections": 64.0, "isolation": isolation, "queueCap": 4000.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func runTransfers(isolation string, keySpace, rps float64, seed uint32) RunStats {
	return run(transferDesign(isolation, keySpace, rps), Options{Seed: seed}, 30).RunSnapshot()
}

func TestReadCommittedLosesUpdatesUnderContention(t *testing.T) {
	r := runTransfers("readCommitted", 4, 100, 3)
	if r.LostUpdates < 30 {
		t.Fatalf("read committed on 4 hot accounts at 100 transfers/s should lose many updates, lost %d", r.LostUpdates)
	}
	if r.TxAborts != 0 || r.Deadlocks != 0 {
		t.Fatalf("read committed neither aborts nor deadlocks: %+v", r)
	}
	if r.OK < 2500 {
		t.Fatalf("it also keeps every transfer succeeding: %d ok of ~3000", r.OK)
	}
}

func TestRepeatableReadTradesLostUpdatesForAborts(t *testing.T) {
	r := runTransfers("repeatableRead", 4, 100, 3)
	if r.LostUpdates != 0 {
		t.Fatalf("a snapshot write conflict is an abort, never a silent lost update: %d", r.LostUpdates)
	}
	// aborts are counted when they happen, errors when the request resolves: a few can still be in flight at the end
	if r.TxAborts < 30 || math.Abs(float64(r.Errors["txAborted"]-r.TxAborts)) > 5 {
		t.Fatalf("first committer wins: conflicting transactions abort (txAborts %d, errors %v)", r.TxAborts, r.Errors)
	}
}

func TestSerializableLocksAndDeadlocks(t *testing.T) {
	r := runTransfers("serializable", 4, 100, 3)
	if r.LostUpdates != 0 {
		t.Fatalf("serializable loses nothing: %d", r.LostUpdates)
	}
	if r.Deadlocks == 0 {
		t.Fatal("two transfers taking the same two accounts in opposite order deadlock: expected some on 4 hot accounts")
	}
	if r.TxAborts != r.Deadlocks {
		t.Fatalf("under locks the only aborts are deadlock victims: aborts %d, deadlocks %d", r.TxAborts, r.Deadlocks)
	}
	rc := runTransfers("readCommitted", 4, 100, 3)
	if r.OK >= rc.OK {
		t.Fatalf("under contention throughput falls as isolation rises: serializable %d ok, read committed %d", r.OK, rc.OK)
	}
}

func TestWithoutContentionTheLevelsAgree(t *testing.T) {
	rc := runTransfers("readCommitted", 1000000, 100, 3)
	for _, lvl := range []string{"repeatableRead", "serializable"} {
		r := runTransfers(lvl, 1000000, 100, 3)
		if r.LostUpdates > 3 || r.TxAborts > 3 || r.Deadlocks != 0 {
			t.Errorf("%s with a million accounts has nothing to conflict on: %+v", lvl, r)
		}
		if math.Abs(float64(r.OK-rc.OK))/float64(rc.OK) > 0.05 {
			t.Errorf("%s throughput %d differs from read committed %d by more than 5%%", lvl, r.OK, rc.OK)
		}
	}
}

func TestIsolationOnlyMattersForTransfers(t *testing.T) {
	kv := func(level string) RunStats {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 80.0, "readPct": 50.0}).node("a", KAppServer, nil).node("db", KDatabase, Params{"isolation": level})
		b.edge("u", "a").edge("a", "db")
		d, _ := Sanitize(b.d)
		return run(d, Options{Seed: 3}, 25).RunSnapshot()
	}
	if a, b := kv("readCommitted"), kv("serializable"); !reflect.DeepEqual(a, b) {
		t.Fatal("a plain key-value workload must not care about the isolation setting")
	}
}

func TestTransfersAreDeterministic(t *testing.T) {
	a := runTransfers("serializable", 4, 100, 11)
	b := runTransfers("serializable", 4, 100, 11)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed, same deadlocks")
	}
}

func TestIsolationSettingsAreInTheRegistry(t *testing.T) {
	var iso, wl bool
	for _, p := range Spec(KDatabase).Params {
		if p.Key == "isolation" && p.Default == "readCommitted" && len(p.Options) == 3 {
			iso = true
		}
	}
	for _, p := range Spec(KClient).Params {
		if p.Key == "workload" {
			for _, o := range p.Options {
				wl = wl || o.Value == "transfer"
			}
		}
	}
	if !iso || !wl {
		t.Fatalf("database isolation (default readCommitted, 3 levels) and the transfer workload must be selectable: iso=%v wl=%v", iso, wl)
	}
}
