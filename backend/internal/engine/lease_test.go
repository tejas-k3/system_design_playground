package engine

import "testing"

// Live asks whether a key is held and unexpired without touching the store: no recency bump, no removal.
func TestLiveReadsAStoreWithoutChangingIt(t *testing.T) {
	s := newStore("lru", 4, 1000, newPRNG(1))
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

// Per-item TTL: a share of the items live for a shorter time than the rest.
func TestAShareOfItemsCanHaveAShorterTTL(t *testing.T) {
	live := func(shortPct float64) int {
		s := newStore("lru", 1000, 100000, newPRNG(1))
		s.setShortTTL(shortPct, 1000)
		for k := range 400 {
			s.Set(0, k, 1)
		}
		n := 0
		for k := range 400 {
			if s.Live(5000, k) {
				n++
			}
		}
		return n
	}
	if got := live(0); got != 400 {
		t.Errorf("with no short share every item is still live after 5 s, got %d of 400", got)
	}
	if got := live(50); got < 140 || got > 260 {
		t.Errorf("with half the items on a 1 s TTL about half should be gone after 5 s, got %d of 400 live", got)
	}
	if got := live(100); got != 0 {
		t.Errorf("with every item on the short TTL none is live after 5 s, got %d", got)
	}
}

// revalRun: one cache in front of a database, 5% writes, entries that expire every 3 s.
func revalRun(revalidate bool) (RunStats, float64) {
	d := Diagram{Nodes: []Node{
		{ID: "users", Kind: KClient, Params: Params{"rps": 200.0, "readPct": 95.0, "keySpace": 200.0, "keyDist": "uniform", "consistency": "bounded", "maxStaleMs": 200.0, "timeoutMs": 3000.0}},
		{ID: "app", Kind: KAppServer, Params: Params{"threads": 64.0, "dbPool": 64.0}},
		{ID: "db", Kind: KDatabase},
		{ID: "c", Kind: KCache, Params: Params{"capacityItems": 1000.0, "ttlSec": 3.0, "writePolicy": "none", "revalidate": revalidate}},
	}, Edges: []Edge{{ID: "u", From: "users", To: "app", LatencyMs: 20}, {ID: "d", From: "app", To: "db", LatencyMs: 2}, {ID: "ac", From: "app", To: "c", LatencyMs: 2}, {ID: "cd", From: "c", To: "db", LatencyMs: 2}}}
	d, _ = Sanitize(d)
	s := New(d, Options{Seed: 1, StopTrafficAtMs: 40000})
	for !s.Finished() && s.Now() < 100000 {
		s.Advance(100)
	}
	return s.RunSnapshot(), s.Snapshot().Nodes["db"].Arr
}

// A revalidating cache asks the origin whether an expired entry is still current and renews it if so, so the database
// is read far less often than when every expiry is a miss, and reads are no staler.
func TestARevalidatingCacheRenewsUnchangedEntriesInsteadOfRefetching(t *testing.T) {
	plain, plainDB := revalRun(false)
	reval, revalDB := revalRun(true)
	if plain.Revalidated != 0 || reval.Revalidated == 0 {
		t.Fatalf("only the revalidating cache should renew entries: %d against %d", plain.Revalidated, reval.Revalidated)
	}
	if revalDB >= plainDB*0.8 {
		t.Errorf("renewing unchanged entries should cut the database's reads clearly: %.0f against %.0f", revalDB, plainDB)
	}
	if reval.Stale > plain.Stale+plain.Stale/5+2 {
		t.Errorf("a version check must not make reads staler: %d against %d", reval.Stale, plain.Stale)
	}
}

// A lease needs the origin to remember who holds what; that bookkeeping adds half of the cache's memory cost.
func TestLeaseBookkeepingIsChargedAsHalfTheMemoryCost(t *testing.T) {
	cost := func(policy string) float64 {
		return Spec(KCache).cost(Params{"capacityItems": 2000.0, "writePolicy": policy})
	}
	none, lease, push := cost("none"), cost("lease"), cost("invalidate")
	if push != none {
		t.Errorf("pushing keeps no record of holders, so it costs the same as doing nothing: %.4f against %.4f", push, none)
	}
	if got, want := lease-none, 0.025; got < want-1e-9 || got > want+1e-9 {
		t.Errorf("a 2,000-item cache should pay %.3f a hour for lease bookkeeping, paid %.4f", want, got)
	}
}
