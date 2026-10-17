package engine

import (
	"encoding/json"
	"fmt"
	"testing"
)

type builder struct{ d Diagram }

func (b *builder) node(id, kind string, p Params) *builder {
	b.d.Nodes = append(b.d.Nodes, Node{ID: id, Kind: kind, Label: id, Params: p})
	return b
}

func (b *builder) edge(from, to string) *builder {
	var fk, tk string
	for _, n := range b.d.Nodes {
		if n.ID == from {
			fk = n.Kind
		}
		if n.ID == to {
			tk = n.Kind
		}
	}
	b.d.Edges = append(b.d.Edges, Edge{ID: from + ">" + to, From: from, To: to, LatencyMs: DefaultLatency(fk, tk)})
	return b
}

func run(d Diagram, opts Options, sec float64) *Sim {
	d, _ = Sanitize(d)
	s := New(d, opts)
	s.Advance(sec * 1000)
	return s
}

// basic web stack: users -> lb -> n apps -> (cache) -> db
func stack(apps int, client, app, cache, db Params) *builder {
	b := &builder{}
	b.node("u", KClient, client).node("lb", KLoadBalancer, nil)
	b.edge("u", "lb")
	b.node("db", KDatabase, db)
	if cache != nil {
		b.node("c", KCache, cache)
		b.edge("c", "db")
	}
	for i := 0; i < apps; i++ {
		id := fmt.Sprintf("a%d", i)
		b.node(id, KAppServer, app)
		b.edge("lb", id)
		if cache != nil {
			b.edge(id, "c")
		}
		b.edge(id, "db")
	}
	return b
}

