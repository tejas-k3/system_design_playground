package engine

import "testing"

func movedShare(a, b *kvRing, keys int) float64 {
	moved := 0
	for k := range keys {
		if a.order(k, 1)[0] != b.order(k, 1)[0] {
			moved++
		}
	}
	return float64(moved) / float64(keys)
}

// Adding the eleventh node to ten: with hash mod n a key keeps its node only when hash mod 10 == hash mod 11, which
// is 1 key in 11, so 10/11 = 90.9% move. On a ring the new node takes only its own arcs: 1/11 = 9.1% of the keys.
// Removing one of ten gives 1/10 on the ring.
func TestRingPlacementMovesOnlyAboutOneNth(t *testing.T) {
	const keys = 20000
	r10, r11, r9 := newKVRing(10, 100), newKVRing(11, 100), newKVRing(9, 100)
	add, rem := movedShare(r10, r11, keys), movedShare(r10, r9, keys)
	t.Logf("ring, 100 vnodes: adding a node to 10 moved %.4f of the keys (formula 1/11 = %.4f); removing one moved %.4f (formula 0.1)", add, 1.0/11, rem)
	if d := add / (1.0 / 11); d < 0.8 || d > 1.2 {
		t.Errorf("adding a node should move 1/11 of the keys within 20%%: %.4f", add)
	}
	if d := rem / 0.1; d < 0.8 || d > 1.2 {
		t.Errorf("removing a node should move 1/10 of the keys within 20%%: %.4f", rem)
	}
	mod := 0
	for k := range keys {
		if kvModFirst(k, 10) != kvModFirst(k, 11) {
			mod++
		}
	}
	t.Logf("mod placement: %.4f moved (formula 10/11 = %.4f)", float64(mod)/keys, 10.0/11)
	if f := float64(mod) / keys; f < 0.85 {
		t.Errorf("mod placement should remap about 91%%: %.4f", f)
	}
}

// Every key has the requested number of distinct nodes, and no node is starved or flooded.
func TestRingPreferenceListsAreDistinctAndBalanced(t *testing.T) {
	r := newKVRing(10, 100)
	count := make([]int, 10)
	for k := range 20000 {
		o := r.order(k, 3)
		if len(o) != 3 || o[0] == o[1] || o[1] == o[2] || o[0] == o[2] {
			t.Fatalf("key %d: %v", k, o)
		}
		count[o[0]]++
	}
	lo, hi := count[0], count[0]
	for _, c := range count {
		lo, hi = min(lo, c), max(hi, c)
	}
	t.Logf("keys per node (fair share 2000): min %d, max %d", lo, hi)
	if lo < 1000 || hi > 3000 {
		t.Errorf("100 vnodes should keep every node within 50%% of a fair share: %d..%d", lo, hi)
	}
}

// The store still obeys the quorum rule with ring placement, and the setting is off by default.
func TestRingPlacementKeepsQuorumRuleAndDefaultIsMod(t *testing.T) {
	ring := quorumRun(Params{"placement": "ring", "readQuorum": 2.0, "writeQuorum": 2.0}, nil)
	weak := quorumRun(Params{"placement": "ring", "readQuorum": 1.0, "writeQuorum": 1.0}, nil)
	t.Logf("ring placement stale reads: R=W=2 %.4f, R=W=1 %.4f", stalePct(ring), stalePct(weak))
	if stalePct(ring) != 0 || stalePct(weak) < 0.01 {
		t.Errorf("R+W>N must be exact, R+W<=N must go stale: %.4f %.4f", stalePct(ring), stalePct(weak))
	}
	a := quorumRun(nil, nil).RunSnapshot()
	b := quorumRun(Params{"placement": "mod"}, nil).RunSnapshot()
	if a.OK != b.OK || a.P50 != b.P50 || a.Stale != b.Stale {
		t.Errorf("default must be mod placement: %+v vs %+v", a, b)
	}
}
