package engine

import (
	"reflect"
	"testing"
)

// 1,000 customers at 0.1 requests a second each, and a scraper using just three accounts at 600 a second
func identityDesign(keyBy string, limit float64) Diagram {
	b := &builder{}
	b.node("users", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0, "identities": 1000.0}).
		node("bot", KClient, Params{"rps": 600.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0, "abuse": true, "identities": 3.0}).
		node("rl", KRateLimiter, Params{"limitRps": limit, "burst": 5.0, "keyBy": keyBy}).
		node("a", KAppServer, Params{"threads": 64.0, "queueCap": 500.0}).
		node("c", KCache, Params{"capacityItems": 1000.0, "ttlSec": 0.0, "startWarm": true}).
		node("db", KDatabase, Params{"connections": 16.0, "readMs": 20.0, "queueCap": 500.0})
	b.edge("users", "rl").edge("bot", "rl").edge("rl", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestPerUserLimitsStopTheScraperAndLeaveCustomersAlone(t *testing.T) {
	s := run(identityDesign("user", 5), Options{Seed: 5}, 40)
	through := s.Snapshot().Edges["rl>a"].Rps
	r := s.RunSnapshot()
	t.Logf("through the limiter %.0f/s of about 700 offered; rejected %d, ok %d", through, r.Errors["rateLimited"], r.OK)
	// three accounts at 5/s each is at most about 15/s of bot traffic plus the customers' 100/s
	if through < 95 || through > 135 {
		t.Fatalf("customers (100/s) plus three throttled accounts (about 15/s) should get through: %.0f/s", through)
	}
	if r.Errors["rateLimited"] < 15000 {
		t.Fatalf("most of the scraper's 600/s must be rejected: %d rejections", r.Errors["rateLimited"])
	}
	if r.P95 > 250 {
		t.Fatalf("the database is protected: p95 %.0f", r.P95)
	}
}

func TestAGlobalLimitCannotTellThemApart(t *testing.T) {
	s := run(identityDesign("none", 150), Options{Seed: 5}, 40)
	through := s.Snapshot().Edges["rl>a"].Rps
	// 150/s through out of 700 offered: customers are about 1/7 of what arrives, so they are turned away too
	if through > 175 {
		t.Fatalf("a 150/s global limit lets only about that through: %.0f", through)
	}
	r := s.RunSnapshot()
	custShare := 100.0 / 700.0
	if float64(r.Errors["rateLimited"]) < 20000*custShare*3 {
		t.Fatalf("customers should be among the rejected: %d rejections in all", r.Errors["rateLimited"])
	}
}

func TestIdentitiesAndKeyByOffChangeNothing(t *testing.T) {
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0}).
		node("rl", KRateLimiter, Params{"limitRps": 80.0, "burst": 5.0}).node("a", KAppServer, Params{"threads": 64.0}).node("db", KDatabase, nil)
	plain.edge("u", "rl").edge("rl", "a").edge("a", "db")
	d1, _ := Sanitize(plain.d)
	off := &builder{}
	off.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0, "identities": 0.0}).
		node("rl", KRateLimiter, Params{"limitRps": 80.0, "burst": 5.0, "keyBy": "none"}).node("a", KAppServer, Params{"threads": 64.0}).node("db", KDatabase, nil)
	off.edge("u", "rl").edge("rl", "a").edge("a", "db")
	d2, _ := Sanitize(off.d)
	if !reflect.DeepEqual(run(d1, Options{Seed: 5}, 20).RunSnapshot(), run(d2, Options{Seed: 5}, 20).RunSnapshot()) {
		t.Fatal("identities 0 and keyBy none must change nothing")
	}
}
