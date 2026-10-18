package engine

import (
	"reflect"
	"testing"
)

func oomDesign(rps, limit float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 100.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 8.0, "cpuMs": 50.0, "queueCap": 1000.0, "memLimitMB": limit, "memBaseMB": 100.0, "memPerReqMB": 2.0}).
		node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func countEvents(s *Sim, typ string) int {
	n := 0
	for _, e := range s.Snapshot().Events {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func TestOverloadFillsMemoryAndTheProcessIsKilled(t *testing.T) {
	s := run(oomDesign(300, 300), Options{Seed: 3}, 60)
	if countEvents(s, "oom") == 0 {
		t.Fatal("a queue of hundreds of requests at 2 MB each must exceed a 300 MB limit")
	}
	if s.Snapshot().Global.Run.Errors["nodeDown"] == 0 {
		t.Fatal("requests in flight when the process is killed fail as node down")
	}
}

func TestLoadWithinCapacityNeverRunsOutOfMemory(t *testing.T) {
	s := run(oomDesign(100, 300), Options{Seed: 3}, 60)
	if got := countEvents(s, "oom"); got != 0 {
		t.Fatalf("a healthy server holds few requests, got %d kills", got)
	}
}

func TestMemoryLimitOffChangesNothing(t *testing.T) {
	a := run(oomDesign(300, 0), Options{Seed: 3}, 30).RunSnapshot()
	b := &builder{}
	b.node("u", KClient, Params{"rps": 300.0, "readPct": 100.0, "timeoutMs": 5000.0}).node("a", KAppServer, Params{"threads": 8.0, "cpuMs": 50.0, "queueCap": 1000.0}).node("db", KDatabase, nil).edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(a, run(d, Options{Seed: 3}, 30).RunSnapshot()) {
		t.Fatal("limit 0 means no memory model")
	}
}
