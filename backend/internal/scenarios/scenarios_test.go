package scenarios

import (
	"fmt"
	"os"
	"testing"

	"sysdesign/internal/engine"
)

func TestScenarioStartFailsAndSolutionPasses(t *testing.T) {
	verbose := os.Getenv("VERBOSE") != ""
	for _, sc := range All() {
		sc := sc
		t.Run(sc.ID, func(t *testing.T) {
			start := engine.RunToEnd(sc.Diagram, sc.Options(42), sc.Goals)
			sol := engine.RunToEnd(sc.Solution, sc.Options(42), sc.Goals)
			if verbose {
				fmt.Printf("%-18s start pass=%v stars=%d | solution pass=%v stars=%d\n", sc.ID, start.Pass, start.Stars, sol.Pass, sol.Stars)
				for _, c := range start.Checks {
					fmt.Printf("    start  %-22s %-14s %-14s %v\n", c.Label, c.Target, c.Actual, c.Pass)
				}
				for _, c := range sol.Checks {
					fmt.Printf("    solved %-22s %-14s %-14s %v\n", c.Label, c.Target, c.Actual, c.Pass)
				}
			}
			if start.Pass {
				t.Errorf("starting design already passes")
			}
			if !sol.Pass {
				t.Errorf("reference solution fails: %+v", sol.Checks)
			}
		})
	}
}

func TestScenarioDiagramsAreValid(t *testing.T) {
	for _, sc := range All() {
		for name, d := range map[string]engine.Diagram{"start": sc.Diagram, "solution": sc.Solution} {
			_, adjusted := engine.Sanitize(d)
			if adjusted {
				t.Errorf("%s %s needed sanitizing", sc.ID, name)
			}
		}
	}
}

// Players can solve challenges in ways other than the reference solution. These pin that the
// mechanics (not just the authored answer) behave correctly.

func withParam(d engine.Diagram, id, key string, v any) engine.Diagram {
	out := engine.Diagram{Nodes: append([]engine.Node(nil), d.Nodes...), Edges: append([]engine.Edge(nil), d.Edges...), Notes: d.Notes}
	for i := range out.Nodes {
		if out.Nodes[i].ID == id {
			p := engine.Params{}
			for k, x := range out.Nodes[i].Params {
				p[k] = x
			}
			p[key] = v
			out.Nodes[i].Params = p
		}
	}
	return out
}

