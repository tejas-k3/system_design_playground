package engine

import "testing"

// A cluster of five nodes holding three copies of each key, read and written by an app server. The client asks for
// the latest value (strong), so every read that returns an older version is counted as stale.
func quorumRun(kv Params, chaos []ChaosEvent) *Sim { return quorumRunWith(nil, kv, chaos) }

func quorumRunWith(client, kv Params, chaos []ChaosEvent) *Sim {
	b := &builder{}
	cp := Params{"rps": 300.0, "readPct": 50.0, "keySpace": 20.0, "keyDist": "uniform", "consistency": "strong", "timeoutMs": 3000.0}
	for k, v := range client {
		cp[k] = v
	}
	b.node("u", KClient, cp)
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 200.0})
	p := Params{"nodes": 5.0, "replicas": 3.0, "readMs": 4.0, "writeMs": 100.0, "variability": "exponential", "concurrency": 32.0}
	for k, v := range kv {
		p[k] = v
	}
	b.node("kv", KKVStore, p)
	b.edge("u", "a").edge("a", "kv")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 11, Chaos: chaos})
	s.Advance(30000)
	return s
}

func stalePct(s *Sim) float64 {
	r := s.RunSnapshot()
	if r.Fresh == 0 {
		return -1
	}
	return float64(r.Stale) / float64(r.Fresh)
}

func TestStaleReadsHappenExactlyWhenTheQuorumsDoNotOverlap(t *testing.T) {
	cases := []struct {
		r, w  float64
		stale bool
	}{{1, 1, true}, {2, 1, true}, {1, 2, true}, {2, 2, false}, {1, 3, false}, {3, 1, false}}
	for _, c := range cases {
		s := quorumRun(Params{"readQuorum": c.r, "writeQuorum": c.w}, nil)
		p := stalePct(s)
		t.Logf("N=3 R=%.0f W=%.0f: %.2f%% of reads stale", c.r, c.w, p*100)
		if p < 0 {
			t.Fatalf("no fresh reads counted")
		}
		if c.stale && p < 0.01 {
			t.Errorf("R=%.0f W=%.0f cannot overlap (R+W <= N) and should read stale data, got %.3f", c.r, c.w, p)
		}
		if !c.stale && p != 0 {
			t.Errorf("R=%.0f W=%.0f overlap (R+W > N): reads must never be stale, got %.3f", c.r, c.w, p)
		}
	}
}

// With a quorum of one, the fastest copy answers; with all three it is the slowest. The latency is the kth fastest
// of the N draws.
func TestLatencyIsTheKthFastestOfTheCopies(t *testing.T) {
	median := func(r float64) float64 {
		s := quorumRunWith(Params{"readPct": 100.0}, Params{"readQuorum": r, "writeQuorum": 1.0, "readMs": 60.0}, nil)
		return s.RunSnapshot().P50
	}
	one, two, three := median(1), median(2), median(3)
	t.Logf("median latency by read quorum: R=1 %.1f ms, R=2 %.1f ms, R=3 %.1f ms", one, two, three)
	if !(one < two && two < three) {
		t.Errorf("a bigger read quorum waits for a slower copy: %.1f %.1f %.1f", one, two, three)
	}
}

func TestLosingTooManyNodesLosesTheQuorum(t *testing.T) {
	down := func(n float64) float64 {
		chaos := []ChaosEvent{{Type: "kvNodeDown", NodeID: "kv", AtSec: 5, DurationSec: 30, Factor: n}}
		s := quorumRun(Params{"readQuorum": 2.0, "writeQuorum": 2.0}, chaos)
		r := s.RunSnapshot()
		return float64(r.OK) / float64(r.Resolved+r.Pending)
	}
	none, one, three := down(0), down(1), down(3)
	t.Logf("success with 0, 1 and 3 of 5 nodes down (R=W=2, N=3): %.3f %.3f %.3f", none, one, three)
	if none < 0.99 {
		t.Errorf("healthy cluster should answer everything: %.3f", none)
	}
	if one < 0.99 {
		t.Errorf("one node down leaves every key with at least two copies: %.3f", one)
	}
	if three > 0.9 {
		t.Errorf("three of five nodes down leaves some keys with fewer than two copies: %.3f", three)
	}
}
