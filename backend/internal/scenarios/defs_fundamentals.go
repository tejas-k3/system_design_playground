package scenarios

import (
	"fmt"
	"sysdesign/internal/engine"
)

// registerFundamentals registers the first challenges: a database, a front door, caching, connection limits, retries, stale reads and double writes.
func (c *catalog) registerFundamentals() {
	// 1 ─ Tutorial ───────────────────────────────────────────────────────────────────
	c.register(&Scenario{
		ID: "hello-world", Title: "Hello, database", Difficulty: "Tutorial",
		Summary: "Your app answers every request, but remembers nothing.",
		Story:   "Visitors are hitting your app server and getting answers, but nothing is stored anywhere. Connect the server to the database (drag from the round handle on the server's right edge) so it does real work, then press Play.",
		Diagram: (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 30.0}).
			n("app", engine.KAppServer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[2], rowMid, nil).
			e("users", "app").done(),
		Solution: (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 30.0}).
			n("app", engine.KAppServer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[2], rowMid, nil).
			e("users", "app").e("app", "db").done(),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(0.6)},
		DurationSec: 30, WarmupSec: 3, Speed: 1,
		Hints: [3]string{
			"A server with no data store behind it cannot persist anything.",
			"Drag from the app server's edge handle to the database to draw a connection.",
			"Which box does the line have to start from, and which box does it end at? Requests flow in that direction.",
		},
		Answer: "The answer: use the connect tool, click the app server, then click the database.",
		Predictions: []Prediction{{ID: "p1", Question: "With one app server and one database, what limits throughput first?", Options: []string{"The app server's threads", "The database's disk speed", "Bandwidth between the two boxes"}, Answer: 0,
			Explain: "Each request holds a thread for CPU plus the database wait; 8 threads at ~40 ms each serve about 200 requests per second."}},
		Debrief: "Every request travels users → app → database and back. Latency is the sum of the hops plus the time spent waiting for a free {{thread-pool|thread}}. Keep that picture in mind: every later challenge is about where the waiting happens.",
	})

	// 2 ─ Easy ────────────────────────────────────────────────────────────────────────
	opening := func(servers int, lb bool) engine.Diagram {
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 80.0, "readPct": 85.0}).
			n("db", engine.KDatabase, cols[4], rowMid, nil)
		if !lb {
			b.n("app1", engine.KAppServer, cols[2], rowMid, P{"threads": 4.0}).e("users", "app1").e("app1", "db")
			return b.done()
		}
		b.n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).e("users", "lb")
		return b.servers(servers, cols[2], rowTop, 100, P{"threads": 8.0}).done()
	}
	c.register(&Scenario{
		ID: "opening-night", Title: "Opening night", Difficulty: "Easy",
		Summary: "A launch sends traffic from 80 to 600 requests per second.",
		Story:   "Marketing just announced the launch. Traffic will jump to 600 requests per second for a while. One small server will not survive it.",
		Diagram: opening(0, false), Solution: opening(4, true),
		Traffic:     map[string][]engine.TrafficPoint{"users": {{0, 80}, {8, 80}, {14, 600}, {50, 600}, {56, 80}, {70, 80}}},
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(500), MaxCostPerHour: f(0.9)},
		DurationSec: 70, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Add up how many requests per second one server can handle: {{thread-pool|threads}} ÷ time each request holds a thread.",
			"You need a load balancer in front of several app servers.",
			"Divide the peak load by what one server can take and round up, then check the total against the cost cap. Is there room for a spare?",
		},
		Answer: "The answer: four 8-thread servers behind one load balancer will carry 600 rps within the cost cap.",
		Predictions: []Prediction{{ID: "p1", Question: "A 4-thread server spends ~40 ms per request. Roughly what is its limit?", Options: []string{"~100 rps", "~400 rps", "~1000 rps"}, Answer: 0,
			Explain: "4 threads ÷ 0.04 s ≈ 100 requests per second. Beyond that the queue fills and requests are rejected."}},
		Debrief: "Horizontal scaling is a load balancer plus enough identical servers. Notice the cost: {{thread-pool|threads}} are not free, so you want just enough {{queueing-knee|headroom}}, not ten times too much.",
	})

	// 3 ─ Easy ────────────────────────────────────────────────────────────────────────
	slow := func(cache bool) engine.Diagram {
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 500.0, "readPct": 95.0, "keySpace": 5000.0, "zipfS": 1.1}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, P{"readMs": 40.0}).
			e("users", "lb")
		if cache {
			b.n("cache", engine.KCache, cols[3], rowTop, P{"capacityItems": 2000.0})
		}
		if cache {
			b.servers(3, cols[2], rowTop, 100, nil, "cache")
		} else {
			b.servers(3, cols[2], rowTop, 100, nil)
		}
		if cache {
			b.e("cache", "db")
		}
		return b.done()
	}
	c.register(&Scenario{
		ID: "slow-reads", Title: "Slow reads", Difficulty: "Easy",
		Summary: "The database groans under a read-heavy, popularity-skewed workload.",
		Story:   "95% of requests are reads and a few items are far more popular than the rest. Each read costs the database 40 ms, and it is saturated.",
		Diagram: slow(false), Solution: slow(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(250), MaxCostPerHour: f(1.4)},
		DurationSec: 50, WarmupSec: 6, Speed: 1,
		Hints: [3]string{
			"Most requests ask for the same few items. Do they all need to reach the database?",
			"Put a cache between the app servers and the database.",
			"A cache that misses still has to fetch from somewhere. Which connections does the cache need so that it can get what it lacks?",
		},
		Answer: "The answer: connect each app server to a cache, and the cache to the database so misses are fetched for you.",
		Predictions: []Prediction{{ID: "p1", Question: "A skewed workload with a cache holding the hottest 40% of items will hit roughly...", Options: []string{"About 40%: hit rate follows cache size", "Far above 40%: the hot items are in it", "Under 40%: hot items get evicted first"}, Answer: 1,
			Explain: "Popularity is skewed, so a small fraction of items accounts for most requests. A modest cache absorbs most reads."}},
		Debrief: "Caches work because access is skewed. The {{cache-hit-rate|hit rate}} is not the cache size over the data size, it is the share of requests that land on the items you kept.",
	})

	// 4 ─ Medium, config only ─────────────────────────────────────────────────────────
	crunch := func(pool float64) engine.Diagram {
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 800.0, "readPct": 100.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, P{"readMs": 60.0, "connections": 128.0, "maxConnections": 64.0}).
			e("users", "lb")
		return b.servers(6, cols[2], 20, 70, P{"threads": 24.0, "dbPool": pool, "queueCap": 200.0}).done()
	}
	c.register(&Scenario{
		ID: "connection-crunch", Title: "Connection crunch", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Every server may open 100 connections. The database allows 64 in total.",
		Story:   "Six app servers each keep a {{connection-pool|pool}} of up to 100 database connections, but the database only accepts 64 at once. Under load, new connections are refused. The architecture is fixed: fix it with configuration.",
		Diagram: crunch(100), Solution: crunch(10),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(600)},
		DurationSec: 40, WarmupSec: 6, Speed: 1,
		Hints: [3]string{
			"Open the database and compare its maximum connections with servers × pool size.",
			"{{connection-pool|Pools}} should add up to less than the database limit, leaving headroom.",
			"Multiply the servers by the pool each may open. How does that compare with what the database accepts, and how much room do you want to leave?",
		},
		Answer: "The answer: six servers × a {{connection-pool|pool}} of 10 is 60 connections, under the limit of 64.",
		Predictions: []Prediction{{ID: "p1", Question: "If you shrink the pool, what happens to requests that cannot get a connection?", Options: []string{"They are refused with an error at once", "They wait for a free connection", "They open a temporary extra connection"}, Answer: 1,
			Explain: "A pool makes callers wait for a free connection instead of opening a new one, which keeps the database healthy."}},
		Debrief: "{{connection-pool|Connection pools}} protect the database from the fleet in front of it. Pool size is a budget shared by every server: per-server pool × number of servers must stay under the database's limit.",
	})

	// 5 ─ Medium, config only ─────────────────────────────────────────────────────────
	storm := func(fixed bool) engine.Diagram {
		app := P{"threads": 8.0, "queueCap": 400.0}
		cl := P{"rps": 380.0, "readPct": 100.0, "timeoutMs": 800.0, "retries": 4.0, "backoff": "none"}
		if fixed {
			app = P{"threads": 8.0, "queueCap": 400.0, "queueOrder": "lifo", "queueTimeoutMs": 400.0, "callTimeoutMs": 300.0, "circuitBreaker": true}
			cl = P{"rps": 380.0, "readPct": 100.0, "timeoutMs": 800.0, "retries": 2.0, "backoff": "exponential"}
		}
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, cl).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, P{"readMs": 10.0}).
			e("users", "lb")
		return b.servers(3, cols[2], rowTop, 100, app).done()
	}
	c.register(&Scenario{
		ID: "retry-pileup", Title: "Retry pile-up", Difficulty: "Medium", ConfigOnly: true,
		Summary: "A short database slowdown turns into a long outage because everyone retries at once.",
		Story:   "The database gets 10× slower for ten seconds. When it recovers, the system does not: clients {{retry-storm|retry instantly}} and servers keep working on requests whose users already left. The architecture is fixed.",
		Diagram: storm(false), Solution: storm(true),
		Chaos:       []engine.ChaosEvent{{Type: "slow", NodeID: "db", AtSec: 15, Factor: 12, DurationSec: 10}},
		Goals:       engine.Goals{MinSuccessRate: 0.85, MaxP95Ms: f(1500), MaxCostPerHour: f(1.2)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Retries add load exactly when the system is struggling. How are the clients retrying?",
			"Back off exponentially, retry less, and make servers drop work nobody is waiting for.",
			"Four things pile work onto a struggling system: how retries are timed and counted, the order of the queue, how long work may wait, and calls that wait forever. Which have you not changed yet?",
		},
		Answer: "The answer: exponential backoff, 2 retries, newest-first queue, a short max queue wait, and a call timeout with a {{circuit-breaker|circuit breaker}}.",
		Predictions: []Prediction{{ID: "p1", Question: "With oldest-first queues under overload, who gets served?", Options: []string{"Users who are still waiting for an answer", "Users who have already given up", "Whoever sent the cheapest request"}, Answer: 1,
			Explain: "The oldest requests are the ones whose callers have likely timed out. Serving them wastes capacity; newest-first or a queue timeout avoids it."}},
		Debrief: "A {{retry-storm|retry storm}} is a {{metastable-failure|feedback loop}}: slowness causes timeouts, timeouts cause retries, retries cause slowness. Break it with backoff and jitter, bounded retries, {{load-shedding|load shedding}} and {{circuit-breaker|circuit breakers}}.",
	})

	// 6 ─ Medium ──────────────────────────────────────────────────────────────────────
	stale := func(fixed bool) engine.Diagram {
		app := P{"readRouting": "nearest", "threads": 16.0}
		cache := P{"capacityItems": 1000.0, "ttlSec": 30.0, "writePolicy": "none"}
		if fixed {
			app = P{"readRouting": "byConsistency", "threads": 16.0}
			cache = P{"capacityItems": 1000.0, "ttlSec": 30.0, "writePolicy": "invalidate"}
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 300.0, "readPct": 85.0, "keySpace": 200.0, "keyDist": "uniform", "consistency": "bounded", "maxStaleMs": 500.0}).
			n("app", engine.KAppServer, cols[1], rowMid, app).
			n("cache", engine.KCache, cols[2], rowTop, cache).
			n("primary", engine.KDatabase, cols[3], rowMid, nil).label("primary", "Primary DB").
			n("replica", engine.KReplica, cols[3], rowBot, P{"replicationLagMs": 2000.0}).
			e("users", "app").e("app", "cache").e("app", "replica").e("app", "primary").done()
	}
	c.register(&Scenario{
		ID: "stale-prices", Title: "Stale prices", Difficulty: "Medium",
		Summary: "Shoppers see yesterday's prices because copies lag behind the primary.",
		Story:   "Prices change often and users tolerate at most half a second of staleness. The cache and the replica both serve old data.",
		Diagram: stale(false), Solution: stale(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxStaleReadPct: f(0.02), MaxP95Ms: f(300), MaxCostPerHour: f(1.2)},
		DurationSec: 50, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Two components can serve old data: the cache and the replica. Which one does a read use?",
			"Reads that need freshness should go to the primary, and writes should clear stale cache entries.",
			"Two things can serve an old price: the replica and the cache. Which setting on each decides whether a read can be stale?",
		},
		Answer: "The answer: set the app server's read routing to 'By freshness' and the cache to invalidate on writes.",
		Predictions: []Prediction{{ID: "p1", Question: "Why does a 2 second replication lag matter even though the replica is 'a copy'?", Options: []string{"Reads may return versions older than a recent write", "Writes take longer because they wait for the replica", "The replica refuses reads until it catches up"}, Answer: 0,
			Explain: "A replica applies writes after a delay, so a read right after a write can miss it."}},
		Debrief: "Caches and replicas trade freshness for speed. Decide per read {{consistency-levels|how stale is acceptable}}, route accordingly, and invalidate on writes so the cache does not outlive the truth.",
	})

	// 7 ─ Medium ──────────────────────────────────────────────────────────────────────
	booking := func(fixed bool) engine.Diagram {
		dbp := P{"connections": 32.0}
		app := P{"threads": 24.0}
		if fixed {
			dbp = P{"connections": 32.0, "rowLocks": true}
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 250.0, "readPct": 0.0, "workload": "booking", "keySpace": 300.0, "keyDist": "uniform"}).
			n("app", engine.KAppServer, cols[1], rowMid, app).
			n("db", engine.KDatabase, cols[2], rowMid, dbp).
			e("users", "app").e("app", "db").done()
	}
	c.register(&Scenario{
		ID: "double-booked", Title: "Double-booked seats", Difficulty: "Medium",
		Summary: "Two people buy the same seat at the same moment.",
		Story:   "Only 300 seats exist and people are racing for them. Booking is 'check, then write', so two overlapping requests for the same seat both succeed.",
		Diagram: booking(false), Solution: booking(true),
		Goals:       engine.Goals{MinSuccessRate: 0.98, MaxConflicts: i(0), MaxP95Ms: f(500), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.05)},
		DurationSec: 30, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"The bug is a race between the check and the write. Something has to make them atomic per seat.",
			"The database can serialize writes per item, or Redis can hand out a lock.",
			"Two requests check the same seat at the same moment. Which component could make the second wait, or refuse, until the first has finished?",
		},
		Answer: "The answer: turn on row locks in the database, or add Redis and give the app server a lock wait.",
		Predictions: []Prediction{{ID: "p1", Question: "A fail-fast Redis lock (no wait) will…", Options: []string{"Reject some honest bookings that collide", "Allow a few double bookings under load", "Queue colliding bookings until the lock frees"}, Answer: 0,
			Explain: "Failing fast avoids double bookings but turns every collision into an error unless you wait a little for the lock."}},
		Debrief: "Correctness under concurrency needs an explicit mechanism: row locks, optimistic version checks, or a distributed lock with a lease. Locks have costs: contention, waiting, and expiry races.",
	})

	// 8 ─ Medium ──────────────────────────────────────────────────────────────────────
	charge := func(fixed bool) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 30.0, "readPct": 0.0, "timeoutMs": 450.0, "retries": 2.0, "backoff": "none"}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 24.0, "idempotencyKeys": fixed}).
			n("pay", engine.KExternalAPI, cols[2], rowTop, P{"latencyMs": 70.0, "errorPct": 0.0}).label("pay", "Payments API").
			n("db", engine.KDatabase, cols[2], rowBot, P{"writeMs": 130.0, "connections": 16.0}).
			e("users", "app").e("app", "pay").e("app", "db").done()
	}
	c.register(&Scenario{
		ID: "charged-twice", Title: "Charged twice", Difficulty: "Medium",
		Summary: "A slow write times out, the client retries, and the customer pays twice.",
		Story:   "Orders sometimes take longer than the client's patience. The client retries, but the first attempt keeps running and also completes.",
		Diagram: charge(false), Solution: charge(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxDuplicates: i(0), MaxP95Ms: f(1500), MaxCostPerHour: f(1.2)},
		DurationSec: 40, WarmupSec: 4, Speed: 1,
		Hints: [3]string{
			"A timeout does not cancel the work. Both attempts may commit.",
			"The server needs a way to recognise a retry of the same request.",
			"The retry looks identical to the first request. What would the server need to remember about a request to notice it has seen it before?",
		},
		Answer: "The answer: enable {{idempotency|idempotency}} keys on the app server.",
		Predictions: []Prediction{{ID: "p1", Question: "What does an idempotency key guarantee?", Options: []string{"The write is applied at most once per key", "The request is sent to the server only once", "The request can never be lost on the way"}, Answer: 0,
			Explain: "The server remembers the key and returns the earlier outcome instead of applying the write again."}},
		Debrief: "Retries are only safe for {{idempotency|idempotent}} operations. Make writes idempotent with a client-supplied key, then retry freely.",
	})

	// 8b ─ Medium, config only: concurrent transfers silently overwrite each other ───────
	transfers := func(fixed bool) engine.Diagram {
		iso := "readCommitted"
		if fixed {
			iso = "serializable"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 60.0, "workload": "transfer", "keySpace": 60.0, "keyDist": "uniform", "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "isolation": iso}).
			e("users", "app").e("app", "db").done()
	}
	c.register(&Scenario{
		ID: "disappearing-money", Title: "The disappearing money", Difficulty: "Medium",
		Summary: "Two transfers touch the same account at once and one of them vanishes.",
		Story:   "A payments team moves money between 60 accounts. Every transfer succeeds, yet the books are short: transfers that overlap on an account read the same balance and the second write overwrites the first.",
		Diagram: transfers(false), Solution: transfers(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxLostUpdates: i(0), MaxP95Ms: f(600), MaxCostPerHour: f(1.2)},
		DurationSec: 30, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Nothing is failing, so look at what the database promises about overlapping transactions.",
			"The database has an isolation level. The default lets overlapping read-modify-write transfers overwrite each other.",
			"Which isolation level stops two transfers from reading the same balance and both writing back? Then look at what it costs under contention.",
		},
		Answer: "The answer: raise the isolation level on the database to serializable and watch what it costs under contention.",
		Predictions: []Prediction{{ID: "p1", Question: "Moving to a stronger isolation level will…", Options: []string{"Make every transfer slower, overlapping or not", "Make overlapping transfers wait or abort", "Stop transfers from ever being aborted"}, Answer: 1,
			Explain: "Stronger isolation turns silent overwrites into waiting (locks) or errors (aborts and deadlocks): correctness is paid for with latency under contention."}},
		Debrief: "Weak isolation is fast and silently wrong. Stronger isolation is right but makes contending transactions wait, abort or deadlock, and the cost grows with how hot the shared rows are.",
	})
	// Who needs telling: four caches in front of one database; leaving them alone serves stale items, telling all of them costs messages
	tellWho := func(fixed bool) engine.Diagram {
		policy := "none"
		if fixed {
			policy = "lease"
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 200.0, "readPct": 70.0, "keySpace": 2000.0, "keyDist": "uniform", "consistency": "bounded", "maxStaleMs": 200.0, "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 64.0, "dbPool": 64.0}).
			n("db", engine.KDatabase, cols[3], rowMid, nil).
			e("users", "app").e("app", "db")
		for k := 1; k <= 4; k++ {
			id := fmt.Sprintf("cache%d", k)
			b.n(id, engine.KCache, cols[2], rowTop+float64(k-1)*70, P{"capacityItems": 300.0, "ttlSec": 30.0, "writePolicy": policy}).e("app", id)
		}
		return b.done()
	}
	c.register(&Scenario{
		ID: "who-needs-telling", Title: "Who needs telling?", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Four caches serve old prices, and telling all of them about every change costs too many messages.",
		Story:   "A catalogue is served from four caches in front of one database. Items change all the time and shoppers tolerate at most 200 ms of staleness. Today the caches are left alone when an item changes, so they keep the old one until it expires. Telling every cache about every change would fix that, but each message is a round trip the origin waits for during the write, and the caches can take only a few thousand of them in this run.",
		Diagram: tellWho(false), Solution: tellWho(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxStaleReadPct: f(0.0005), MaxCacheMessages: i(3000), MaxP95Ms: f(185), MaxCostPerHour: f(3.0), StarP95Ms: f(175), StarCostPerHour: f(2.8)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The goals count stale reads and the messages the origin sends to the caches. Which setting decides what happens to cached copies when data is written?",
			"Telling every cache about every write makes reads fresh, but it also messages caches that do not hold the item at all: each cache holds only a small part of the catalogue.",
			"What if the origin told only the caches that actually hold the item? What would an entry's time to live then stand for?",
		},
		Answer: "The answer: set every cache's write policy to 'Lease'. An entry is a lease for as long as its time to live, and the origin tells only the caches holding one, so the copies stay fresh and the messages fall from about 9,000 to about 1,500.",
		Predictions: []Prediction{{ID: "p1", Question: "Telling every cache about every write will…", Options: []string{"Keep reads fresh at the price of many messages", "Leave reads stale, because the caches hear too late", "Send few messages, because a cache needs only some"}, Answer: 0,
			Explain: "Every write reaches all four caches, so reads stay fresh, but a cache that holds nothing for the item gets a message anyway: about 9,000 messages in the run, over three times the limit."}},
		Debrief: "Pulling is simple (each cache looks again when its time to live ends) but stale until then. Pushing tells every cache about every write: fresh, but a message per cache per write, mostly wasted when a cache holds a small part of the data. A {{lease|lease}} is a promise that holds for a limited time: the origin tells only the caches that hold an unexpired copy, and a cache without one simply fetches afresh. The price is that the origin must remember who holds what; this simulator does not charge for that.",
	})
}
