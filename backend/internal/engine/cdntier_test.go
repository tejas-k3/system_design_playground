package engine

import "testing"

// users -> edge CDN -> shield CDN -> app server -> database, with a slow origin so that concurrent misses overlap.
func tierDesign(edges int, coalesce bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 300.0, "readPct": 100.0, "keySpace": 20.0, "keyDist": "zipf", "timeoutMs": 8000.0})
	for i := range edges {
		b.node(edgeID(i), KCDN, Params{"capacityItems": 100.0, "ttlSec": 30.0, "startWarm": false})
	}
	b.node("shield", KCDN, Params{"capacityItems": 100.0, "ttlSec": 30.0, "startWarm": false, "coalesce": coalesce})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 1000.0}).node("db", KDatabase, Params{"connections": 16.0, "readMs": 200.0})
	for i := range edges {
		b.edge("u", edgeID(i)).edge(edgeID(i), "shield")
	}
	b.edge("shield", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func edgeID(i int) string { return "edge" + string(rune('A'+i)) }

func TestAShieldTierAnswersWhatTheEdgeMissed(t *testing.T) {
	s := New(tierDesign(1, false), Options{Seed: 5})
	s.Advance(2000) // cold start: the edge misses everything at first, so the shield is asked
	if arr := s.Snapshot().Nodes["shield"].Arr; arr < 1 {
		t.Fatalf("the edge's misses should reach the shield, it saw %.1f/s", arr)
	}
	s.Advance(58000)
	r := s.RunSnapshot()
	if r.OK < r.Resolved*95/100 {
		t.Fatalf("a two-tier chain should serve: %d ok of %d", r.OK, r.Resolved)
	}
	if hr := s.Snapshot().Nodes["edgeA"].HitRate; hr < 0.95 {
		t.Fatalf("once warm the edge answers nearly everything, hit rate %.2f", hr)
	}
}

// Several edge CDNs that all miss at once would each send the same request to the origin; a shield that collapses
// concurrent misses sends one.
func TestShieldCollapsingKeepsMissesOffTheOrigin(t *testing.T) {
	var without, with float64
	for _, c := range []bool{false, true} {
		s := New(tierDesign(4, c), Options{Seed: 5})
		total := 0.0
		for sec := 1; sec <= 30; sec++ {
			s.Advance(1000)
			total += s.Snapshot().Nodes["a"].Arr
		}
		if c {
			with = total
		} else {
			without = total
		}
	}
	t.Logf("origin requests in 30 s: %.0f without collapsing, %.0f with", without, with)
	if with > without*0.5 {
		t.Fatalf("collapsing should at least halve the origin traffic: %.0f vs %.0f", with, without)
	}
}
