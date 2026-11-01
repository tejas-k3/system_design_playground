package engine

import (
	"math"
	"testing"
)

func leakySim(limit, queue float64) (*Sim, *node) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 1.0}).node("rl", KRateLimiter, Params{"limitRps": limit, "burst": queue, "algorithm": "leakyBucket"}).node("a", KAppServer, nil).node("db", KDatabase, nil)
	b.edge("u", "rl").edge("rl", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 1})
	return s, s.nodes["rl"]
}

// A leaky bucket does not turn a burst away, it lets it out at the steady rate: 50 requests arriving together at
// 100 a second leave 10 ms apart.
func TestALeakyBucketSpacesABurstAtTheSteadyRate(t *testing.T) {
	s, rl := leakySim(100, 200)
	s.now = 0
	var last float64
	for i := range 50 {
		ok, d := s.leak(rl, &call{})
		if !ok {
			t.Fatalf("request %d of 50 should fit a queue of 200", i)
		}
		if i > 0 && math.Abs(d-last-10) > 1e-9 {
			t.Fatalf("requests leave 10 ms apart at 100 a second: %.2f after %.2f", d, last)
		}
		last = d
	}
	if math.Abs(last-490) > 1e-9 {
		t.Errorf("the 50th leaves 490 ms after the first: %.1f", last)
	}
}

func TestALeakyBucketRefusesWhatDoesNotFitTheQueue(t *testing.T) {
	s, rl := leakySim(100, 20)
	s.now = 0
	got := 0
	for range 50 {
		if ok, _ := s.leak(rl, &call{}); ok {
			got++
		}
	}
	if got != 21 { // the first goes straight through, 20 more wait
		t.Errorf("one through plus a queue of 20: %d", got)
	}
	// after the queue has drained there is room again
	s.now = 1000
	if ok, d := s.leak(rl, &call{}); !ok || d != 0 {
		t.Errorf("an idle bucket passes the next request at once: %v %.1f", ok, d)
	}
}

// End to end: a tenfold burst through a token bucket reaches the servers as a burst; through a leaky bucket it
// reaches them at the steady rate, later.
func TestALeakyBucketSmoothsWhatTheServersSee(t *testing.T) {
	peak := func(algo string) float64 {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 100.0, "arrival": "bursty", "burstFactor": 5.0, "burstSec": 1.0, "timeoutMs": 5000.0, "readPct": 100.0})
		b.node("rl", KRateLimiter, Params{"limitRps": 100.0, "burst": 400.0, "algorithm": algo})
		b.node("a", KAppServer, Params{"threads": 128.0, "queueCap": 2000.0})
		b.node("db", KDatabase, Params{"connections": 64.0})
		b.edge("u", "rl").edge("rl", "a").edge("a", "db")
		d, _ := Sanitize(b.d)
		s := New(d, Options{Seed: 4})
		p := 0.0
		for range 40 {
			s.Advance(1000)
			if a := s.Snapshot().Nodes["a"].Arr; a > p {
				p = a
			}
		}
		return p
	}
	tok, leak := peak("tokenBucket"), peak("leakyBucket")
	t.Logf("busiest second at the app server: %.0f req/s through a token bucket, %.0f through a leaky bucket (limit 100)", tok, leak)
	if leak > 115 {
		t.Errorf("a leaky bucket releases at most the steady rate: %.0f", leak)
	}
	if tok < 1.5*leak {
		t.Errorf("a token bucket lets a burst through: %.0f vs %.0f", tok, leak)
	}
}

// Four tenants share a service; the first sends 400 requests a second, the others 50 each. A limiter that counts per
// tenant holds the noisy one to its allowance and does not touch the quiet ones; one shared limit turns everybody away.
func TestAPerTenantLimitContainsTheNoisyTenant(t *testing.T) {
	admitted := func(keyBy string) [4]int {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 1.0, "identities": 100.0, "tenants": 4.0})
		b.node("rl", KRateLimiter, Params{"limitRps": 100.0, "burst": 1.0, "keyBy": keyBy})
		b.node("a", KAppServer, nil).node("db", KDatabase, nil)
		b.edge("u", "rl").edge("rl", "a").edge("a", "db")
		d, _ := Sanitize(b.d)
		s := New(d, Options{Seed: 1})
		cl, rl := s.nodes["u"], s.nodes["rl"]
		var got [4]int
		for ms := 0.0; ms < 10000; ms += 10 { // every 10 ms
			s.now = ms
			send := func(tenant, count int) {
				for range count {
					user := cl.idBase + (tenant-1)*25 + 1 // an account of that tenant (blocks of 25 of the 100)
					lg := &logical{client: cl}
					if s.admit(rl, &call{user: user, lg: lg}) {
						got[tenant-1]++
					}
				}
			}
			send(1, 4) // 400 a second
			if int(ms/10)%2 == 0 {
				for tn := 2; tn <= 4; tn++ {
					send(tn, 1) // 50 a second each
				}
			}
		}
		return got
	}
	shared, per := admitted("none"), admitted("tenant")
	t.Logf("admitted in 10 s (tenant 1 is noisy): one shared limit %v, one limit per tenant %v", shared, per)
	for i := 1; i < 4; i++ {
		if per[i] < 490 {
			t.Errorf("a quiet tenant keeps all of its 500 requests under its own limit: tenant %d got %d", i+1, per[i])
		}
		if shared[i] > 400 {
			t.Errorf("under one shared limit the quiet tenants are turned away too: tenant %d got %d", i+1, shared[i])
		}
	}
	if per[0] < 900 || per[0] > 1100 {
		t.Errorf("the noisy tenant gets its allowance of about 1,000 in 10 s: %d", per[0])
	}
}

func tenantRun(keyBy string) RunStats {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 300.0, "readPct": 100.0, "identities": 100.0, "identitySkew": 1.2, "tenants": 4.0, "timeoutMs": 2000.0})
	b.node("rl", KRateLimiter, Params{"limitRps": 100.0, "burst": 20.0, "keyBy": keyBy})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0})
	b.node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "rl").edge("rl", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 6})
	s.Advance(30000)
	return s.RunSnapshot()
}

// The goal looks at the worst-served of the quiet tenants, not the average and not the noisy one itself.
func TestTheWorstTenantIsMeasuredAndTheLimitPerTenantProtectsTheQuietOnes(t *testing.T) {
	shared, per := tenantRun("none"), tenantRun("tenant")
	t.Logf("worst-served quiet tenant: %.2f with one shared limit, %.2f with a limit per tenant (all traffic: %d tenants)", shared.WorstTenantSuccess, per.WorstTenantSuccess, per.Tenants)
	if per.Tenants != 4 || shared.Tenants != 4 {
		t.Fatalf("four tenants should be tracked: %d, %d", shared.Tenants, per.Tenants)
	}
	if per.WorstTenantSuccess <= shared.WorstTenantSuccess+0.1 {
		t.Errorf("a limit per tenant should leave the worst-off tenant better served: %.2f vs %.2f", per.WorstTenantSuccess, shared.WorstTenantSuccess)
	}
}

func TestNoTenantsNothingMeasured(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "identities": 10.0}).node("a", KAppServer, nil).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 1})
	s.Advance(5000)
	if r := s.RunSnapshot(); r.Tenants != 0 {
		t.Errorf("tenants not modelled: %d", r.Tenants)
	}
}
