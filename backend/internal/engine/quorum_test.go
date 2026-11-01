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

// The k-th fastest of N exponential service times has mean m x (1/N + 1/(N-1) + ... + 1/(N-k+1)). With N = 3 and a
// mean of 60 ms that is 20 ms (k = 1), 50 ms (k = 2) and 110 ms (k = 3), so waiting for two copies instead of one adds
// 30 ms and waiting for all three adds 90 ms, whatever the fixed overhead of the path is.
func TestReadLatencyDifferencesFollowTheOrderStatistics(t *testing.T) {
	mean := func(r float64) float64 {
		s := quorumRunWith(Params{"readPct": 100.0, "rps": 100.0}, Params{"readQuorum": r, "writeQuorum": 1.0, "readMs": 60.0}, nil)
		return s.RunSnapshot().Mean
	}
	m1, m2, m3 := mean(1), mean(2), mean(3)
	t.Logf("mean latency: R=1 %.1f ms, R=2 %.1f ms (+%.1f, expected +30), R=3 %.1f ms (+%.1f, expected +90)", m1, m2, m2-m1, m3, m3-m1)
	if d := m2 - m1; d < 30*0.85 || d > 30*1.15 {
		t.Errorf("R=2 should cost 30 ms more than R=1, within 15%%: %.1f", d)
	}
	if d := m3 - m1; d < 90*0.85 || d > 90*1.15 {
		t.Errorf("R=3 should cost 90 ms more than R=1, within 15%%: %.1f", d)
	}
}

// Hinted handoff conserves writes: after the node is back and its hints are replayed, it holds, for every key it is a
// copy of, the newest version any copy has: nothing written during the outage is missing.
func TestHintedHandoffLeavesNoCopyBehind(t *testing.T) {
	_, _, s := staleAfterReturn(Params{"hintedHandoff": true})
	n := s.nodes["kv"]
	nodes, copies, _, _ := kvShape(n)
	missing, checked := 0, 0
	for k, have := range n.kvVer[0] { // node 0 is the one that went down
		first := int(Hash32(uint32(k)*2654435761+11) % uint32(nodes))
		newest := 0
		for j := range copies {
			newest = max(newest, n.kvVer[(first+j)%nodes][k])
		}
		checked++
		if have < newest {
			missing++
		}
	}
	t.Logf("%d keys on the returned node checked, %d older than the newest copy (writes since it came back may still be on their way)", checked, missing)
	if checked == 0 {
		t.Fatalf("the node should hold keys")
	}
	if frac := float64(missing) / float64(checked); frac > 0.15 {
		t.Errorf("hinted handoff should leave almost nothing behind, got %.2f", frac)
	}
}
