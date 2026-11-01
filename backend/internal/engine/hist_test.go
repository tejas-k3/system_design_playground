package engine

import "testing"

func TestLatencyBucketsAddUpAndContainTheQuantiles(t *testing.T) {
	s := run(stack(2, Params{"rps": 150.0, "readPct": 100.0}, Params{}, nil, nil).d, Options{Seed: 3}, 20)
	r := s.RunSnapshot()
	if len(r.LatencyBuckets) != len(latencyBounds)+1 {
		t.Fatalf("one bucket per bound plus an overflow bucket, got %d", len(r.LatencyBuckets))
	}
	total := 0
	for _, b := range r.LatencyBuckets {
		total += b.Count
	}
	if total != r.OK {
		t.Errorf("bucket counts add up to %d, successes are %d", total, r.OK)
	}
	in := func(v float64) int {
		lo := 0.0
		for i, b := range r.LatencyBuckets {
			if v > lo && v <= b.LeMs || (i == 0 && v <= b.LeMs) {
				return i
			}
			lo = b.LeMs
		}
		return len(r.LatencyBuckets) - 1
	}
	cum := func(upto int) int {
		c := 0
		for i := 0; i <= upto; i++ {
			c += r.LatencyBuckets[i].Count
		}
		return c
	}
	for _, q := range []struct {
		name string
		v, p float64
	}{{"p50", r.P50, 0.5}, {"p95", r.P95, 0.95}, {"p99", r.P99, 0.99}} {
		i := in(q.v)
		lower := 0
		if i > 0 {
			lower = cum(i - 1)
		}
		if float64(lower) > q.p*float64(r.OK)+1 || float64(cum(i)) < q.p*float64(r.OK)-1 {
			t.Errorf("%s = %.0f ms lies in bucket %d but the cumulative count there is %d..%d of %d", q.name, q.v, i, lower, cum(i), r.OK)
		}
	}
}

func TestLatencyBucketBoundsAreFixedAndIncreasing(t *testing.T) {
	for i := 1; i < len(latencyBounds); i++ {
		if latencyBounds[i] <= latencyBounds[i-1] {
			t.Fatalf("bounds must increase: %v", latencyBounds)
		}
	}
}
