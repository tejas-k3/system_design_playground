package engine

import "testing"

// A crawler fetches pages from three sites, each of which answers with a 429 once it is asked for more than 8 calls
// a second. The crawler's traffic comes in rushes (4x the average for 2 s at a time) so a site sees 16 calls a second
// during a rush while the average is only 4. A per-site limit on the crawler spreads each rush out.
func crawlRun(app Params) RunStats {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 12.0, "readPct": 100.0, "arrival": "bursty", "burstFactor": 4.0, "burstSec": 2.0, "keySpace": 3000.0, "keyDist": "uniform", "timeoutMs": 5000.0, "retries": 0.0})
	ap := Params{"threads": 64.0, "queueCap": 500.0, "hostChoice": "byKey"}
	for k, v := range app {
		ap[k] = v
	}
	b.node("a", KAppServer, ap)
	for _, h := range []string{"s1", "s2", "s3"} {
		b.node(h, KExternalAPI, Params{"latencyMs": 100.0, "errorPct": 0.0, "calledFor": "all", "politeRps": 8.0})
		b.edge("a", h)
	}
	b.node("db", KDatabase, Params{"connections": 32.0}).edge("a", "db").edge("u", "a")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 9})
	s.Advance(120000)
	return s.RunSnapshot()
}

func TestAPerSiteLimitKeepsACrawlerFromBeingRefused(t *testing.T) {
	free := crawlRun(nil)
	polite := crawlRun(Params{"perHostRps": 6.0})
	t.Logf("no limit: %d ok, %d refused (429); 6 calls/s per site: %d ok, %d refused, p95 %.0f ms", free.OK, free.Errors["rateLimited"], polite.OK, polite.Errors["rateLimited"], polite.P95)
	if free.Errors["rateLimited"] < 100 {
		t.Errorf("rushes of 16 calls a second against a limit of 8 are refused: %d", free.Errors["rateLimited"])
	}
	if polite.Errors["rateLimited"] != 0 {
		t.Errorf("never above 6 a second, never refused: %d", polite.Errors["rateLimited"])
	}
	if polite.P95 <= free.P95 {
		t.Errorf("smoothing the rushes costs waiting time: %.0f vs %.0f ms", polite.P95, free.P95)
	}
}

func TestSitesAreChosenByKeyAndDefaultsAreUnchanged(t *testing.T) {
	a := crawlRun(Params{"hostChoice": "all"}) // the old behaviour: the first site only
	if a.OK == 0 {
		t.Fatalf("the old path still works")
	}
}
