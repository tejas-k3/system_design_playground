package engine

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// TestPerfGuard fails if the large-design benchmark regresses. It is skipped unless PERF_GUARD=1 (CI sets it)
// so an ordinary `go test` stays fast and free of timing noise.
//
// Baseline (Apple M-series laptop, 30 simulated seconds, 1,000 rps, 20 nodes): 65 ms, 90.7 MB and 1.56 M
// allocations per run. Allocation counts and bytes do not depend on the machine, so their ceilings are tight
// (about +20-25 %). Time does, so its ceiling is generous: 300 ms, raised from 200 ms at the owner's decision after the shared CI runner went over by up to 6 ms six times with unchanged allocations (the local benchmark is about 120 ms); set PERF_NS_CEILING_MS to tighten it on a
// machine you have a baseline for (the ticket's rule of thumb is 1.4x).
func TestPerfGuard(t *testing.T) {
	if os.Getenv("PERF_GUARD") != "1" {
		t.Skip("set PERF_GUARD=1 to run the performance guard")
	}
	ceilingMs := 300.0
	if v, err := strconv.ParseFloat(os.Getenv("PERF_NS_CEILING_MS"), 64); err == nil && v > 0 {
		ceilingMs = v
	}
	const maxAllocs, maxBytes = 2_000_000, 125 << 20
	best := testing.Benchmark(BenchmarkLargeDesign)
	for i := 0; i < 2; i++ { // the allocation numbers are stable, the time is the noisy one: keep the fastest of three
		if r := testing.Benchmark(BenchmarkLargeDesign); r.NsPerOp() < best.NsPerOp() {
			best = r
		}
	}
	t.Logf("large design: %v per run, %.1f MB, %d allocs", time.Duration(best.NsPerOp()), float64(best.AllocedBytesPerOp())/(1<<20), best.AllocsPerOp())
	if a := best.AllocsPerOp(); a > maxAllocs {
		t.Errorf("allocations per run %d exceed %d", a, maxAllocs)
	}
	if b := best.AllocedBytesPerOp(); b > maxBytes {
		t.Errorf("bytes per run %d exceed %d", b, maxBytes)
	}
	if ms := float64(best.NsPerOp()) / 1e6; ms > ceilingMs {
		t.Errorf("run time %.0f ms exceeds %.0f ms", ms, ceilingMs)
	}
}