func TestDefaultLatencyRules(t *testing.T) {
	for _, c := range []struct {
		from, to string
		want     float64
	}{
		{KClient, KCDN, 10}, {KClient, KAppServer, 40}, {KClient, KLoadBalancer, 40}, {KCDN, KAppServer, 40},
		{KAppServer, KExternalAPI, 20}, {KWorker, KExternalAPI, 20}, {KAppServer, KDatabase, 5}, {KLoadBalancer, KAppServer, 5},
	} {
		if got := DefaultLatency(c.from, c.to); got != c.want {
			t.Errorf("%s -> %s: got %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestEveryReasonHasAnIDAndLabel(t *testing.T) {
	infos := ReasonInfos()
	if len(infos) != int(NumReasons) {
		t.Fatalf("got %d reasons, want %d", len(infos), NumReasons)
	}
	seen := map[string]bool{}
	for i, r := range infos {
		if r.ID == "" || r.Label == "" || seen[r.ID] || r.ID != Reason(i).String() {
			t.Errorf("reason %d is malformed or duplicated: %+v", i, r)
		}
		seen[r.ID] = true
	}
}

func TestSanitizeRejectsBadEdgesAndClamps(t *testing.T) {
	d := Diagram{
		Nodes: []Node{
			{ID: "a", Kind: KAppServer, Params: Params{"threads": 9999.0, "bogus": 1.0, "queueOrder": "weird"}},
			{ID: "d", Kind: KDatabase},
			{ID: "x", Kind: "nope"},
			{ID: "a", Kind: KDatabase}, // duplicate id
		},
		Edges: []Edge{
			{ID: "e1", From: "a", To: "d", LatencyMs: 9999},
			{ID: "e2", From: "d", To: "a"},     // db cannot connect to app
			{ID: "e3", From: "a", To: "a"},     // self loop
			{ID: "e4", From: "a", To: "d"},     // duplicate pair
			{ID: "e5", From: "a", To: "ghost"}, // missing node
		},
	}
	out, adj := Sanitize(d)
	if !adj || len(out.Nodes) != 2 || len(out.Edges) != 1 {
		t.Fatalf("unexpected sanitize result: adj=%v nodes=%d edges=%d", adj, len(out.Nodes), len(out.Edges))
	}
	if out.Nodes[0].Params["threads"].(float64) != 64 {
		t.Fatalf("threads not clamped: %v", out.Nodes[0].Params["threads"])
	}
	if _, ok := out.Nodes[0].Params["bogus"]; ok {
		t.Fatal("unknown param kept")
	}
	if _, ok := out.Nodes[0].Params["queueOrder"]; ok {
		t.Fatal("invalid select kept")
	}
	if out.Edges[0].LatencyMs != MaxLatencyMs {
		t.Fatalf("latency not clamped: %v", out.Edges[0].LatencyMs)
	}
}

func TestDeterministic(t *testing.T) {
	mk := func() []SeriesPoint {
		d := stack(2, Params{"rps": 300.0}, nil, Params{}, nil).d
		s := run(d, Options{Seed: 7}, 20)
		return s.Snapshot().Series
	}
	a, _ := json.Marshal(mk())
	b, _ := json.Marshal(mk())
	if string(a) != string(b) {
		t.Fatal("same seed produced different series")
	}
	if len(a) < 10 {
		t.Fatal("series too short")
	}
}

func TestHealthyStackSucceeds(t *testing.T) {
	d := stack(2, Params{"rps": 100.0}, nil, Params{}, nil).d
	st := run(d, Options{Seed: 1}, 20).Snapshot()
	r := st.Global.Run
	if r.Resolved < 1500 || float64(r.OK)/float64(r.Resolved) < 0.99 {
		t.Fatalf("expected healthy run, got %+v", r)
	}
	if r.P50 < 40 || r.P50 > 400 {
		t.Fatalf("implausible p50 %v", r.P50)
	}
}

func TestUtilizationMatchesLittlesLaw(t *testing.T) {
	// one app, 8 threads, 20ms cpu, 100 rps, no data: busy ~ 100*0.02 = 2 of 8 threads
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0}).node("a", KAppServer, Params{"cpuMs": 20.0, "threads": 8.0}).edge("u", "a")
	st := run(b.d, Options{Seed: 3}, 30).Snapshot()
	u := st.Nodes["a"].Util
	if u < 0.18 || u > 0.34 {
		t.Fatalf("util %v, want ~0.25", u)
	}
}

func TestOverloadRejectsWith503(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 800.0}).node("a", KAppServer, Params{"cpuMs": 40.0, "threads": 4.0, "queueCap": 10.0}).edge("u", "a")
	st := run(b.d, Options{Seed: 3}, 20).Snapshot()
	if st.Global.Run.Errors["queueFull"] == 0 {
		t.Fatalf("expected queueFull errors: %+v", st.Global.Run.Errors)
	}
}

func TestRetriesAmplifyLoadDuringOutage(t *testing.T) {
	mk := func(retries float64) *Sim {
		d := stack(1, Params{"rps": 100.0, "retries": retries, "backoff": "none", "timeoutMs": 500.0}, Params{}, nil, nil).d
		return run(d, Options{Seed: 5, Chaos: []ChaosEvent{{Type: "crash", NodeID: "a0", AtSec: 2, DownSec: 20}}}, 15)
	}
	no, yes := mk(0), mk(3)
	if yes.attempts.Count(0) <= no.attempts.Count(0)*2 {
		t.Fatalf("retries did not amplify attempts: %d vs %d", yes.attempts.Count(0), no.attempts.Count(0))
	}
}

func TestPoolExhaustionRefusesConnections(t *testing.T) {
	// 6 servers x pool 20 = 120 > max 50 connections.
	d := stack(6, Params{"rps": 600.0}, Params{"dbPool": 20.0, "threads": 32.0}, nil,
		Params{"maxConnections": 50.0, "connections": 64.0, "readMs": 100.0}).d
	st := run(d, Options{Seed: 9}, 20).Snapshot()
	if st.Global.Run.Errors["connRefused"] == 0 {
		t.Fatalf("expected connRefused: %+v", st.Global.Run.Errors)
	}
	if got := st.Nodes["db"].Extra["openConns"]; got > 50 {
		t.Fatalf("open connections %v exceeded max", got)
	}
}

