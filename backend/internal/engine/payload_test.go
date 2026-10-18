package engine

import (
	"reflect"
	"testing"
)

func uploadDesign(reqKB, limitKB float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 0.0, "reqKB": reqKB, "retries": 0.0}).node("a", KAppServer, nil).node("q", KQueue, Params{"maxMessageKB": limitKB})
	b.node("w", KWorker, Params{"concurrency": 8.0, "processMs": 20.0}).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "q").edge("q", "w").edge("w", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestQueueRefusesAMessageOverItsLimit(t *testing.T) {
	big := run(uploadDesign(500, 256), Options{Seed: 5}, 20).Snapshot().Global.Run
	if big.OK != 0 || big.Errors["messageTooLarge"] == 0 || big.Errors["messageTooLarge"] != big.Failed {
		t.Fatalf("every 500 KB write should be refused by a 256 KB queue as messageTooLarge: ok=%d failed=%d errors=%v", big.OK, big.Failed, big.Errors)
	}
	small := run(uploadDesign(100, 256), Options{Seed: 5}, 20).Snapshot().Global.Run
	if small.Failed != 0 || small.OK == 0 || small.Errors["messageTooLarge"] != 0 {
		t.Fatalf("a 100 KB message fits: %+v", small)
	}
	exact := run(uploadDesign(256, 256), Options{Seed: 5}, 20).Snapshot().Global.Run
	if exact.Errors["messageTooLarge"] != 0 {
		t.Fatalf("a message exactly at the limit is accepted: %+v", exact)
	}
	raised := run(uploadDesign(500, 1024), Options{Seed: 5}, 20).Snapshot().Global.Run
	if raised.Errors["messageTooLarge"] != 0 || raised.OK == 0 {
		t.Fatalf("raising the limit to 1 MB lets 500 KB through: %+v", raised)
	}
}

func TestAReadNeverTouchesTheQueueLimit(t *testing.T) {
	d := uploadDesign(500, 256)
	for i := range d.Nodes {
		if d.Nodes[i].ID == "u" {
			d.Nodes[i].Params["readPct"] = 100.0
		}
	}
	// reads fail for another reason (a queue only accepts writes) but never as messageTooLarge
	if r := run(d, Options{Seed: 5}, 10).Snapshot().Global.Run; r.Errors["messageTooLarge"] != 0 {
		t.Fatalf("reads are not messages: %+v", r.Errors)
	}
}

// Sizes default to 0, which means 'not modelled': the result must be bit-identical to a design that never mentions them.
func TestDefaultSizesChangeNothing(t *testing.T) {
	mk := func(explicit bool) RunStats {
		b := &builder{}
		p := Params{"rps": 80.0, "readPct": 50.0}
		if explicit {
			p["reqKB"], p["respKB"] = 0.0, 0.0
		}
		b.node("u", KClient, p).node("a", KAppServer, nil).node("q", KQueue, nil).node("w", KWorker, nil).node("db", KDatabase, nil)
		b.edge("u", "a").edge("a", "q").edge("q", "w").edge("w", "db").edge("a", "db")
		return run(b.d, Options{Seed: 11}, 25).RunSnapshot()
	}
	if a, b := mk(false), mk(true); !reflect.DeepEqual(a, b) {
		t.Fatalf("explicit zero sizes changed the run:\n%+v\n%+v", a, b)
	}
}

func TestPayloadSettingsAreInTheRegistry(t *testing.T) {
	has := func(kind, key string) *ParamSpec {
		for _, p := range Spec(kind).Params {
			if p.Key == key {
				p := p
				return &p
			}
		}
		return nil
	}
	for _, k := range []string{"reqKB", "respKB"} {
		if p := has(KClient, k); p == nil || p.Default != 0.0 || p.Min != 0 {
			t.Errorf("client %s: want default 0 (not modelled), got %+v", k, p)
		}
	}
	if p := has(KQueue, "maxMessageKB"); p == nil || p.Default != 256.0 || p.Min < 1 {
		t.Errorf("queue maxMessageKB: want default 256, got %+v", p)
	}
	found := false
	for _, r := range ReasonInfos() {
		found = found || r.ID == "messageTooLarge"
	}
	if !found {
		t.Error("the reason messageTooLarge must be listed for the UI")
	}
}

func TestLintFlagsAPayloadTheQueueCannotCarry(t *testing.T) {
	has := func(d Diagram) bool {
		for _, f := range Lint(d) {
			if f.Rule == "large-payload-into-queue" {
				return len(f.Nodes) > 0 && f.Fix != ""
			}
		}
		return false
	}
	if !has(uploadDesign(500, 256)) {
		t.Error("500 KB into a 256 KB queue should be flagged")
	}
	if has(uploadDesign(100, 256)) || has(uploadDesign(0, 256)) {
		t.Error("a payload that fits, or sizes not modelled, must not be flagged")
	}
}
