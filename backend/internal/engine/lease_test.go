package engine

import "testing"

// Live asks whether a key is held and unexpired without touching the store: no recency bump, no removal.
func TestLiveReadsAStoreWithoutChangingIt(t *testing.T) {
	s := NewStore("lru", 4, 1000, NewRNG(1))
	s.Set(0, 7, 3)
	if !s.Live(500, 7) || s.Live(500, 8) {
		t.Fatal("a held key is live and an absent one is not")
	}
	if s.Live(1500, 7) {
		t.Error("an expired entry is not live")
	}
	if s.Size() != 1 {
		t.Errorf("Live removed the expired entry: size %d", s.Size())
	}
}

// leaseRun: four caches in front of one database, a third of the traffic writes, items spread over more keys than a
// cache holds, so any one write is of interest to only some of the caches.
func leaseRun(policy string) RunStats { return leaseRunWith(policy, nil, nil) }

func leaseRunWith(policy string, cache, users Params) RunStats {
	u := Params{"rps": 200.0, "readPct": 70.0, "keySpace": 2000.0, "keyDist": "uniform", "consistency": "bounded", "maxStaleMs": 200.0, "timeoutMs": 3000.0}
	for k, v := range users {
		u[k] = v
	}
	d := Diagram{Nodes: []Node{
		{ID: "users", Kind: KClient, Params: u},
		{ID: "app", Kind: KAppServer, Params: Params{"threads": 64.0, "dbPool": 64.0}},
		{ID: "db", Kind: KDatabase},
	}, Edges: []Edge{{ID: "u", From: "users", To: "app", LatencyMs: 20}, {ID: "d", From: "app", To: "db", LatencyMs: 2}}}
	for _, id := range []string{"c1", "c2", "c3", "c4"} {
		p := Params{"capacityItems": 300.0, "ttlSec": 30.0, "writePolicy": policy}
		for k, v := range cache {
			p[k] = v
		}
		d.Nodes = append(d.Nodes, Node{ID: id, Kind: KCache, Params: p})
		d.Edges = append(d.Edges, Edge{ID: "a" + id, From: "app", To: id, LatencyMs: 2})
	}
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 1, StopTrafficAtMs: 40000})
	for !s.Finished() && s.Now() < 100000 {
		s.Advance(100)
	}
	return s.RunSnapshot()
}

// Pull leaves caches stale until their entries expire; push keeps them fresh but tells every cache about every write;
// a lease keeps them just as fresh while telling only the caches that hold the item.
func TestALeaseTellsOnlyTheCachesThatHoldTheItem(t *testing.T) {
	pull, push, lease := leaseRun("none"), leaseRun("invalidate"), leaseRun("lease")
	if pull.CacheMsgs != 0 || pull.Stale == 0 {
		t.Errorf("pull sends nothing and serves stale reads: %d messages, %d stale of %d", pull.CacheMsgs, pull.Stale, pull.Fresh)
	}
	if push.CacheMsgs == 0 || push.Stale*20 > pull.Stale {
		t.Errorf("push should tell the caches and leave almost no stale reads: %d messages, %d stale against %d for pull", push.CacheMsgs, push.Stale, pull.Stale)
	}
	if lease.CacheMsgs*2 >= push.CacheMsgs {
		t.Errorf("a lease should send less than half the messages of pushing to every cache: %d against %d", lease.CacheMsgs, push.CacheMsgs)
	}
	if lease.Stale > push.Stale+push.Stale/10+2 {
		t.Errorf("a lease should be as fresh as push: %d stale reads against %d", lease.Stale, push.Stale)
	}
}

// A cache that keeps big objects in pieces holds them under other keys, so the origin cannot tell from the item's own
// key whether it holds anything: a lease must still reach it, as pushing does.
func TestALeaseStillReachesACacheThatKeepsPieces(t *testing.T) {
	cache, users := Params{"rangeKB": 100.0}, Params{"respKB": 500.0}
	push, lease := leaseRunWith("invalidate", cache, users), leaseRunWith("lease", cache, users)
	if push.CacheMsgs == 0 {
		t.Fatal("pushing should send messages")
	}
	if lease.CacheMsgs != push.CacheMsgs {
		t.Errorf("a cache that keeps pieces cannot be skipped: %d messages against %d for push", lease.CacheMsgs, push.CacheMsgs)
	}
}
