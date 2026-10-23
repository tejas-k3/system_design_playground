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
	t.Run("bot-flood: a higher shared limit trades one problem for another", func(t *testing.T) {
		sc := Get("bot-flood")
		high := withParam(sc.Diagram, "rl", "limitRps", 400.0)
		if res := engine.RunToEnd(high, sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("400 shared requests a second still rejects too many customers: %+v", res.Checks)
		}
	})
	t.Run("retry-storm: no retries also recovers, a single retry does not", func(t *testing.T) {
		sc := Get("retry-storm")
		none := withParam(sc.Diagram, "users", "retries", 0.0)
		if res := engine.RunToEnd(none, sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("no retries should recover: %+v", res.Checks)
		}
		one := withParam(sc.Diagram, "users", "retries", 1.0)
		if res := engine.RunToEnd(one, sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("one immediate retry is still a storm: %+v", res.Checks)
		}
	})
	t.Run("zone-outage: spreading only the servers is not enough", func(t *testing.T) {
		sc := Get("zone-outage")
		d := sc.Diagram
		for i := range d.Nodes {
			switch d.Nodes[i].ID {
			case "app2":
				d.Nodes[i].Params["zone"] = "az-b"
			case "app3":
				d.Nodes[i].Params["zone"] = "az-c"
			}
		}
		if res := engine.RunToEnd(d, sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("the balancer and database are still in the failed zone: %+v", res.Checks)
		}
	})
	t.Run("bad-deploy: a 10% canary passes but a 30% one is too much exposure", func(t *testing.T) {
		sc := Get("bad-deploy")
		small := withParam(sc.Diagram, "lb", "canaryPct", 10.0)
		if res := engine.RunToEnd(small, sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("a 10%% canary should pass: %+v", res.Checks)
		}
		big := withParam(sc.Diagram, "lb", "canaryPct", 30.0)
		if res := engine.RunToEnd(big, sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("30%% of the traffic on a broken version is too much exposure: %+v", res.Checks)
		}
	})
	t.Run("poison-pill: fewer attempts without a dead-letter queue still loses jobs", func(t *testing.T) {
		sc := Get("poison-pill")
		d := withParam(sc.Diagram, "q", "maxAttempts", 3.0)
		if res := engine.RunToEnd(d, sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("dropping the bad jobs silently must fail the 'jobs lost' goal: %+v", res.Checks)
		}
	})
	t.Run("consumer-lag: 8 partitions are still not enough", func(t *testing.T) {
		sc := Get("consumer-lag")
		if res := engine.RunToEnd(withParam(sc.Diagram, "log", "partitions", 8.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("8 partitions leave a backlog: %+v", res.Checks)
		}
	})
	t.Run("pick-the-store: wide-column also works, more database connections do not", func(t *testing.T) {
		sc := Get("pick-the-store")
		if res := engine.RunToEnd(withParam(sc.Diagram, "db", "model", "wideColumn"), sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("wide-column should pass: %+v", res.Checks)
		}
		if res := engine.RunToEnd(withParam(sc.Diagram, "db", "connections", 64.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("more connections alone must not pass (the app's pool is the limit): %+v", res.Checks)
		}
	})
	t.Run("slow-query: a bigger database does not fix a scan", func(t *testing.T) {
		sc := Get("slow-query")
		if res := engine.RunToEnd(withParam(sc.Diagram, "db", "connections", 64.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("more connections should not fix a scan: %+v", res.Checks)
		}
	})
	t.Run("nobody-on-call: snapshot isolation is not enough", func(t *testing.T) {
		sc := Get("nobody-on-call")
		res := engine.RunToEnd(withParam(sc.Diagram, "db", "isolation", "repeatableRead"), sc.Options(42), sc.Goals)
		if res.Pass || res.Run.WriteSkew == 0 {
			t.Fatalf("repeatable read still lets both doctors leave: %+v skew %d", res.Checks, res.Run.WriteSkew)
		}
	})
	t.Run("disk-writes: a faster disk also passes but earns fewer stars than dropping the waste", func(t *testing.T) {
		sc := Get("disk-writes")
		fast := withParam(sc.Diagram, "db", "diskIops", 400.0)
		res := engine.RunToEnd(fast, sc.Options(42), sc.Goals)
		if !res.Pass {
			t.Fatalf("400 IOPS should be enough: %+v", res.Checks)
		}
		if ref := engine.RunToEnd(sc.Solution, sc.Options(42), sc.Goals); res.Stars >= ref.Stars {
			t.Fatalf("removing the unused indexes should score at least as well as buying IOPS: %d vs %d stars", res.Stars, ref.Stars)
		}
	})
	t.Run("one-heavy-query: a few more threads are not enough, 64 are, and a retry does not help", func(t *testing.T) {
		sc := Get("one-heavy-query")
		if res := engine.RunToEnd(withParam(sc.Diagram, "app", "threads", 20.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("20 threads still leave the light requests queueing: %+v", res.Checks)
		}
		if res := engine.RunToEnd(withParam(sc.Diagram, "users", "retries", 2.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("retries add load to a saturated server: %+v", res.Checks)
		}
	})
	t.Run("reconnect-storm: retries and a 2 s spread do not pass, a 30 s spread does", func(t *testing.T) {
		sc := Get("reconnect-storm")
		if res := engine.RunToEnd(withParam(sc.Diagram, "users", "retries", 3.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("retries repeat the herd: %+v", res.Checks)
		}
		if res := engine.RunToEnd(withParam(sc.Diagram, "users", "reconnectJitterSec", 2.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("a 2 s spread is still a spike: %+v", res.Checks)
		}
	})
	t.Run("hot-cell: more servers and retries do not pass, spreading by request does", func(t *testing.T) {
		sc := Get("hot-cell")
		if res := engine.RunToEnd(withParam(sc.Diagram, "app", "threads", 64.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("more app threads do not unload the hot shard: %+v", res.Checks)
		}
		if res := engine.RunToEnd(withParam(sc.Diagram, "users", "retries", 2.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("retries add to the hot shard: %+v", res.Checks)
		}
	})
	t.Run("restore-drill: backups alone and a fast restore alone each miss one goal", func(t *testing.T) {
		sc := Get("restore-drill")
		if res := engine.RunToEnd(withParam(sc.Diagram, "db", "backupEverySec", 5.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("frequent backups do not shorten the restore: %+v", res.Checks)
		}
		if res := engine.RunToEnd(withParam(sc.Diagram, "db", "restoreMBps", 600.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("a fast restore without backups still loses everything: %+v", res.Checks)
		}
	})
	t.Run("lost-cart: a shorter delay still loses updates, merging and one leader do not", func(t *testing.T) {
		sc := Get("lost-cart")
		if res := engine.RunToEnd(withParam(sc.Diagram, "db", "syncLagMs", 50.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("50 ms still has conflicts: %+v", res.Checks)
		}
		if res := engine.RunToEnd(withParam(sc.Diagram, "db", "topology", "singleLeader"), sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("one leader has no conflicts: %+v", res.Checks)
		}
	})
	t.Run("quorum-overlap: only R + W > N passes", func(t *testing.T) {
		sc := Get("quorum-overlap")
		if res := engine.RunToEnd(withParam(sc.Diagram, "kv", "writeQuorum", 2.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("R=1 W=2 does not overlap: %+v", res.Checks)
		}
		if res := engine.RunToEnd(withParam(sc.Diagram, "kv", "writeQuorum", 3.0), sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("R=1 W=3 overlaps and should pass: %+v", res.Checks)
		}
	})
	t.Run("lease-expired: a 1 s lease passes until pauses get longer, a conditional write does not care", func(t *testing.T) {
		sc := Get("lease-expired")
		lease := sc.Diagram
		longer := sc.Diagram
		for _, id := range []string{"app1", "app2", "app3"} {
			lease = withParam(lease, id, "lockTtlMs", 1000.0)
			longer = withParam(withParam(longer, id, "lockTtlMs", 1000.0), id, "gcPauseMs", 3000.0)
		}
		if res := engine.RunToEnd(lease, sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("a 1 s lease outlasts 600 ms pauses: %+v", res.Checks)
		}
		if res := engine.RunToEnd(longer, sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("3 s pauses outlast a 1 s lease: %+v", res.Checks)
		}
		if res := engine.RunToEnd(withParam(longer, "db", "conditionalWrite", true), sc.Options(42), sc.Goals); res.Run.Conflicts != 0 {
			t.Fatalf("a conditional write has no double bookings whatever the pause: %d", res.Run.Conflicts)
		}
	})
	t.Run("add-a-shard: more app threads do not hide the move, a ring does", func(t *testing.T) {
		sc := Get("add-a-shard")
		if res := engine.RunToEnd(withParam(sc.Diagram, "app", "threads", 128.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("hash mod still slows four requests in five: %+v", res.Checks)
		}
	})
	t.Run("heavy-tail: more threads do not help, hedging does", func(t *testing.T) {
		sc := Get("heavy-tail")
		if res := engine.RunToEnd(withParam(sc.Diagram, "app", "threads", 96.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("threads do not shorten a service-time tail: %+v", res.Checks)
		}
		if res := engine.RunToEnd(withParam(sc.Diagram, "app", "hedgeReadsMs", 5.0), sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("hedging after 5 ms also passes (and doubles the load): %+v", res.Checks)
		}
	})
	t.Run("noisy-tenant: a higher shared limit does not protect the quiet tenants", func(t *testing.T) {
		sc := Get("noisy-tenant")
		if res := engine.RunToEnd(withParam(sc.Diagram, "rl", "limitRps", 200.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("a bigger shared limit still favours the loudest: %+v", res.Checks)
		}
	})
	t.Run("go-global: capacity in one region does not help far users", func(t *testing.T) {
		sc := Get("go-global")
		if res := engine.RunToEnd(withParam(sc.Diagram, "app-us", "threads", 96.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("more US capacity does not shorten the ocean: %+v", res.Checks)
		}
	})
	t.Run("gc-storm: retries and extra servers do not pass, a 400 ms pause does", func(t *testing.T) {
		sc := Get("gc-storm")
		retry := sc.Diagram
		for i := range retry.Nodes {
			if retry.Nodes[i].Kind == engine.KClient {
				retry.Nodes[i].Params["retries"] = 2.0
			}
		}
		if res := engine.RunToEnd(retry, sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("retries rescue the requests but wreck the p95: %+v", res.Checks)
		}
		short := sc.Diagram
		for i := range short.Nodes {
			if short.Nodes[i].Kind == engine.KAppServer {
				short.Nodes[i].Params["gcPauseMs"] = 400.0
			}
		}
		if res := engine.RunToEnd(short, sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("a 400 ms pause is below the clients' patience and should pass: %+v", res.Checks)
		}
	})
	t.Run("video-upload is not fixed by more threads, and 400 Mbit/s is still too little", func(t *testing.T) {
		sc := Get("video-upload")
		if res := engine.RunToEnd(withParam(sc.Diagram, "app", "threads", 64.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("more threads alone must not pass: %+v", res.Checks)
		}
		d := sc.Diagram
		for k := range d.Edges {
			if d.Edges[k].ID == "app>files" {
				d.Edges[k].BandwidthMbps = 400
			}
		}
		if res := engine.RunToEnd(d, sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("400 Mbit/s cannot carry the 480 Mbit/s peak: %+v", res.Checks)
		}
	})
	t.Run("image-egress: a CDN that holds a quarter of the pictures is not enough", func(t *testing.T) {
		sc := Get("image-egress")
		d := withParam(sc.Solution, "cdn", "capacityItems", 500.0)
		if res := engine.RunToEnd(d, sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("a small CDN should still miss too often to meet the budget: %+v", res.Checks)
		}
	})
	t.Run("celebrity-post is solved by pulling, not by a bigger database connection pool", func(t *testing.T) {
		sc := Get("celebrity-post")
		if res := engine.RunToEnd(withParam(sc.Diagram, "app", "feedStrategy", "pullOnRead"), sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("pull on read should pass: %+v", res.Checks)
		}
		if res := engine.RunToEnd(withParam(sc.Diagram, "db", "connections", 64.0), sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("more connections alone must not pass: %+v", res.Checks)
		}
	})
	t.Run("search-tail is also solved by hedging, but not by more threads", func(t *testing.T) {
		sc := Get("search-tail")
		hedged := withParam(sc.Diagram, "app", "hedgeAfterMs", 100.0)
		if res := engine.RunToEnd(hedged, sc.Options(42), sc.Goals); !res.Pass {
			t.Fatalf("hedging should pass: %+v", res.Checks)
		}
		threads := withParam(sc.Diagram, "app", "threads", 64.0)
		if res := engine.RunToEnd(threads, sc.Options(42), sc.Goals); res.Pass {
			t.Fatalf("more threads alone must not pass (the wait is on the shards): %+v", res.Checks)
		}
	})
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
			// A fix-mode reference solution is an exemplar and must earn two stars. An open challenge's witness only
			// proves the requirements can be met (it is never shown), so it must pass: one star.
			need := 2
			if sc.Open {
				need = 1
			}
			for _, seed := range []uint32{1, 2, 3} {
				if s := engine.RunToEnd(sc.Solution, sc.Options(seed), sc.Goals); s.Stars < need {
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

// The open challenges must be a real puzzle: the blank canvas fails, a plausible-but-wrong design fails (too
// few servers; too many for the budget), and the witness passes, on several seeds.
func TestOpenChallengesAreWinnableButNotTrivial(t *testing.T) {
	n := 0
	for _, sc := range All() {
		if !sc.Open {
			continue
		}
		n++
		for _, seed := range []uint32{1, 2, 3} {
			if engine.RunToEnd(sc.Diagram, sc.Options(seed), sc.Goals).Pass {
				t.Errorf("%s seed %d: the blank canvas passes", sc.ID, seed)
			}
			if r := engine.RunToEnd(sc.Solution, sc.Options(seed), sc.Goals); !r.Pass {
				t.Errorf("%s seed %d: the witness fails: %+v", sc.ID, seed, r.Checks)
			}
		}
	}
	if n < 2 {
		t.Fatalf("%d open challenges, want at least 2", n)
	}
	// ten-thousand-students: three servers are too few, six blow the budget
	ts := Get("ten-thousand-students")
	for servers, wantPass := range map[int]bool{3: false, 4: true, 6: false} {
		d := openStudentsWith(ts, servers)
		if got := engine.RunToEnd(d, ts.Options(1), ts.Goals).Pass; got != wantPass {
			t.Errorf("ten-thousand-students with %d servers: pass=%v, want %v", servers, got, wantPass)
		}
	}
}

func openStudentsWith(sc *Scenario, servers int) engine.Diagram {
	b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 80.0}).
		n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).e("users", "lb").
		n("db", engine.KDatabase, cols[4], rowMid, P{"maxConnections": 500.0}).
		n("cache", engine.KCache, cols[3], rowTop, P{"ttlSec": 30.0, "ttlJitterPct": 20.0})
	return b.servers(servers, cols[2], rowTop, 70, P{"threads": 16.0, "callTimeoutMs": 400.0, "circuitBreaker": true, "dbPool": 20.0}, "cache").done()
}