func TestAlternateSolutions(t *testing.T) {
	t.Run("double-booked with a redis lock that waits", func(t *testing.T) {
		sc := Get("double-booked")
		d := withParam(sc.Diagram, "app", "lockWaitMs", 500.0)
		d.Nodes = append(d.Nodes, engine.Node{ID: "r", Kind: engine.KRedis, Label: "Redis", Params: engine.Params{}})
		d.Edges = append(d.Edges, engine.Edge{ID: "app>r", From: "app", To: "r", LatencyMs: 5})
		sc2 := engine.RunToEnd(d, sc.Options(42), sc.Goals)
		if !sc2.Pass {
			t.Fatalf("redis lock solution failed: %+v", sc2.Checks)
		}
	})
	t.Run("double-booked with a fail-fast redis lock rejects honest bookings", func(t *testing.T) {
		sc := Get("double-booked")
		d := sc.Diagram
		d.Nodes = append(d.Nodes, engine.Node{ID: "r", Kind: engine.KRedis, Label: "Redis", Params: engine.Params{}})
		d.Edges = append(d.Edges, engine.Edge{ID: "app>r", From: "app", To: "r", LatencyMs: 5})
		res := engine.RunToEnd(d, sc.Options(42), sc.Goals)
		if res.Run.Conflicts != 0 {
			t.Fatalf("lock still double booked: %d", res.Run.Conflicts)
		}
		if res.Run.Errors["lockBusy"] == 0 {
			t.Fatal("expected some lockBusy rejections without a wait")
		}
	})
	t.Run("slow-reads with cache-aside instead of read-through", func(t *testing.T) {
		sc := Get("slow-reads")
		d := sc.Solution
		var edges []engine.Edge
		for _, e := range d.Edges {
			if e.ID != "cache>db" {
				edges = append(edges, e)
			}
		}
		d.Edges = edges
		res := engine.RunToEnd(d, sc.Options(42), sc.Goals)
		if !res.Pass {
			t.Fatalf("cache-aside failed: %+v", res.Checks)
		}
	})
	t.Run("server-down survives with a faster health check and 3 servers", func(t *testing.T) {
		sc := Get("server-down")
		d := withParam(sc.Solution, "lb", "healthCheckSec", 1.0)
		if res := engine.RunToEnd(d, sc.Options(7), sc.Goals); !res.Pass {
			t.Fatalf("failed: %+v", res.Checks)
		}
	})
	t.Run("one-slow-server is also fixed by ejecting the outlier", func(t *testing.T) {
		sc := Get("one-slow-server")
		d := withParam(sc.Diagram, "lb", "outlierEjection", true)
		if res := engine.RunToEnd(d, sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("outlier ejection failed: %+v", res.Checks)
		}
	})
	t.Run("split-the-database needs the data actually spread over the shards", func(t *testing.T) {
		sc := Get("split-the-database")
		d := sc.Solution
		var edges []engine.Edge
		for _, e := range d.Edges {
			if e.ID != "router>shard2" { // second shard exists but receives nothing
				edges = append(edges, e)
			}
		}
		d.Edges = edges
		if res := engine.RunToEnd(d, sc.Options(42), sc.Goals); res.Pass {
			t.Fatal("a router feeding a single shard should not beat the single database")
		}
	})
	t.Run("the capstone is not solved by capacity alone", func(t *testing.T) {
		sc := Get("launch-day-capstone")
		d := sc.Solution
		for i := 1; i <= 5; i++ {
			id := fmt.Sprintf("app%d", i)
			d = withParam(d, id, "callTimeoutMs", 0.0)
			d = withParam(d, id, "queueTimeoutMs", 0.0)
			d = withParam(d, id, "queueOrder", "fifo")
		}
		if res := engine.RunToEnd(d, sc.Options(42), sc.Goals); res.Pass {
			t.Fatal("without timeouts and a short queue the slow database should break the latency goal")
		}
	})
	t.Run("results are stable across seeds for the reference solutions", func(t *testing.T) {
		for _, sc := range All() {
			for _, seed := range []uint32{1, 2, 3} {
				if res := engine.RunToEnd(sc.Solution, sc.Options(seed), sc.Goals); !res.Pass {
					t.Errorf("%s seed %d fails: %+v", sc.ID, seed, res.Checks)
				}
			}
		}
	})
}

// A learner who reproduces the reference solution should be rewarded: at least two stars on every seed.
func TestReferenceSolutionsEarnTwoStars(t *testing.T) {
	for _, sc := range All() {
		sc := sc
		t.Run(sc.ID, func(t *testing.T) {
			for _, seed := range []uint32{1, 2, 3} {
				if s := engine.RunToEnd(sc.Solution, sc.Options(seed), sc.Goals); s.Stars < 2 {
					t.Errorf("seed %d: reference solution earns %d star(s), cost $%.2f/hr, p95 %.0f ms", seed, s.Stars, s.CostPerHour, s.Run.P95)
				}
			}
		})
	}
}

// The star targets are shown to the learner, so every scenario with a cost or p95 goal must publish them.
func TestStarTargetsArePublished(t *testing.T) {
	for _, sc := range All() {
		if sc.Goals.MaxCostPerHour != nil && sc.Goals.StarCostPerHour == nil {
			t.Errorf("%s: cost goal without a star target", sc.ID)
		}
		if sc.Goals.MaxP95Ms != nil && sc.Goals.StarP95Ms == nil {
			t.Errorf("%s: p95 goal without a star target", sc.ID)
		}
	}
}

// The star bars must not sit on a knife edge: a learner who rebuilds the reference solution should get the
// same stars whichever seed the run happens to use.
func TestReferenceStarsDoNotDependOnTheSeed(t *testing.T) {
	for _, sc := range All() {
		sc := sc
		t.Run(sc.ID, func(t *testing.T) {
			first := -1
			for seed := uint32(1); seed <= 8; seed++ {
				s := engine.RunToEnd(sc.Solution, sc.Options(seed), sc.Goals)
				if first < 0 {
					first = s.Stars
				}
				if s.Stars != first {
					t.Fatalf("seed %d earns %d stars, seed 1 earned %d (p95 %.0f ms, cost $%.2f/hr)", seed, s.Stars, first, s.Run.P95, s.CostPerHour)
				}
			}
		})
	}
}
