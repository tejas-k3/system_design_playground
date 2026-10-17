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
