package engine

import "testing"

// Every request carries a token. The app server can ask an auth service whether it is valid (a call each time, or once
// per user for a while when it remembers the answer) or verify the signature itself (no call, but a little CPU).
func authRun(app Params, authP Params, crashAuth bool) (*Sim, RunStats) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 200.0, "readPct": 100.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0})
	ap := Params{"threads": 64.0, "queueCap": 1000.0, "authMode": "service", "tokenUsers": 100.0}
	for k, v := range app {
		ap[k] = v
	}
	b.node("a", KAppServer, ap)
	b.node("db", KDatabase, Params{"connections": 64.0, "readMs": 2.0})
	b.node("auth", KAuth, authP)
	b.edge("u", "a").edge("a", "db").edge("a", "auth")
	d, _ := Sanitize(b.d)
	var ch []ChaosEvent
	if crashAuth {
		ch = []ChaosEvent{{Type: "crash", NodeID: "auth", AtSec: 8, DownSec: 6}}
	}
	s := New(d, Options{Seed: 5, Chaos: ch})
	s.Advance(20000)
	return s, s.RunSnapshot()
}

func TestAnAuthServiceIsAskedAboutEveryRequestUnlessTokensAreCached(t *testing.T) {
	_, plain := authRun(nil, Params{"checkMs": 20.0}, false)
	_, cached := authRun(Params{"tokenCacheSec": 60.0}, Params{"checkMs": 20.0}, false)
	t.Logf("no cache: %d calls, %d cached, p50 %.0f; 60 s cache: %d calls, %d cached, p50 %.0f", plain.AuthCalls, plain.AuthCached, plain.P50, cached.AuthCalls, cached.AuthCached, cached.P50)
	if plain.AuthCalls < 3500 || plain.AuthCached != 0 {
		t.Errorf("4,000 requests, each asks the service: %d calls, %d cached", plain.AuthCalls, plain.AuthCached)
	}
	if cached.AuthCalls > 150 || cached.AuthCached < 3500 {
		t.Errorf("100 users need one call each while their token is remembered: %d calls, %d cached", cached.AuthCalls, cached.AuthCached)
	}
	if cached.P50 >= plain.P50-10 {
		t.Errorf("a remembered token saves the round trip: %.0f against %.0f ms", cached.P50, plain.P50)
	}
}

func TestVerifyingTheSignatureLocallyNeedsNoCall(t *testing.T) {
	s, local := authRun(Params{"authMode": "localSignature", "verifyMs": 0.5}, Params{"checkMs": 20.0}, false)
	_, plain := authRun(Params{"authMode": "none"}, Params{"checkMs": 20.0}, false)
	if got := s.Snapshot().Nodes["auth"].Arr; got != 0 || local.AuthCalls != 0 {
		t.Errorf("the service is never asked: %v arrivals, %d calls", got, local.AuthCalls)
	}
	if d := local.Mean - plain.Mean; d < 0.3 || d > 1 {
		t.Errorf("the check costs about its CPU time: %.2f ms more", d)
	}
}

func TestWhenTheAuthServiceIsDownFailClosedRefusesAndFailOpenLetsRequestsThroughUnchecked(t *testing.T) {
	_, closed := authRun(nil, Params{"checkMs": 20.0}, true)
	_, open := authRun(Params{"authFailMode": "open"}, Params{"checkMs": 20.0}, true)
	t.Logf("closed: %d of %d ok, %d unchecked; open: %d of %d ok, %d unchecked", closed.OK, closed.Resolved, closed.Unchecked, open.OK, open.Resolved, open.Unchecked)
	if closed.Resolved-closed.OK < 800 || closed.Unchecked != 0 {
		t.Errorf("six seconds without the service fails every request: %d failed, %d unchecked", closed.Resolved-closed.OK, closed.Unchecked)
	}
	if open.Resolved-open.OK > 50 || open.Unchecked < 800 {
		t.Errorf("open lets them through and counts them: %d failed, %d unchecked", open.Resolved-open.OK, open.Unchecked)
	}
}

func TestASlowAuthServiceBecomesTheBottleneck(t *testing.T) {
	small := Params{"checkMs": 40.0, "connections": 2.0, "queueCap": 20.0}
	_, slow := authRun(nil, small, false)
	_, cached := authRun(Params{"tokenCacheSec": 60.0}, small, false)
	if slow.OK*10 > slow.Resolved*6 {
		t.Errorf("2 slots at 40 ms take 50 checks a second against 200 asked: %d of %d ok", slow.OK, slow.Resolved)
	}
	if cached.OK*100 < cached.Resolved*95 {
		t.Errorf("remembering tokens takes the load off it: %d of %d ok", cached.OK, cached.Resolved)
	}
}

// A TLS handshake costs time on a new connection; a resumed session costs a third of it.
func TestTLSHandshakesCostTimeAndResumptionCutsIt(t *testing.T) {
	p50 := func(p Params) float64 {
		p["authMode"] = "none"
		_, r := authRun(p, Params{}, false)
		return r.P50
	}
	base, full, resumed := p50(Params{}), p50(Params{"tlsMs": 30.0}), p50(Params{"tlsMs": 30.0, "tlsResumePct": 100.0})
	t.Logf("p50 without TLS %.1f, full handshakes %.1f, all resumed %.1f", base, full, resumed)
	if full-base < 25 || full-base > 35 {
		t.Errorf("every request opens a connection and pays the handshake: +%.1f ms", full-base)
	}
	if resumed-base < 8 || resumed-base > 12 {
		t.Errorf("a resumed session costs a third: +%.1f ms", resumed-base)
	}
}
