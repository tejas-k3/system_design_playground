package engine

import "testing"

// A feed that is ranked before it is shown: the app server scores every candidate post with a model, so a read costs
// time in proportion to the number of candidates; a write costs nothing extra.
func rankRun(app Params, readPct float64) RunStats {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 30.0, "readPct": readPct, "keySpace": 1000.0, "timeoutMs": 3000.0, "retries": 0.0})
	ap := Params{"threads": 8.0, "cpuMs": 10.0, "queueCap": 200.0}
	for k, v := range app {
		ap[k] = v
	}
	b.node("a", KAppServer, ap)
	b.node("db", KDatabase, Params{"connections": 32.0, "readMs": 2.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 6})
	s.Advance(30000)
	return s.RunSnapshot()
}

func TestRankingCostGrowsWithTheNumberOfCandidates(t *testing.T) {
	off := rankRun(nil, 100)
	few := rankRun(Params{"ranking": true, "rankCandidates": 100.0, "rankUsPerCandidate": 100.0}, 100)
	many := rankRun(Params{"ranking": true, "rankCandidates": 1000.0, "rankUsPerCandidate": 100.0}, 100)
	t.Logf("mean latency: no ranking %.1f ms, 100 candidates %.1f ms, 1,000 candidates %.1f ms", off.Mean, few.Mean, many.Mean)
	// 100 candidates at 100 microseconds is 10 ms of model time on top of the 10 ms of ordinary work
	if d := few.Mean - off.Mean; d < 8 || d > 14 {
		t.Errorf("100 candidates cost about 10 ms: +%.1f", d)
	}
	if d := many.Mean - off.Mean; d < 90 || d > 130 {
		t.Errorf("ten times the candidates cost ten times as much: +%.1f", d)
	}
}

func TestOnlyReadsPayForRanking(t *testing.T) {
	off := rankRun(nil, 0)
	on := rankRun(Params{"ranking": true, "rankCandidates": 1000.0, "rankUsPerCandidate": 100.0}, 0)
	if d := on.Mean - off.Mean; d > 1 {
		t.Errorf("a write is not ranked: +%.1f ms", d)
	}
}

func TestTooManyCandidatesSaturateTheServer(t *testing.T) {
	ok := rankRun(Params{"ranking": true, "rankCandidates": 200.0, "rankUsPerCandidate": 100.0}, 100)
	over := rankRun(Params{"ranking": true, "rankCandidates": 5000.0, "rankUsPerCandidate": 100.0}, 100)
	t.Logf("200 candidates: %d of %d ok; 5,000: %d of %d", ok.OK, ok.Resolved, over.OK, over.Resolved)
	if ok.OK < ok.Resolved*98/100 || over.OK > over.Resolved*70/100 {
		t.Errorf("30 reads a second at 30 ms each need one thread of 8; at 5,000 candidates, 510 ms each, they need 15 and the server cannot keep up: %d/%d against %d/%d", ok.OK, ok.Resolved, over.OK, over.Resolved)
	}
}
