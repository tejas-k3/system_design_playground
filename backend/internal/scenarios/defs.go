package scenarios

import "sysdesign/internal/engine"

func init() {
	// 1 ─ Tutorial ───────────────────────────────────────────────────────────────────
	register(&Scenario{
		ID: "hello-world", Title: "Hello, database", Difficulty: "Tutorial",
		Summary: "Your app answers every request, but remembers nothing.",
		Story:   "Visitors are hitting your app server and getting answers, but nothing is stored anywhere. Connect the server to the database so it does real work, then press Play.",
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
			"Use the connect tool, click the app server, then click the database.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "With one app server and one database, what limits throughput first?", Options: []string{"App server threads", "Database disk", "The network"}, Answer: 0,
			Explain: "Each request holds a thread for CPU plus the database wait; 8 threads at ~40 ms each serve about 200 requests per second."}},
		Debrief: "Every request travels users → app → database and back. Latency is the sum of the hops plus the time spent waiting for a free thread. Keep that picture in mind: every later challenge is about where the waiting happens.",
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
		for k := 0; k < servers; k++ {
			id := "app" + string(rune('1'+k))
			b.n(id, engine.KAppServer, cols[2], rowTop+float64(k)*100, P{"threads": 8.0}).e("lb", id).e(id, "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "opening-night", Title: "Opening night", Difficulty: "Easy",
		Summary: "A launch sends traffic from 80 to 600 requests per second.",
		Story:   "Marketing just announced the launch. Traffic will jump to 600 requests per second for a while. One small server will not survive it.",
		Diagram: opening(0, false), Solution: opening(4, true),
		Traffic:     map[string][]engine.TrafficPoint{"users": {{0, 80}, {8, 80}, {14, 600}, {50, 600}, {56, 80}, {70, 80}}},
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(500), MaxCostPerHour: f(0.9)},
		DurationSec: 70, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Add up how many requests per second one server can handle: threads ÷ time each request holds a thread.",
			"You need a load balancer in front of several app servers.",
			"Four 8-thread servers behind one load balancer will carry 600 rps within the cost cap.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A 4-thread server spends ~40 ms per request. Roughly what is its limit?", Options: []string{"~100 rps", "~400 rps", "~1000 rps"}, Answer: 0,
			Explain: "4 threads ÷ 0.04 s ≈ 100 requests per second. Beyond that the queue fills and requests are rejected."}},
		Debrief: "Horizontal scaling is a load balancer plus enough identical servers. Notice the cost: threads are not free, so you want just enough headroom, not ten times too much.",
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
		for k := 0; k < 3; k++ {
			id := "app" + string(rune('1'+k))
			b.n(id, engine.KAppServer, cols[2], rowTop+float64(k)*100, nil).e("lb", id).e(id, "db")
			if cache {
				b.e(id, "cache")
			}
		}
		if cache {
			b.e("cache", "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "slow-reads", Title: "Slow reads", Difficulty: "Easy",
		Summary: "The database groans under a read-heavy, popularity-skewed workload.",
		Story:   "95% of requests are reads and a few items are far more popular than the rest. Each read costs the database 40 ms, and it is saturated.",
		Diagram: slow(false), Solution: slow(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(250), MaxCostPerHour: f(1.4)},
		DurationSec: 50, WarmupSec: 6, Speed: 1,
		Hints: [3]string{
			"Most requests ask for the same few items. Do they all need to reach the database?",
			"Put a cache between the app servers and the database.",
			"Connect each app server to a cache, and the cache to the database so misses are fetched for you.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A skewed workload with a cache holding the hottest 40% of items will hit roughly...", Options: []string{"40% of the time", "Far more than 40%", "Almost never"}, Answer: 1,
			Explain: "Popularity is skewed, so a small fraction of items accounts for most requests. A modest cache absorbs most reads."}},
		Debrief: "Caches work because access is skewed. The hit rate is not the cache size over the data size, it is the share of requests that land on the items you kept.",
	})

	// 4 ─ Medium, config only ─────────────────────────────────────────────────────────
	crunch := func(pool float64) engine.Diagram {
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 800.0, "readPct": 100.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, P{"readMs": 60.0, "connections": 128.0, "maxConnections": 64.0}).
			e("users", "lb")
		for k := 0; k < 6; k++ {
			id := "app" + string(rune('1'+k))
			b.n(id, engine.KAppServer, cols[2], 20+float64(k)*70, P{"threads": 24.0, "dbPool": pool, "queueCap": 200.0}).e("lb", id).e(id, "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "connection-crunch", Title: "Connection crunch", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Every server may open 100 connections. The database allows 64 in total.",
		Story:   "Six app servers each keep a pool of up to 100 database connections, but the database only accepts 64 at once. Under load, new connections are refused. The architecture is fixed: fix it with configuration.",
		Diagram: crunch(100), Solution: crunch(10),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(600)},
		DurationSec: 40, WarmupSec: 6, Speed: 1,
		Hints: [3]string{
			"Open the database and compare its maximum connections with servers × pool size.",
			"Pools should add up to less than the database limit, leaving headroom.",
			"Six servers × a pool of 10 is 60 connections, under the limit of 64.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "If you shrink the pool, what happens to requests that cannot get a connection?", Options: []string{"They are refused immediately", "They wait for a free connection", "They are retried by the database"}, Answer: 1,
			Explain: "A pool makes callers wait for a free connection instead of opening a new one, which keeps the database healthy."}},
		Debrief: "Connection pools protect the database from the fleet in front of it. Pool size is a budget shared by every server: per-server pool × number of servers must stay under the database's limit.",
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
		for k := 0; k < 3; k++ {
			id := "app" + string(rune('1'+k))
			b.n(id, engine.KAppServer, cols[2], rowTop+float64(k)*100, app).e("lb", id).e(id, "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "retry-pileup", Title: "Retry pile-up", Difficulty: "Medium", ConfigOnly: true,
		Summary: "A short database slowdown turns into a long outage because everyone retries at once.",
		Story:   "The database gets 10× slower for ten seconds. When it recovers, the system does not: clients retry instantly and servers keep working on requests whose users already left. The architecture is fixed.",
		Diagram: storm(false), Solution: storm(true),
		Chaos:       []engine.ChaosEvent{{Type: "slow", NodeID: "db", AtSec: 15, Factor: 12, DurationSec: 10}},
		Goals:       engine.Goals{MinSuccessRate: 0.85, MaxP95Ms: f(1500), MaxCostPerHour: f(1.2)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Retries add load exactly when the system is struggling. How are the clients retrying?",
			"Back off exponentially, retry less, and make servers drop work nobody is waiting for.",
			"Try: exponential backoff, 2 retries, newest-first queue, a short max queue wait, and a call timeout with a circuit breaker.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "With oldest-first queues under overload, who gets served?", Options: []string{"Users still waiting", "Users who already gave up", "Nobody"}, Answer: 1,
			Explain: "The oldest requests are the ones whose callers have likely timed out. Serving them wastes capacity; newest-first or a queue timeout avoids it."}},
		Debrief: "A retry storm is a feedback loop: slowness causes timeouts, timeouts cause retries, retries cause slowness. Break it with backoff and jitter, bounded retries, load shedding and circuit breakers.",
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
	register(&Scenario{
		ID: "stale-prices", Title: "Stale prices", Difficulty: "Medium",
		Summary: "Shoppers see yesterday's prices because copies lag behind the primary.",
		Story:   "Prices change often and users tolerate at most half a second of staleness. The cache and the replica both serve old data.",
		Diagram: stale(false), Solution: stale(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxStaleReadPct: f(0.02), MaxP95Ms: f(300), MaxCostPerHour: f(1.2)},
		DurationSec: 50, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Two components can serve old data: the cache and the replica. Which one does a read use?",
			"Reads that need freshness should go to the primary, and writes should clear stale cache entries.",
			"Set the app server's read routing to 'By freshness' and the cache to invalidate on writes.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Why does a 2 second replication lag matter even though the replica is 'a copy'?", Options: []string{"Reads may return versions older than a recent write", "Writes get slower", "The replica refuses connections"}, Answer: 0,
			Explain: "A replica applies writes after a delay, so a read right after a write can miss it."}},
		Debrief: "Caches and replicas trade freshness for speed. Decide per read how stale is acceptable, route accordingly, and invalidate on writes so the cache does not outlive the truth.",
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
	register(&Scenario{
		ID: "double-booked", Title: "Double-booked seats", Difficulty: "Medium",
		Summary: "Two people buy the same seat at the same moment.",
		Story:   "Only 300 seats exist and people are racing for them. Booking is 'check, then write', so two overlapping requests for the same seat both succeed.",
		Diagram: booking(false), Solution: booking(true),
		Goals:       engine.Goals{MinSuccessRate: 0.98, MaxConflicts: i(0), MaxP95Ms: f(500), MaxCostPerHour: f(1.2)},
		DurationSec: 30, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"The bug is a race between the check and the write. Something has to make them atomic per seat.",
			"The database can serialize writes per item, or Redis can hand out a lock.",
			"Turn on row locks in the database, or add Redis and give the app server a lock wait.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A fail-fast Redis lock (no wait) will…", Options: []string{"Reject some honest bookings that collide", "Never double book and never reject", "Make the database slower"}, Answer: 0,
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
	register(&Scenario{
		ID: "charged-twice", Title: "Charged twice", Difficulty: "Medium",
		Summary: "A slow write times out, the client retries, and the customer pays twice.",
		Story:   "Orders sometimes take longer than the client's patience. The client retries, but the first attempt keeps running and also completes.",
		Diagram: charge(false), Solution: charge(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxDuplicates: i(0), MaxP95Ms: f(1500), MaxCostPerHour: f(1.2)},
		DurationSec: 40, WarmupSec: 4, Speed: 1,
		Hints: [3]string{
			"A timeout does not cancel the work. Both attempts may commit.",
			"The server needs a way to recognise a retry of the same request.",
			"Enable idempotency keys on the app server.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "What does an idempotency key guarantee?", Options: []string{"The write is applied at most once per key", "The write is faster", "The client never retries"}, Answer: 0,
			Explain: "The server remembers the key and returns the earlier outcome instead of applying the write again."}},
		Debrief: "Retries are only safe for idempotent operations. Make writes idempotent with a client-supplied key, then retry freely.",
	})

	// 9 ─ Hard ────────────────────────────────────────────────────────────────────────
	vendor := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 30.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("ext", engine.KExternalAPI, cols[4], rowTop, P{"latencyMs": 400.0, "errorPct": 1.0}).label("ext", "Email vendor").
			n("db", engine.KDatabase, cols[4], rowBot, nil).
			e("users", "lb")
		for k := 0; k < 2; k++ {
			id := "app" + string(rune('1'+k))
			b.n(id, engine.KAppServer, cols[2], rowTop+float64(k)*100, P{"threads": 8.0}).e("lb", id).e(id, "db")
			if !fixed {
				b.e(id, "ext")
			}
		}
		if fixed {
			b.n("q", engine.KQueue, cols[3], rowTop, nil).n("w", engine.KWorker, cols[3], rowMid, P{"concurrency": 40.0}).
				e("app1", "q").e("app2", "q").e("q", "w").e("w", "ext").e("w", "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "vendor-wait", Title: "Waiting on a vendor", Difficulty: "Hard",
		Summary: "A slow third-party API holds every thread hostage.",
		Story:   "Every write calls an email vendor that takes almost half a second. Threads pile up waiting, and even reads, which never touch the vendor, are starved.",
		Diagram: vendor(false), Solution: vendor(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(400), MinJobsCompleted: f(0.99), MaxJobP95Ms: f(20000), MaxCostPerHour: f(1.4)},
		DurationSec: 50, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The user does not need to wait for the email to be sent.",
			"Decouple the slow call: acknowledge the request now, do the vendor work later.",
			"Move the vendor call off the request path: app servers → queue → worker → vendor.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "If the vendor's latency doubles, what happens to the user once the vendor call is behind a queue?", Options: []string{"User latency doubles", "User latency is unchanged; the backlog grows", "Requests fail"}, Answer: 1,
			Explain: "The queue absorbs the slowness. Users get a quick acknowledgement while workers fall behind."}},
		Debrief: "Anything slow, unreliable or non-essential for the response belongs behind a queue. You trade immediacy for resilience and must watch the backlog.",
	})

	// 10 ─ Medium ─────────────────────────────────────────────────────────────────────
	spike := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 24.0}).
			n("db", engine.KDatabase, cols[4], rowMid, P{"writeMs": 40.0, "connections": 10.0}).
			e("users", "app")
		if fixed {
			b.n("q", engine.KQueue, cols[2], rowMid, nil).n("w", engine.KWorker, cols[3], rowMid, P{"concurrency": 16.0}).
				e("app", "q").e("q", "w").e("w", "db")
		} else {
			b.e("app", "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "write-flood", Title: "Write flood", Difficulty: "Medium",
		Summary: "A burst of writes outruns the database.",
		Story:   "Writes arrive at 300 per second for a short burst, but the database can only commit about 250 per second.",
		Diagram: spike(false), Solution: spike(true),
		Traffic:     map[string][]engine.TrafficPoint{"users": {{0, 100}, {10, 100}, {12, 300}, {32, 300}, {34, 100}, {50, 100}}},
		Goals:       engine.Goals{MinSuccessRate: 0.99, MinJobsCompleted: f(0.99), MaxJobP95Ms: f(40000), MaxCostPerHour: f(1.2)},
		DurationSec: 50, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The burst is temporary. Could the writes wait in line instead of hitting the database all at once?",
			"A queue plus workers lets the database work at its own pace.",
			"Insert a message queue and a worker between the app server and the database.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "What do you lose by putting a queue in front of the database?", Options: []string{"Nothing", "Immediate consistency: data lands a little later", "The ability to scale"}, Answer: 1,
			Explain: "The write is acknowledged before it is applied, so readers may briefly not see it."}},
		Debrief: "A queue is a shock absorber: it converts a rate spike into a delay. It must be big enough for the spike and workers must eventually catch up.",
	})

	// 11 ─ Medium ─────────────────────────────────────────────────────────────────────
	crash := func(servers int) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 350.0, "readPct": 90.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, P{"healthCheckSec": 2.0}).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "lb")
		for k := 0; k < servers; k++ {
			id := "app" + string(rune('1'+k))
			b.n(id, engine.KAppServer, cols[2], rowTop+float64(k)*90, nil).e("lb", id).e(id, "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "server-down", Title: "A server falls over", Difficulty: "Medium",
		Summary: "Pull the plug on one app server in the middle of peak traffic.",
		Story:   "At twenty seconds one app server crashes for twenty-five seconds. The survivors have to carry everything.",
		Diagram: crash(2), Solution: crash(4),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "random:appServer", AtSec: 20, DownSec: 25}},
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(800), MaxCostPerHour: f(1)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"After a crash, the remaining servers must handle all the traffic. Can they?",
			"Capacity planning is 'N+1': survive losing one node.",
			"Run enough servers that the survivors can carry 350 rps, for example four 8-thread servers.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Why do a few requests still fail even in a well-sized system?", Options: []string{"The balancer takes a moment to notice the failure", "The database is slow", "Caches are cold"}, Answer: 0,
			Explain: "Health checks run periodically, so for a couple of seconds the balancer still sends traffic to a dead server."}},
		Debrief: "Redundancy only helps if the survivors have headroom. Shorter health-check intervals shrink the blip, at the price of more checking.",
	})

	// 12 ─ Hard ───────────────────────────────────────────────────────────────────────
	board := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 300.0, "readPct": 80.0, "workload": "leaderboard", "keySpace": 1000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "lb")
		if fixed {
			b.n("redis", engine.KRedis, cols[3], rowTop, nil)
		}
		for k := 0; k < 3; k++ {
			id := "app" + string(rune('1'+k))
			b.n(id, engine.KAppServer, cols[2], rowTop+float64(k)*100, nil).e("lb", id).e(id, "db")
			if fixed {
				b.e(id, "redis")
			}
		}
		return b.done()
	}
	register(&Scenario{
		ID: "live-scoreboard", Title: "Live scoreboard", Difficulty: "Hard",
		Summary: "Ranking millions of scores with a SQL query is slow.",
		Story:   "Every rank lookup sorts scores in the database and costs ten times a normal read. The database is the bottleneck.",
		Diagram: board(false), Solution: board(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(200), MaxCostPerHour: f(1.2)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Ranking is an ordered-set problem. Is a relational database the right tool?",
			"An in-memory store with sorted sets answers rank queries in microseconds.",
			"Add Redis and connect every app server to it.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Redis runs commands on one thread. Is that a problem at 300 requests per second?", Options: []string{"Yes, it will queue", "No, each command takes microseconds", "Only for writes"}, Answer: 1,
			Explain: "Cheap commands finish in about a tenth of a millisecond, so one thread handles thousands per second."}},
		Debrief: "Use the data structure that matches the question. Sorted sets make 'who is in the top 10' cheap where a relational sort is not.",
	})

	// 13 ─ Hard, config only ──────────────────────────────────────────────────────────
	drivers := func(indexed bool) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 120.0, "readPct": 85.0, "workload": "geo", "keySpace": 5000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("app1", engine.KAppServer, cols[2], rowTop, nil).
			n("app2", engine.KAppServer, cols[2], rowBot, nil).
			n("db", engine.KDatabase, cols[3], rowMid, P{"spatialIndex": indexed}).
			e("users", "lb").e("lb", "app1").e("lb", "app2").e("app1", "db").e("app2", "db").done()
	}
	register(&Scenario{
		ID: "nearby-drivers", Title: "Nearby drivers", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Finding the closest drivers scans the whole table.",
		Story:   "Riders ask for drivers near their location. Without the right index, every search reads the entire table. The architecture is fixed.",
		Diagram: drivers(false), Solution: drivers(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(250), MaxCostPerHour: f(1.2)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at what the database can do differently for location queries.",
			"A spatial index lets the database jump to the neighbourhood instead of scanning.",
			"Turn on the spatial index in the database settings.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A spatial index mainly improves…", Options: []string{"Write speed", "Proximity searches", "Network latency"}, Answer: 1,
			Explain: "Spatial indexes organise points by location so a proximity query touches only nearby entries."}},
		Debrief: "The cheapest scaling fix is often a better index. Measure first: the database was doing 25× the work for every search.",
	})
}