func TestCacheHitRateGrowsWithCapacity(t *testing.T) {
	hit := func(capItems float64) float64 {
		d := stack(2, Params{"rps": 300.0, "keySpace": 20000.0}, nil,
			Params{"capacityItems": capItems, "startWarm": false}, nil).d
		return run(d, Options{Seed: 11}, 40).Snapshot().Nodes["c"].HitRate
	}
	small, big := hit(100), hit(5000)
	if big <= small+0.1 {
		t.Fatalf("hit rate small=%v big=%v", small, big)
	}
}

func TestEvictionPoliciesUnderScan(t *testing.T) {
	r := NewRNG(1)
	for _, pol := range []string{"lru", "lfu", "fifo", "random"} {
		s := NewStore(pol, 10, 0, r)
		for i := 0; i < 1000; i++ {
			s.Set(0, i%37, i)
			if s.Size() > 10 {
				t.Fatalf("%s: size %d > cap", pol, s.Size())
			}
			s.Get(0, (i*7)%37)
		}
	}
	lru := NewStore("lru", 2, 0, r)
	lru.Set(0, 1, 1)
	lru.Set(0, 2, 1)
	lru.Get(0, 1)
	lru.Set(0, 3, 1) // evicts 2
	if _, hit, _ := lru.Get(0, 2); hit {
		t.Fatal("lru kept least recently used")
	}
	ttl := NewStore("lru", 2, 100, r)
	ttl.Set(0, 1, 1)
	if _, hit, exp := ttl.Get(200, 1); hit || !exp {
		t.Fatal("ttl did not expire")
	}
}

func TestReplicaLagCausesStaleReads(t *testing.T) {
	mk := func(lag float64, cons string) RunStats {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 300.0, "readPct": 70.0, "keySpace": 20.0, "consistency": cons, "maxStaleMs": 50.0})
		b.node("a", KAppServer, Params{"readRouting": "nearest"}).node("p", KDatabase, nil).node("r", KReplica, Params{"replicationLagMs": lag})
		b.edge("u", "a").edge("a", "p").edge("a", "r")
		return run(b.d, Options{Seed: 13}, 20).Snapshot().Global.Run
	}
	lo, hi := mk(10, "bounded"), mk(2000, "bounded")
	if hi.Stale <= lo.Stale || hi.Fresh == 0 {
		t.Fatalf("stale lo=%d hi=%d", lo.Stale, hi.Stale)
	}
	// byConsistency routing sends strong reads to the primary
	b := &builder{}
	b.node("u", KClient, Params{"rps": 300.0, "readPct": 70.0, "keySpace": 20.0, "consistency": "strong"})
	b.node("a", KAppServer, Params{"readRouting": "byConsistency"}).node("p", KDatabase, nil).node("r", KReplica, Params{"replicationLagMs": 2000.0})
	b.edge("u", "a").edge("a", "p").edge("a", "r")
	strong := run(b.d, Options{Seed: 13}, 20).Snapshot().Global.Run
	if strong.Stale != 0 {
		t.Fatalf("strong reads were stale: %d", strong.Stale)
	}
}

func TestIdempotencyPreventsDuplicates(t *testing.T) {
	mk := func(idem bool) int {
		d := stack(1, Params{"rps": 60.0, "readPct": 0.0, "timeoutMs": 100.0, "retries": 3.0, "backoff": "none"},
			Params{"idempotencyKeys": idem}, nil, Params{"writeMs": 120.0}).d
		return run(d, Options{Seed: 17}, 20).Snapshot().Global.Run.Dups
	}
	without, with := mk(false), mk(true)
	if without == 0 || with != 0 {
		t.Fatalf("dups without=%d with=%d", without, with)
	}
}

