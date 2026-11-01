package engine

import (
	"math"
	"testing"
)

// Bloom filter: after n items in m bits with k hashes, the share of absent items it wrongly reports as present
// is (1 - e^(-kn/m))^k.
func TestBloomFalsePositiveRateMatchesTheFormula(t *testing.T) {
	for _, c := range []struct{ m, k, n int }{{100000, 7, 10000}, {50000, 4, 10000}, {20000, 3, 5000}} {
		b := NewBloom(c.m, c.k)
		for i := 0; i < c.n; i++ {
			b.Add(uint32(i))
		}
		for i := 0; i < c.n; i++ {
			if !b.Has(uint32(i)) {
				t.Fatalf("a Bloom filter never forgets what was added: %d", i)
			}
		}
		fp, trials := 0, 200000
		for i := range trials {
			if b.Has(uint32(1000000 + i)) {
				fp++
			}
		}
		got := float64(fp) / float64(trials)
		want := math.Pow(1-math.Exp(-float64(c.k*c.n)/float64(c.m)), float64(c.k))
		t.Logf("m=%d k=%d n=%d: measured %.5f, formula %.5f", c.m, c.k, c.n, got, want)
		if math.Abs(got-want)/want > 0.15 {
			t.Errorf("more than 15%% from the formula: %.5f vs %.5f", got, want)
		}
	}
}

// Count-min sketch: the estimate never undercounts, and overcounts by more than epsilon x N with probability at
// most delta, where width = e / epsilon and depth = ln(1 / delta).
func TestCountMinOvercountIsBoundedByEpsilonN(t *testing.T) {
	const w, d = 272, 5 // epsilon = e / 272 = 0.01, delta = e^-5 = 0.0067
	eps, delta := math.E/float64(w), math.Exp(-float64(d))
	over, total := 0, 0
	for trial := range 1000 {
		c := NewCountMin(w, d, uint32(trial))
		rng := NewRNG(uint32(trial) + 1)
		counts := map[uint32]int{}
		n := 0
		for range 2000 {
			x := uint32(rng.Next() * 500)
			c.Add(x, 1)
			counts[x]++
			n++
		}
		for _, x := range []uint32{1, 2, 3} {
			est := c.Estimate(x)
			if est < counts[x] {
				t.Fatalf("a count-min sketch never undercounts: %d < %d", est, counts[x])
			}
			total++
			if float64(est-counts[x]) > eps*float64(n) {
				over++
			}
		}
	}
	t.Logf("epsilon %.4f, delta %.4f: %d of %d estimates over the bound (%.4f)", eps, delta, over, total, float64(over)/float64(total))
	if float64(over)/float64(total) > delta {
		t.Errorf("more estimates beyond epsilon x N than delta allows: %.4f > %.4f", float64(over)/float64(total), delta)
	}
}

// HyperLogLog with 2^p registers has a relative standard error of 1.04 / sqrt(2^p); every one of 20 cardinalities
// has to land within three standard errors.
func TestHyperLogLogErrorIsWithinThreeStandardErrors(t *testing.T) {
	const p = 12
	se := 1.04 / math.Sqrt(float64(int(1)<<p))
	rng := NewRNG(99)
	var sumSq float64
	for i := range 20 {
		n := 100 + int(rng.Next()*200000)
		h := NewHLL(p)
		base := uint32(i) * 1000003
		for j := range n {
			h.Add(base + uint32(j))
			h.Add(base + uint32(j)) // repeats do not count
		}
		rel := (h.Count() - float64(n)) / float64(n)
		sumSq += rel * rel
		if math.Abs(rel) > 3*se {
			t.Errorf("n=%d: estimate %.0f, error %.4f beyond 3 x %.4f", n, h.Count(), rel, se)
		}
	}
	t.Logf("standard error %.4f; rms error over 20 cardinalities %.4f", se, math.Sqrt(sumSq/20))
}

// At small precisions the error is large and has to match theory too: over 300 different sets of 4,577 ids, the rms
// relative error is within 25% of 1.04 / sqrt(2^p) and the estimates are not biased.
func TestHyperLogLogErrorMatchesTheoryAtSmallPrecisions(t *testing.T) {
	const n, trials = 4577, 300
	for _, p := range []uint{4, 6, 8} {
		se := 1.04 / math.Sqrt(float64(int(1)<<p))
		var sq, sum float64
		for i := range trials {
			h := NewHLL(p)
			base := uint32(i) * 7919
			for j := range n {
				h.Add(base + uint32(j)*3)
			}
			r := (h.Count() - n) / n
			sq += r * r
			sum += r
		}
		rms, bias := math.Sqrt(sq/trials), sum/trials
		t.Logf("p=%d: standard error %.3f, measured %.3f, bias %.3f", p, se, rms, bias)
		if math.Abs(rms-se)/se > 0.25 {
			t.Errorf("p=%d: rms error %.3f is not within 25%% of the theory %.3f", p, rms, se)
		}
		if math.Abs(bias) > se/3 {
			t.Errorf("p=%d: the estimates are biased by %.3f", p, bias)
		}
	}
}
