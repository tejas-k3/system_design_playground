package engine

import "testing"

func linkDownDesign(mode string) (Diagram, Options) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 15.0, "readPct": 0.0, "timeoutMs": 2000.0}).
		node("a", KAppServer, Params{"threads": 64.0, "callTimeoutMs": 500.0, "dbPool": 200.0}).
		node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d, Options{Seed: 6, Chaos: []ChaosEvent{{Type: "linkDown", NodeID: "a", ToID: "db", Mode: mode, AtSec: 10, DurationSec: 10}}}
}

func dbDone(s *Sim) int { return len(s.committed) } // distinct writes the database committed

func TestRepliesLostMeansWorkDoneAndCallerStillFails(t *testing.T) {
	d, o := linkDownDesign("replies")
	cut := run(d, o, 20)
	o.Chaos = nil
	base := run(d, o, 20)
	if dbDone(cut) < dbDone(base)*9/10 {
		t.Fatalf("the database still receives and commits writes when only replies are lost: %d against %d", dbDone(cut), dbDone(base))
	}
	var win, winBase float64
	for _, p := range cut.Snapshot().Series {
		if p.T > 11 && p.T < 19 {
			win += p.OkRps
		}
	}
	for _, p := range base.Snapshot().Series {
		if p.T > 11 && p.T < 19 {
			winBase += p.OkRps
		}
	}
	if win > winBase*0.1 {
		t.Fatalf("callers hear nothing back so the writes fail for them: %.0f against %.0f", win, winBase)
	}
}

func TestRequestsLostMeansTheDatabaseDoesNothing(t *testing.T) {
	d, o := linkDownDesign("requests")
	cut := run(d, o, 20)
	o.Chaos = nil
	base := run(d, o, 20)
	if dbDone(cut) > dbDone(base)*3/4 {
		t.Fatalf("with requests lost the database sees nothing for 10 of 20 s: %d against %d", dbDone(cut), dbDone(base))
	}
}

func TestLinkDownWithoutAnEventChangesNothingAndHeals(t *testing.T) {
	d, o := linkDownDesign("both")
	s := run(d, o, 40)
	var late float64
	var n int
	for _, p := range s.Snapshot().Series {
		if p.T > 30 {
			late += p.OkRps
			n++
		}
	}
	if late/float64(n) < 10 {
		t.Fatalf("after the link heals the service recovers, got %.0f rps", late/float64(n))
	}
}