func TestCircuitBreakerOpensAndRecovers(t *testing.T) {
	b := stack(1, Params{"rps": 100.0, "timeoutMs": 800.0}, Params{"circuitBreaker": true}, nil, nil)
	s := run(b.d, Options{Seed: 19, Chaos: []ChaosEvent{{Type: "crash", NodeID: "db", AtSec: 2, DownSec: 6}}}, 6)
	if s.Snapshot().Edges["a0>db"].Breaker != "open" {
		t.Fatalf("breaker %q, want open", s.Snapshot().Edges["a0>db"].Breaker)
	}
	s.Advance(12000)
	if got := s.Snapshot().Edges["a0>db"].Breaker; got != "closed" {
		t.Fatalf("breaker %q, want closed after recovery", got)
	}
}

func TestRateLimiterSheds(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 500.0}).node("rl", KRateLimiter, Params{"limitRps": 100.0, "burst": 10.0}).node("a", KAppServer, nil)
	b.edge("u", "rl").edge("rl", "a")
	st := run(b.d, Options{Seed: 23}, 20).Snapshot()
	if st.Global.Run.Errors["rateLimited"] < st.Global.Run.Resolved/2 {
		t.Fatalf("limiter did not shed: %+v", st.Global.Run)
	}
}

func TestQueueAndWorkersDrain(t *testing.T) {
	mk := func(conc float64) *JobStats {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 100.0, "readPct": 0.0}).node("a", KAppServer, nil).node("q", KQueue, nil)
		b.node("w", KWorker, Params{"concurrency": conc, "processMs": 100.0}).node("db", KDatabase, nil)
		b.edge("u", "a").edge("a", "q").edge("q", "w").edge("w", "db")
		return run(b.d, Options{Seed: 29}, 30).Snapshot().Global.Run.Jobs
	}
	few, many := mk(2), mk(16)
	if many.Completed <= few.Completed || many.Backlog >= few.Backlog {
		t.Fatalf("few=%+v many=%+v", few, many)
	}
}

func TestBookingConflictsNeedLocks(t *testing.T) {
	mk := func(rowLocks bool, redis bool) int {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 400.0, "readPct": 0.0, "workload": "booking", "keySpace": 20.0, "keyDist": "uniform"})
		b.node("a", KAppServer, Params{"threads": 32.0, "lockWaitMs": 500.0}).node("db", KDatabase, Params{"rowLocks": rowLocks, "connections": 64.0})
		b.edge("u", "a").edge("a", "db")
		if redis {
			b.node("r", KRedis, nil).edge("a", "r")
		}
		return run(b.d, Options{Seed: 31}, 20).Snapshot().Global.Run.Conflicts
	}
	if none := mk(false, false); none == 0 {
		t.Fatal("expected double bookings without any lock")
	}
	if rl := mk(true, false); rl != 0 {
		t.Fatalf("row locks still double booked: %d", rl)
	}
	if rd := mk(false, true); rd != 0 {
		t.Fatalf("redis lock still double booked: %d", rd)
	}
}

func TestOutlierEjectionAndHealthChecks(t *testing.T) {
	b := stack(2, Params{"rps": 200.0, "timeoutMs": 3000.0}, nil, nil, nil)
	b.d.Nodes[1].Params = Params{"outlierEjection": true}
	s := run(b.d, Options{Seed: 37, Chaos: []ChaosEvent{{Type: "slow", NodeID: "a0", AtSec: 1, Factor: 50, DurationSec: 40}}}, 20)
	ej := false
	for _, e := range s.Snapshot().Edges {
		ej = ej || e.Ejected
	}
	if !ej {
		t.Fatal("slow server was not ejected")
	}
}

func TestSyncAddsNodesLive(t *testing.T) {
	d := stack(1, Params{"rps": 600.0}, Params{"threads": 4.0, "cpuMs": 30.0}, nil, nil).d
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 41})
	s.Advance(10000)
	before := s.Snapshot().Global.Run.Failed
	nb := &builder{d: d}
	for i := 1; i < 4; i++ {
		id := fmt.Sprintf("a%d", i)
		nb.node(id, KAppServer, Params{"threads": 4.0, "cpuMs": 30.0}).edge("lb", id).edge(id, "db")
	}
	nd, _ := Sanitize(nb.d)
	s.Sync(nd)
	s.Advance(20000)
	st := s.Snapshot()
	if len(st.Nodes) != 7 {
		t.Fatalf("nodes %d", len(st.Nodes))
	}
	if st.Nodes["a3"].Arr == 0 {
		t.Fatal("new server received no traffic")
	}
	_ = before
}

// BenchmarkLargeDesign simulates 30 s of a 1,000 rps, 20-node system (run with -bench).
func BenchmarkLargeDesign(b *testing.B) {
	bd := &builder{}
	bd.node("u", KClient, Params{"rps": 1000.0, "readPct": 90.0}).node("lb", KLoadBalancer, nil).edge("u", "lb")
	bd.node("db", KDatabase, Params{"connections": 64.0}).node("rep", KReplica, nil).node("c", KCache, nil).node("q", KQueue, nil).node("w", KWorker, nil)
	bd.edge("c", "db").edge("q", "w").edge("w", "db")
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("a%d", i)
		bd.node(id, KAppServer, Params{"threads": 16.0}).edge("lb", id).edge(id, "c").edge(id, "rep").edge(id, "db").edge(id, "q")
	}
	d, _ := Sanitize(bd.d)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := New(d, Options{Seed: 1})
		s.Advance(30000)
		if s.Snapshot().Global.Run.Resolved == 0 {
			b.Fatal("nothing ran")
		}
	}
}

// A server that crashes while a request is waiting on its database must not report that request
// as a success once the database answers: the work was lost with the process.
func TestCrashFailsRequestsAlreadyInFlight(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 60.0, "readPct": 100.0, "timeoutMs": 5000.0})
	// 60 rps x ~0.65 s per request is ~40 busy threads of 64 and the pool is large: nothing queues, so every request
	// that fails at the crash was genuinely in flight.
	b.node("a", KAppServer, Params{"threads": 64.0, "dbPool": 200.0}).node("db", KDatabase, Params{"readMs": 600.0, "connections": 128.0})
	b.edge("u", "a").edge("a", "db")
	// a 10 ms outage: almost no new arrivals fall inside it
	s := run(b.d, Options{Seed: 3, Chaos: []ChaosEvent{{Type: "crash", NodeID: "a", AtSec: 3, DownSec: 0.01}}}, 8)
	lost := s.Snapshot().Global.Run.Errors["nodeDown"]
	if lost < 25 {
		t.Fatalf("only %d requests failed with nodeDown; in-flight work should be lost with the crash", lost)
	}
}

// Redis keeps scores in memory. A crash loses the writes that were never persisted: everything
// without persistence, only the last second with an append-only file.
func TestRedisCrashLosesUnpersistedWrites(t *testing.T) {
	lost := func(persistence string, crashAt float64) int {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 300.0, "readPct": 0.0, "workload": "leaderboard", "keySpace": 1000.0})
		b.node("a", KAppServer, Params{"threads": 32.0}).node("r", KRedis, Params{"persistence": persistence})
		b.edge("u", "a").edge("a", "r")
		var chaos []ChaosEvent
		if crashAt > 0 {
			chaos = []ChaosEvent{{Type: "crash", NodeID: "r", AtSec: crashAt, DownSec: 2}}
		}
		return run(b.d, Options{Seed: 5, Chaos: chaos}, 20).Snapshot().Global.Run.LostWrites
	}
	if got := lost("none", 0); got != 0 {
		t.Fatalf("no crash should lose nothing, lost %d", got)
	}
	none, aof := lost("none", 12), lost("aof", 12)
	if none < 2500 {
		t.Fatalf("without persistence about 12 s x 300 writes should be lost, lost %d", none)
	}
	if aof < 100 || aof > 500 {
		t.Fatalf("with an append-only file only ~1 s x 300 writes should be lost, lost %d", aof)
	}
}
