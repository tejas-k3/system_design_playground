package scenarios

import (
	"fmt"

	"sysdesign/internal/engine"
)

func init() {
	// 1 ─ Tutorial ───────────────────────────────────────────────────────────────────
	register(&Scenario{
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
		return b.servers(servers, cols[2], rowTop, 100, P{"threads": 8.0}).done()
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
		return b.servers(6, cols[2], 20, 70, P{"threads": 24.0, "dbPool": pool, "queueCap": 200.0}).done()
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
		return b.servers(3, cols[2], rowTop, 100, app).done()
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
		Goals:       engine.Goals{MinSuccessRate: 0.98, MaxConflicts: i(0), MaxP95Ms: f(500), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.05)},
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
	register(&Scenario{
		ID: "disappearing-money", Title: "The disappearing money", Difficulty: "Medium",
		Summary: "Two transfers touch the same account at once and one of them vanishes.",
		Story:   "A payments team moves money between 60 accounts. Every transfer succeeds, yet the books are short: transfers that overlap on an account read the same balance and the second write overwrites the first.",
		Diagram: transfers(false), Solution: transfers(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxLostUpdates: i(0), MaxP95Ms: f(600), MaxCostPerHour: f(1.2)},
		DurationSec: 30, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Nothing is failing, so look at what the database promises about overlapping transactions.",
			"The database has an isolation level. The default lets overlapping read-modify-write transfers overwrite each other.",
			"Raise the isolation level on the database to serializable and watch what it costs under contention.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Moving to a stronger isolation level will…", Options: []string{"Never change throughput", "Make overlapping transfers wait for each other or abort", "Make every read slower"}, Answer: 1,
			Explain: "Stronger isolation turns silent overwrites into waiting (locks) or errors (aborts and deadlocks): correctness is paid for with latency under contention."}},
		Debrief: "Weak isolation is fast and silently wrong. Stronger isolation is right but makes contending transactions wait, abort or deadlock, and the cost grows with how hot the shared rows are.",
	})

	// 9 ─ Hard ────────────────────────────────────────────────────────────────────────
	vendor := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 30.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("ext", engine.KExternalAPI, cols[4], rowTop, P{"latencyMs": 400.0, "errorPct": 1.0}).label("ext", "Email vendor").
			n("db", engine.KDatabase, cols[4], rowBot, nil).
			e("users", "lb")
		if fixed {
			b.servers(2, cols[2], rowTop, 100, P{"threads": 8.0})
		} else {
			b.servers(2, cols[2], rowTop, 100, P{"threads": 8.0}, "ext")
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
		return b.servers(servers, cols[2], rowTop, 90, nil).done()
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
		if fixed {
			b.servers(3, cols[2], rowTop, 100, nil, "redis")
		} else {
			b.servers(3, cols[2], rowTop, 100, nil)
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

	// 14 ─ Hard, config only ──────────────────────────────────────────────────────────
	durable := func(persistence string) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 300.0, "readPct": 20.0, "workload": "leaderboard", "keySpace": 1000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowBot, nil).
			n("redis", engine.KRedis, cols[3], rowTop, P{"persistence": persistence}).
			e("users", "lb").servers(3, cols[2], rowTop, 100, nil, "redis").done()
	}
	register(&Scenario{
		ID: "scoreboard-survives", Title: "Scoreboard survives a crash", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Redis holds every score in memory. Pull the plug and watch them vanish.",
		Story:   "Players submit scores all game long and Redis keeps them. At 25 seconds Redis crashes and comes back three seconds later. How many scores did you lose? The architecture is fixed: fix it with configuration.",
		Diagram: durable("none"), Solution: durable("aof"),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "redis", AtSec: 25, DownSec: 3}},
		Goals:       engine.Goals{MinSuccessRate: 0.9, MaxP95Ms: f(300), MaxLostWrites: i(400)},
		DurationSec: 50, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"A crash empties anything that only lives in memory. What does Redis offer to keep a copy on disk?",
			"Open the Redis settings and look at what it says about persistence.",
			"Switch Redis persistence to the append-only file: a crash then loses at most the last second of writes.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "With an append-only file, what is the most you can lose in a crash?", Options: []string{"Nothing at all", "About the last second of writes", "Everything since the last backup"}, Answer: 1,
			Explain: "The file is flushed about once a second, so writes acknowledged since the last flush can still be lost."}},
		Debrief: "Memory is fast and volatile. Persistence trades a little speed for a bounded loss window: choose how many seconds of data you can afford to lose, then pick the setting that guarantees it.",
	})

	// 14b ─ Hard, config only: the primary database dies ───────────────────────────────
	failover := func(db, replica engine.Params) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 40.0, "keyDist": "uniform", "keySpace": 20000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowTop, db).
			n("replica", engine.KReplica, cols[4], rowBot, replica).
			e("users", "lb").servers(2, cols[2], rowTop, 100, P{"threads": 16.0}, "replica").done()
	}
	register(&Scenario{
		ID: "primary-dies", Title: "The primary dies", Difficulty: "Hard", ConfigOnly: true,
		Summary:     "The database server is gone for a minute. A replica is sitting right there.",
		Story:       "Six in ten requests write to the database. At 20 seconds the primary crashes and stays down for a minute. There is a read replica, three seconds behind. Keep most requests succeeding and lose as few acknowledged writes as you can. The architecture is fixed: fix it with configuration.",
		Diagram:     failover(P{}, P{"replicationLagMs": 3000.0}),
		Solution:    failover(P{"onFailure": "promote", "detectSec": 3.0, "promoteSec": 5.0}, P{"replicationLagMs": 500.0}),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "db", AtSec: 20, DownSec: 60}},
		Goals:       engine.Goals{MinSuccessRate: 0.85, MaxP95Ms: f(800), MaxLostWrites: i(100)},
		DurationSec: 70, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"For a minute every write fails. Is there another machine that already has most of the data?",
			"The database can promote a replica when it fails. Look at its 'If it fails' setting, and at how long it takes to notice and to switch.",
			"Promote a replica, shorten the detection and promotion times, and shrink the replica's replication lag: whatever it had not copied yet is lost.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "The replica is 3 seconds behind and gets promoted. What happens to the writes from the last 3 seconds before the crash?", Options: []string{"They are safe: the primary saved them", "They are lost: the replica never received them", "They are replayed when the old primary returns"}, Answer: 1,
			Explain: "Asynchronous replication acknowledges a write before the replica has it, so a failover keeps only what had already been copied."}},
		Debrief: "Failover has two costs. Time: while the failure is noticed and a replica takes over, nothing can write. Data: with asynchronous replication, whatever the replica had not yet copied is gone. Both are settings you choose: faster detection risks false alarms, and a smaller lag costs the primary more work.",
	})

	// 14c ─ Hard, config only: the traffic ramps and the fleet is fixed ─────────────────────
	elastic := func(app engine.Params) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 80.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "lb").servers(1, cols[2], rowMid, 100, app).done()
	}
	register(&Scenario{
		ID: "morning-rush", Title: "Morning rush", Difficulty: "Hard", ConfigOnly: true,
		Summary:     "Traffic climbs to seven times its normal level, then falls. One server does not follow.",
		Story:       "Over a minute traffic climbs from 80 to 600 requests per second, stays there for 40 seconds, then drops. You run a single app server. A new server takes 20 seconds to boot, and every running server costs money, so buying a huge fleet for the whole day is not the answer. The architecture is fixed: fix it with configuration.",
		Diagram:     elastic(P{"threads": 8.0, "scaleUpDelaySec": 20.0}),
		Solution:    elastic(P{"threads": 8.0, "scaleUpDelaySec": 20.0, "autoscale": true, "maxInstances": 4.0, "targetUtilPct": 40.0}),
		Traffic:     map[string][]engine.TrafficPoint{"users": {{0, 80}, {10, 80}, {70, 600}, {110, 600}, {130, 100}, {150, 100}}},
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(600), MaxCostPerHour: f(0.85)},
		DurationSec: 150, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"One server has a fixed number of threads. What if the number of servers could follow the traffic?",
			"Open the App Server's settings, choose 'Show advanced' and look at the Scaling group.",
			"Turn on autoscaling, then lower the target busy level: new servers take 20 seconds to boot, so they must be ordered while the old ones still have room.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A new server takes 20 seconds to boot and traffic climbs for a minute. When should the scaler start adding servers?", Options: []string{"As late as possible, to save money", "Early, while the existing servers still have spare room", "Only once requests start failing"}, Answer: 1,
			Explain: "Capacity ordered now only arrives in 20 seconds. A scaler that waits until the servers are full has already started failing requests by the time help arrives."}},
		Debrief: "Autoscaling trades money for time. A low target busy level orders servers early and costs more; a high one is cheap and late. Boot time sets how far ahead you must look, and a wait before removing servers stops the fleet flapping. The very large fixed fleet works too, but costs all day what you only need at the peak.",
	})

	// 14d ─ Medium, config only: the database is cut off (a partition, not a crash) ─────────
	cutOff := func(app engine.Params) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 120.0, "readPct": 60.0, "keySpace": 2000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("cache", engine.KCache, cols[3], rowTop, P{"capacityItems": 2000.0}).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "lb").servers(2, cols[2], rowTop, 100, app, "cache").e("cache", "db").done()
	}
	register(&Scenario{
		ID: "cut-off", Title: "Cut off", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The database does not crash. It just stops answering.",
		Story:   "At 20 seconds the network around the database goes quiet for 25 seconds. Nothing is refused: requests to it simply never come back. Most reads are served from the cache, but your servers keep waiting on the database for everything else. Keep as many requests succeeding as you can. The architecture is fixed: fix it with configuration.",
		Diagram: cutOff(P{"threads": 16.0}), Solution: cutOff(P{"threads": 16.0, "callTimeoutMs": 300.0, "circuitBreaker": true}),
		Chaos:       []engine.ChaosEvent{{Type: "partition", NodeID: "db", AtSec: 20, DurationSec: 25}},
		Goals:       engine.Goals{MinSuccessRate: 0.82, MaxP95Ms: f(1000)},
		DurationSec: 80, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"A crashed database refuses connections at once. A cut-off one never answers. What does a server thread do while it waits?",
			"Every waiting request holds a thread. Look at the app server's 'Call timeout': zero means wait forever.",
			"Give calls to the database a timeout of a few hundred milliseconds so threads come back, and turn on the circuit breaker so servers stop trying while the database is unreachable.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A server calls a database that is cut off, and 'Call timeout' is 0 (wait forever). What happens to the server?", Options: []string{"Its threads fail fast and are reused", "Its threads all end up stuck waiting, so it stops answering even requests that never needed the database", "The load balancer notices and takes over"}, Answer: 1,
			Explain: "Each waiting request holds a thread for good. Within a second or two none are left, and cache hits queue behind them."}},
		Debrief: "Silence is worse than an error. A crashed dependency refuses quickly, but a partitioned one makes callers wait, and waiting callers use up threads until the healthy parts of the system starve too. Timeouts turn silence into a fast failure, and a circuit breaker stops you spending a thread on every attempt.",
	})

	// 14e ─ Medium, config only: one server keeps stalling (a gray failure that pulses) ────
	flaky := func(lb engine.Params) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 150.0, "readPct": 100.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, lb).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "lb").servers(3, cols[2], rowTop, 100, P{"threads": 8.0}).done()
	}
	register(&Scenario{
		ID: "flaky-neighbour", Title: "Flaky neighbour", Difficulty: "Medium", ConfigOnly: true,
		Summary: "One of three servers keeps stalling, then recovering, then stalling again.",
		Story:   "From 15 seconds on, one of your three app servers alternates between normal and fifteen times slower every four seconds (a noisy neighbour, a garbage-collection storm). It never goes down, so the balancer keeps sending it a third of the traffic. Keep users happy. The architecture is fixed: fix it with configuration.",
		Diagram: flaky(nil), Solution: flaky(P{"outlierEjection": true}),
		Chaos:       []engine.ChaosEvent{{Type: "slow", NodeID: "app2", AtSec: 15, Factor: 15, DurationSec: 120, PulseSec: 4}},
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(500)},
		DurationSec: 120, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The slow server is not down, so health checks pass. What else tells you a server is bad?",
			"Open the load balancer's settings and look at the health group.",
			"Turn on 'Eject outliers' (servers much slower than their peers leave rotation for a while), or switch to least connections: a stalling server piles up unfinished requests, so it is given fewer new ones.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A server is slow half the time but never fails a health check. Which balancing rule protects users best?", Options: []string{"Round robin: everyone gets a fair share", "Dropping servers whose latency is far above their peers'", "Sending more traffic to the slow one so it catches up"}, Answer: 1,
			Explain: "A slow server is worse than a dead one: a dead one is skipped, a slow one keeps being given work it cannot finish."}},
		Debrief: "Gray failures pass the checks that catch dead servers. Comparing each server with its peers (latency and errors) is what catches the ones that are merely sick, and least-connections balancing does it implicitly, because a sick server is the one with the most unfinished work.",
	})

	// 14f ─ Medium, config only: every cache entry expires at the same instant ────────────
	stampede := func(cache engine.Params) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 400.0, "readPct": 97.0, "consistency": "bounded", "maxStaleMs": 15000.0, "keyDist": "zipf", "keySpace": 1000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("cache", engine.KCache, cols[3], rowTop, cache).
			n("db", engine.KDatabase, cols[4], rowMid, P{"connections": 8.0, "readMs": 100.0}).
			e("users", "lb").servers(2, cols[2], rowTop, 100, P{"threads": 32.0, "queueCap": 500.0}, "cache").e("cache", "db").done()
	}
	register(&Scenario{
		ID: "cache-stampede", Title: "Everything expires at once", Difficulty: "Medium", ConfigOnly: true,
		Summary:     "The cache starts full, every entry lives exactly ten seconds, and then they all die together.",
		Story:       "You warmed the cache before launch, so the database sees almost nothing. Entries live ten seconds, because prices change and users must not see data older than about fifteen. At the first expiry the whole popular set misses at the same moment and four hundred reads a second land on a database that can answer eighty. Keep it standing without letting data go stale. The architecture is fixed: fix it with configuration.",
		Diagram:     stampede(P{"capacityItems": 2000.0, "ttlSec": 10.0, "startWarm": true}),
		Solution:    stampede(P{"capacityItems": 2000.0, "ttlSec": 10.0, "startWarm": true, "ttlJitterPct": 50.0, "coalesce": true}),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(500), MaxStaleReadPct: f(0.12)},
		DurationSec: 50, WarmupSec: 3, Speed: 1,
		Hints: [3]string{
			"Every entry was created at the same moment with the same lifetime. What happens when the lifetime ends?",
			"Open the cache's settings: look at Freshness and at 'Show advanced'. You need the entries to stop expiring together, and concurrent misses for one item to stop piling onto the database.",
			"Add TTL jitter so entries expire at different times, and turn on 'Share concurrent misses'. Do not simply raise the TTL: the data would go stale.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "The cache is warm and the hit rate is near 100%. Why can the database still be overwhelmed after ten seconds?", Options: []string{"The cache fills up and evicts popular items", "Every entry expires at the same instant, so almost every request misses at once", "The database slows down over time"}, Answer: 1,
			Explain: "A warm start gives every entry the same creation time, so they share one expiry time. Misses arrive as a wave, not a trickle."}},
		Debrief: "A cache hides load until the moment it cannot. Synchronised expiry turns a 99% hit rate into a 0% one for a few seconds. Jitter spreads the expiries out, and sharing concurrent misses keeps one popular item from being fetched a hundred times. Making the TTL very long also avoids the stampede, but it trades it for stale data, which is why this challenge limits staleness.",
	})

	// 14g ─ Medium, config only: how much data may a failover lose? ─────────────────────
	replicas := func(db, rep engine.Params) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 80.0, "readPct": 0.0, "keyDist": "uniform", "keySpace": 100000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowTop, db).
			n("replica", engine.KReplica, cols[4], rowBot, rep).
			e("users", "lb").servers(2, cols[2], rowTop, 100, P{"threads": 16.0}, "replica").done()
	}
	promote := P{"onFailure": "promote", "detectSec": 5.0, "promoteSec": 10.0}
	register(&Scenario{
		ID: "sync-or-async", Title: "Sync or async?", Difficulty: "Medium", ConfigOnly: true,
		Summary:     "The primary dies. How many acknowledged writes does the replica know about?",
		Story:       "Every request is a write that users were told succeeded. At 20 seconds the primary database dies and the replica is promoted. The replica is three seconds behind, so it never saw the last three seconds of writes. Lose at most twenty acknowledged writes and keep the slowest ten percent of writes under about a quarter of a second. The architecture is fixed: fix it with configuration.",
		Diagram:     replicas(promote, P{"replicationLagMs": 3000.0}),
		Solution:    replicas(promote, P{"replicationLagMs": 3000.0, "replication": "sync"}),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "db", AtSec: 20, DownSec: 300}},
		Goals:       engine.Goals{MinSuccessRate: 0.70, MaxP95Ms: f(260), MaxLostWrites: i(20), StarP95Ms: f(250)},
		DurationSec: 65, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"A write was acknowledged to the user, then the primary died. Where else did that write exist at that moment?",
			"Open the replica's settings and read the Replication help text: who waits for whom?",
			"Switch the replica to synchronous replication: the primary only acknowledges once the replica has the write, so nothing acknowledged is lost. Check what it does to write latency.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "What is the price of synchronous replication?", Options: []string{"Nothing: it is strictly better", "Every write also waits for the replica, and writes stop if the replica is down", "Reads become stale"}, Answer: 1,
			Explain: "Synchronous replication buys zero data loss with latency, and with a dependency: the primary cannot acknowledge what its replica has not confirmed."}},
		Debrief: "Replication lag is your data-loss window in a failover. Synchronous replication closes it, at the cost of slower writes and a new way to be unavailable (a dead replica blocks writes). Shrinking the lag also helps; in this simulator lag is free to configure, but in a real system a tiny lag costs primary throughput or is simply unachievable across regions.",
	})

	// 14h ─ Hard, config only: the old primary comes back ────────────────────────────────
	register(&Scenario{
		ID: "fence-the-old-primary", Title: "Fence the old primary", Difficulty: "Hard", ConfigOnly: true,
		Summary:     "The failover worked. Then the dead primary woke up.",
		Story:       "The primary dies at 10 seconds and a replica is promoted by 25 seconds. At 50 seconds the old primary restarts. Nobody told it that it was replaced. Keep the data: lose at most a hundred acknowledged writes, and keep most requests succeeding. The architecture is fixed: fix it with configuration.",
		Diagram:     replicas(P{"onFailure": "promote", "detectSec": 5.0, "promoteSec": 10.0, "fencing": false}, P{"replicationLagMs": 500.0}),
		Solution:    replicas(promote, P{"replicationLagMs": 500.0}),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 40}},
		Goals:       engine.Goals{MinSuccessRate: 0.68, MaxLostWrites: i(100)},
		DurationSec: 65, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The new primary has been taking writes for 25 seconds when the old one comes back. Who do clients talk to then?",
			"Look at the database's Failover settings: there is one that decides whether a recovered old primary is allowed back in.",
			"Turn on 'Fence the old primary'. Without it the old primary rejoins as primary and everything written to the replacement since promotion is lost. Do not just disable failover: writes would fail for the whole outage.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "The old primary restarts and nothing stops it. What happens to writes made to the new primary in the meantime?", Options: []string{"They are merged into the old primary", "They exist only on the new primary, and clients go back to the old one: they are lost", "Nothing, both stay in sync"}, Answer: 1,
			Explain: "Two nodes that both believe they are the primary diverge. Without fencing, the survivor of the confusion silently drops the other's writes."}},
		Debrief: "Failover is only half the job: you also have to make sure the old primary cannot come back as one. Fencing (a token, a lease, or simply keeping the node out until it is reconciled) is what prevents split brain. Staying down avoids the problem but costs the whole outage.",
	})

	// 14i ─ Medium, config only: demand swings faster than a new server boots ─────────────
	var waves []engine.TrafficPoint
	waves = append(waves, engine.TrafficPoint{0, 40})
	for k := 0; k < 6; k++ {
		at := float64(20 + 40*k)
		waves = append(waves, engine.TrafficPoint{at, 40}, engine.TrafficPoint{at + 2, 180}, engine.TrafficPoint{at + 22, 180}, engine.TrafficPoint{at + 24, 40})
	}
	waveFleet := func(app engine.Params) engine.Diagram {
		base := P{"threads": 4.0, "cpuMs": 40.0, "queueCap": 100.0, "autoscale": true, "maxInstances": 8.0, "scaleUpDelaySec": 10.0, "scaleDownDelaySec": 0.0, "targetUtilPct": 60.0}
		for k, v := range app {
			base[k] = v
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 40.0, "readPct": 100.0, "timeoutMs": 1500.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "lb").servers(1, cols[2], rowMid, 100, base).done()
	}
	register(&Scenario{
		ID: "wave-rider", Title: "Waves faster than boot time", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Autoscaling is on, and users still get errors at the start of every wave.",
		Story:   "Every forty seconds traffic jumps from 40 to 180 requests a second for twenty seconds and drops again. Autoscaling is switched on, yet each wave starts with a burst of failures: a new server takes ten seconds to boot, and the scaler only looks at the last thirty seconds of load. Keep success high and the slowest requests fast without buying a big fleet for the whole run. The architecture is fixed: fix it with configuration.",
		Diagram: waveFleet(nil), Solution: waveFleet(P{"minInstances": 3.0}),
		Traffic:     map[string][]engine.TrafficPoint{"users": waves},
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(500), MaxCostPerHour: f(0.5)},
		DurationSec: 260, WarmupSec: 5, Speed: 4,
		Hints: [3]string{
			"Look at when the failures happen: at the start of each wave. How much capacity is running at that moment, and how long until more arrives?",
			"Open the app server's settings, 'Show advanced', Scaling group. Waiting longer before removing servers is not the whole story: think about the floor and about when the scaler decides to act.",
			"Keep a floor of servers that covers the wave (minimum servers 3), or make the scaler act earlier with a lower target busy level. Compare the cost of each.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A wave arrives every 40 seconds and a server takes 10 seconds to boot. Which setting helps most?", Options: []string{"Remove idle servers sooner, to save money", "Keep a floor of servers (or a lower target), because help arrives after the damage is done", "A longer boot time"}, Answer: 1,
			Explain: "Reactive scaling always lags demand by the measuring window plus the boot time. When waves are shorter than that lag, only capacity that is already running helps."}},
		Debrief: "Autoscaling is reactive, so it suits changes slower than its own reaction time. For predictable, fast waves, pre-provision a floor (or scale on a schedule); a lower target busy level buys earlier action at a higher cost. Waiting longer before scaling down avoids flapping but would not have fixed the wave start.",
	})

	// 14j ─ Medium, config only: the load test passed, the launch did not ─────────────────
	loadTest := func(app engine.Params) engine.Diagram {
		return (&bld{}).
			n("staff", engine.KClient, cols[0], rowTop, P{"arrival": "closed", "users": 40.0, "thinkMs": 1000.0, "readPct": 100.0, "timeoutMs": 5000.0}).
			n("public", engine.KClient, cols[0], rowBot, P{"rps": 80.0, "readPct": 100.0, "timeoutMs": 2000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("staff", "lb").e("public", "lb").servers(1, cols[2], rowMid, 100, app).done()
	}
	register(&Scenario{
		ID: "passed-the-load-test", Title: "It passed the load test", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Forty staff tested the system and everything was fine. Then the public arrived.",
		Story:   "Your team load-tested with forty staff clicking around, each waiting for an answer before the next click: no errors, fast pages. The public launch brings eighty requests a second from people who do not wait for anyone. Both crowds hit the same server. Make the launch survive. The architecture is fixed: fix it with configuration.",
		Diagram: loadTest(P{"threads": 4.0, "cpuMs": 40.0, "queueCap": 100.0}), Solution: loadTest(P{"threads": 8.0, "cpuMs": 40.0, "queueCap": 100.0}),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(500), MaxCostPerHour: f(0.6)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Why did the test pass? Think about what the staff do when the server gets slow.",
			"Staff wait for each answer, so a slow server slows them down. The public keeps arriving regardless. Compare what each crowd does to the queue.",
			"The public crowd is open-loop: its load does not fall when the server struggles. Give the server the capacity for it: look at the app server's threads.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Why can a closed-loop load test pass at a load that breaks the system in production?", Options: []string{"Test machines are faster", "Users who wait for answers slow themselves down as the system slows, so the queue never explodes", "Production has more data"}, Answer: 1,
			Explain: "A fixed crowd that waits is self-limiting: latency rises, arrivals fall. Real internet traffic keeps coming at the same rate however slow you are."}},
		Debrief: "Closed-loop tests flatter a system: the offered load shrinks as latency grows (and they hide latency by not measuring the requests that were never sent). Real traffic is mostly open-loop, so size for the arrival rate, not for the number of testers.",
	})

	// 14k ─ Medium, config only: short intense rushes at a modest average ───────────────────
	rush := func(app engine.Params) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"arrival": "bursty", "rps": 90.0, "burstFactor": 3.0, "burstSec": 1.5, "readPct": 100.0, "timeoutMs": 3000.0}).
			n("limiter", engine.KRateLimiter, cols[1], rowMid, P{"limitRps": 1000.0, "burst": 1000.0}).
			n("lb", engine.KLoadBalancer, cols[2], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "limiter").e("limiter", "lb").servers(1, cols[3], rowMid, 100, app).done()
	}
	register(&Scenario{
		ID: "flash-crowd", Title: "Short, sharp rushes", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The average load is fine. The load a second at a time is not.",
		Story:   "Traffic averages 90 requests a second, comfortably inside what one server can do. But it arrives in rushes: for a second or two the rate triples, then it eases off. Your server's queue only holds ten requests, so every rush drops hundreds. A rate limiter sits in front of the server. Keep requests succeeding and the slowest ones under 0.7 s, on a budget of 45 cents an hour. The architecture is fixed: fix it with configuration.",
		Diagram: rush(P{"threads": 8.0, "cpuMs": 20.0, "queueCap": 10.0}), Solution: rush(P{"threads": 10.0, "cpuMs": 20.0, "queueCap": 100.0}),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(700), MaxCostPerHour: f(0.45), StarCostPerHour: f(0.42), StarP95Ms: f(500)},
		DurationSec: 90, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Average load and peak load are different numbers. What does the server do during a rush, and what happens to the requests that arrive while all threads are busy?",
			"A queue turns a spike into a delay, as long as it is big enough and the wait stays acceptable. The rate limiter rejects traffic: a rejected request is still a failed request.",
			"Make the queue big enough to hold a rush (about a hundred) and give the server a few more threads so the queue drains quickly. A huge queue alone pushes the slowest requests past 0.7 s; sixteen threads cost too much.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Average load is 70% of capacity. Why do users still see errors?", Options: []string{"The average is wrong", "Traffic comes in rushes well above capacity, and a short queue overflows during them", "The database is slow"}, Answer: 1,
			Explain: "Utilisation is an average over time; failures happen in the moments when arrivals exceed capacity. Queues and headroom absorb those moments."}},
		Debrief: "Provision for the burst, not the mean. A queue absorbs a burst only if it is large enough and drains fast enough; it trades errors for waiting time. Rate limiting protects a system from overload but does not make rejected users succeed, so it is the wrong tool when the burst is short enough to wait out.",
	})

	// 14l ─ Medium: a hot key overloads one shard; more shards do not help ────────────────
	hotShard := func(shards int, cache bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 700.0, "readPct": 90.0, "keySpace": 100000.0, "hotKeyPct": 35.0, "timeoutMs": 2000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("router", engine.KShardRouter, cols[3], rowMid, nil)
		for k := 0; k < shards; k++ {
			b.n(fmt.Sprintf("db%d", k+1), engine.KDatabase, cols[4], rowTop+float64(k)*60, P{"connections": 8.0, "readMs": 30.0, "writeMs": 40.0}).e("router", fmt.Sprintf("db%d", k+1))
		}
		b.e("users", "lb")
		if cache {
			b.n("cache", engine.KCache, cols[3], rowTop, P{"capacityItems": 1000.0}).e("cache", "router")
			return b.servers(4, cols[2], 0, 80, P{"threads": 16.0}, "router", "cache").done()
		}
		return b.servers(4, cols[2], 0, 80, P{"threads": 16.0}, "router").done()
	}
	register(&Scenario{
		ID: "hot-shard", Title: "One shard on fire", Difficulty: "Medium",
		Summary: "The data is split four ways, and one celebrity item gets a third of all reads.",
		Story:   "You sharded the database four ways so no shard sees more than a quarter of the traffic. Then one item went viral: 35% of all requests ask for it, and it lives on exactly one shard, which now takes half the load and falls over while the other three idle. Fix it. You may add and connect components.",
		Diagram: hotShard(4, false), Solution: hotShard(4, true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(500), MaxCostPerHour: f(1.7), StarCostPerHour: f(1.65)},
		DurationSec: 50, WarmupSec: 3, Speed: 1,
		Hints: [3]string{
			"Which shard is failing, and why does adding capacity to the other three not help?",
			"More shards will not spread one item: it still maps to a single shard. What could answer the popular reads without touching that shard at all?",
			"Put a cache in front of the shard router (connect the app servers to it and the cache to the router). A tiny cache is enough: the hot item stays in it.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "One item gets 35% of reads. What does going from 4 shards to 8 do?", Options: []string{"Halves the hot shard's load", "Helps a little, but the hot item still lives on one shard", "Nothing at all"}, Answer: 1,
			Explain: "Resharding spreads the other keys thinner, but a single key cannot be split by hashing: its load stays on whichever shard owns it."}},
		Debrief: "Sharding balances many keys, not one. A hot key needs a different tool: a cache in front, a replica set for that key, or splitting the key (for example, a counter sharded into N sub-counters). Check the load per shard, not the average.",
	})

	// 14m ─ Medium, config only: a scan flushes the useful part of the cache ────────────────
	scanned := func(cache engine.Params) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowTop, P{"rps": 300.0, "readPct": 100.0, "keyDist": "zipf", "keySpace": 500.0, "timeoutMs": 2000.0}).
			n("crawler", engine.KClient, cols[0], rowBot, P{"rps": 150.0, "readPct": 100.0, "keyDist": "scan", "keySpace": 100000.0, "timeoutMs": 2000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("cache", engine.KCache, cols[3], rowTop, cache).
			n("db", engine.KDatabase, cols[4], rowMid, P{"connections": 8.0, "readMs": 40.0}).
			e("users", "lb").e("crawler", "lb").servers(2, cols[2], rowTop, 100, P{"threads": 32.0, "queueCap": 300.0}, "cache").e("cache", "db").done()
	}
	register(&Scenario{
		ID: "scan-pollution", Title: "The crawler that ate the cache", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Real users ask for the same few hundred items. A crawler reads every item exactly once.",
		Story:   "Users keep asking for the same 500 popular items, and the cache holds 600. A search-engine crawler is walking your whole catalogue of 100,000 items, once each, at 150 requests a second. Every item the crawler touches goes into the cache and pushes out something users want. The database can only do 200 reads a second. Keep users fast. The architecture is fixed: fix it with configuration.",
		Diagram: scanned(P{"capacityItems": 600.0, "eviction": "fifo"}), Solution: scanned(P{"capacityItems": 600.0, "eviction": "lfu"}),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(400)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Items the crawler reads once go into the cache and are never read again. What do they push out?",
			"Open the cache's eviction setting. Which policy decides by how recently an item was used, and which by how often?",
			"Switch eviction to LFU (least frequently used): an item read once has a count of one and is the first to go, so the popular items stay. LRU is better than FIFO here but still lets a long scan push things out; a much bigger cache also works but is the expensive answer.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Which eviction policy best protects a small popular set from a one-pass scan?", Options: []string{"FIFO: oldest out first", "LFU: least frequently used out first", "Random"}, Answer: 1,
			Explain: "A scan produces many items with a count of one. LFU evicts those first and keeps the items that are read again and again."}},
		Debrief: "Eviction policy is a bet about the future. LRU assumes recent means useful, which a scan breaks; FIFO ignores use altogether; LFU assumes frequent means useful, which resists scans (but can hold stale favourites). Real systems often add scan-resistant variants or keep crawler traffic out of the cache.",
	})

	// Search over 16 shards: the slowest shard sets the latency. Config only: ask for fewer answers, or ask again.
	searchTail := func(fixed bool) engine.Diagram {
		app := P{"threads": 32.0, "gather": "all"}
		if fixed {
			app = P{"threads": 32.0, "gather": "firstK", "k": 12.0}
		}
		b := (&bld{}).n("users", engine.KClient, cols[0], 200, P{"rps": 60.0, "readPct": 100.0, "timeoutMs": 2000.0}).
			n("app", engine.KAppServer, cols[1], 200, app).e("users", "app")
		for k := 0; k < 16; k++ {
			id := fmt.Sprintf("shard%d", k+1)
			b.n(id, engine.KIndex, cols[2], float64(k)*80, P{"queryMs": 30.0, "variability": "heavy", "connections": 16.0}).label(id, fmt.Sprintf("Search Shard %d", k+1)).e("app", id)
		}
		return b.done()
	}
	register(&Scenario{
		ID: "search-tail", Title: "Search at 16 shards", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Every search waits for the slowest of 16 shards.",
		Story:   "Search is split over 16 shards and the app server asks all of them and waits for every answer. Each shard answers in about 30 ms, but some answers take far longer, and with 16 shards nearly every search meets one of them. The architecture is fixed; tune the fan-out.",
		Diagram: searchTail(false), Solution: searchTail(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(300), MaxCostPerHour: f(1.7), StarCostPerHour: f(1.65), StarP95Ms: f(200)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the latency distribution: the median is fine for one shard, so why is the p95 so far from it?",
			"A request that waits for all 16 answers is as slow as the slowest of 16. Do you really need all of them?",
			"Set the app server's fan-out to answer on the first K shards (try 12 of 16), or hedge: ask a slow shard again. More threads will not help.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Adding more app-server threads will…", Options: []string{"Cut the p95 a lot", "Barely change it: the wait is on the shards, not on threads", "Make the shards faster"}, Answer: 1,
			Explain: "The threads are not the bottleneck: each request is waiting for the slowest of 16 shards. Fewer required answers or a second try attacks that wait."}},
		Debrief: "Waiting for all N answers makes latency the maximum of N draws, so tail latency grows with fan-out. Answering on the first K, hedging slow calls, or allowing partial results trade completeness or extra load for a much shorter tail.",
	})

	// A celebrity's post is pushed into millions of timelines. Config only: stop pushing for the few huge accounts.
	celebrity := func(fixed bool) engine.Diagram {
		app := P{"threads": 32.0, "queueCap": 500.0}
		if fixed {
			app["feedStrategy"] = "hybrid"
			app["celebrityThreshold"] = 2000.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 40.0, "readPct": 70.0, "workload": "feed", "keySpace": 5000.0, "followersMean": 50.0, "followersSkew": 1.2, "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, app).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "queueCap": 500.0}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "celebrity-post", Title: "The celebrity post", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Most posts are fine, but a few accounts make the whole feed stall.",
		Story:   "Every post is copied into each follower's timeline when it is written, so reads are fast. Most accounts have a few dozen followers, but a handful have tens of thousands, and every time one of them posts the database is buried under timeline inserts. The architecture is fixed; change how the feed is built.",
		Diagram: celebrity(false), Solution: celebrity(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(300), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.05), StarP95Ms: f(250)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at the failures and the p95: they come in bursts. Who is posting when it happens?",
			"Pushing a post costs one insert per follower. For an account with 30,000 followers that is 30,000 inserts for a single post.",
			"Set the app server's feed strategy to hybrid: push for ordinary accounts, pull celebrities when a timeline is read. More connections or threads only cost money.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Where does the work go if celebrities are pulled instead of pushed?", Options: []string{"Nowhere, it disappears", "To readers: each timeline read also fetches a few celebrities' posts", "To the author, who waits longer to post"}, Answer: 1,
			Explain: "Fan-out on write spends effort once per follower; fan-out on read spends it once per reader. A hybrid pays the cheap side for each kind of account."}},
		Debrief: "Push makes reads cheap and writes proportional to the audience; pull makes writes cheap and reads proportional to who you follow. With heavy skew a hybrid wins: push for the many small accounts and pull the few giants.",
	})

	// Image downloads billed per GB: every view pays egress unless a CDN answers it
	imageFeed := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 100.0, "respKB": 300.0, "keySpace": 2000.0, "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[2], rowMid, P{"threads": 32.0}).
			n("db", engine.KDatabase, cols[3], rowTop, nil).
			n("files", engine.KObjectStore, cols[3], rowBot, nil).label("files", "Image store").
			e("app", "db").e("app", "files")
		if fixed {
			b.n("cdn", engine.KCDN, cols[1], rowMid, P{"capacityItems": 5000.0, "ttlSec": 300.0}).e("users", "cdn").e("cdn", "app")
		} else {
			b.e("users", "app")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "image-egress", Title: "The image feed bill", Difficulty: "Medium",
		Summary: "Everything works, but the cloud bill is shocking.",
		Story:   "An image feed serves 100 views a second, each a 300 KB picture fetched from the image store, which charges for every GB that leaves it. The pages are fast and nothing fails, but finance wants the hourly bill under $2.50.",
		Diagram: imageFeed(false), Solution: imageFeed(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(2.5), StarCostPerHour: f(1.5)},
		DurationSec: 60, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at the cost, not the latency: 100 views a second x 300 KB is 30 MB every second.",
			"Which component can answer a popular picture without going back to the image store?",
			"Put a CDN in front of the app server and give it room for the popular pictures (there are about 2,000 distinct ones).",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A CDN that can only hold a quarter of the pictures will…", Options: []string{"Cut the bill to nearly nothing", "Cut it somewhat: popular pictures hit, the rest still pay egress", "Make no difference"}, Answer: 1,
			Explain: "Views are skewed, so a small cache catches the popular ones, but every miss still downloads from the store and pays for it."}},
		Debrief: "Egress is billed per byte, so the cheapest download is the one that never reaches the origin. A CDN sized to the popular set turns a dominant cost into a rounding error; one that is too small saves some and still pays for the rest.",
	})

	// Video uploads of 10 MB at 6 a second need 480 Mbit/s on every hop: the link to the store is sized for far less
	videoUpload := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 6.0, "readPct": 0.0, "reqKB": 10000.0, "timeoutMs": 4000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("db", engine.KDatabase, cols[2], rowTop, nil).label("db", "Video catalogue").
			n("files", engine.KObjectStore, cols[2], rowBot, nil).label("files", "Video store").
			e("users", "app").e("app", "db").e("app", "files")
		for k := range b.d.Edges {
			if b.d.Edges[k].ID == "app>files" {
				b.d.Edges[k].BandwidthMbps = 200
				if fixed {
					b.d.Edges[k].BandwidthMbps = 1000
				}
			}
		}
		return b.done()
	}
	register(&Scenario{
		ID: "video-upload", Title: "Video uploads", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Short uploads work, but videos time out.",
		Story:   "Creators upload clips of about 10 MB, six a second at the evening peak. The app server streams each one to the video store over a 200 Mbit/s link. In testing with tiny files everything was fine; at the peak most uploads time out after 4 seconds.",
		Diagram: videoUpload(false), Solution: videoUpload(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(1500), MaxCostPerHour: f(1.3), StarCostPerHour: f(1.1), StarP95Ms: f(1000)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Do the arithmetic: 6 uploads a second x 10 MB is how many Mbit/s? Compare it with the link.",
			"6 x 10 MB x 8 bits = 480 Mbit/s, and the link to the store carries 200. Uploads queue behind each other on it until they time out.",
			"Select the connection from the app server to the video store and raise its bandwidth above what the peak needs (1,000 Mbit/s leaves room).",
		},
		Predictions: []Prediction{{ID: "p1", Question: "More app-server threads will…", Options: []string{"Fix it: more uploads in parallel", "Not help: they all share the same saturated link", "Make the uploads smaller"}, Answer: 1,
			Explain: "Parallel uploads still squeeze through one link. When a link is the limit, adding workers only adds more bytes waiting for it."}},
		Debrief: "Bandwidth is a capacity like any other: bytes per second in must be less than the link can carry. Estimate peak QPS x object size x 8 before choosing the pipe, and keep big bodies off paths that are sized for small ones.",
	})

	// GC storm: long stop-the-world pauses on every app server. Config only: tune the pauses, do not paper over them.
	gcStorm := func(fixed bool) engine.Diagram {
		pause := 1500.0
		if fixed {
			pause = 200.0
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 120.0, "readPct": 90.0, "timeoutMs": 800.0, "retries": 0.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[3], rowMid, nil).
			e("users", "lb")
		for k := 0; k < 3; k++ {
			id := fmt.Sprintf("app%d", k+1)
			b.n(id, engine.KAppServer, cols[2], float64(k)*110+60, P{"threads": 16.0, "gcPauseMs": pause, "gcEverySec": 8.0}).e("lb", id).e(id, "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "gc-storm", Title: "The GC storm", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Averages look fine, yet one request in seven fails.",
		Story:   "Each app server freezes for a second and a half every eight seconds or so (a stop-the-world garbage collection after a heap change). During a pause nothing on that server answers, and the clients give up after 800 ms. The median latency is healthy, which is why nobody noticed.",
		Diagram: gcStorm(false), Solution: gcStorm(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.1), StarP95Ms: f(200)},
		DurationSec: 60, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at the failures: they are timeouts, in waves, and the latency of the requests that do succeed is fine. What freezes a whole server for a second?",
			"The failure breakdown and the events on the chart show GC pauses. Retries or more servers hide some of the damage but cost latency or money.",
			"Open an app server's advanced settings: shorten the GC pause (a smaller heap or a different collector). A pause shorter than the client's patience costs nothing.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Letting clients retry twice will…", Options: []string{"Fix it completely", "Recover most requests but make the slow ones very slow", "Do nothing"}, Answer: 1,
			Explain: "A retry reaches a healthy server, so more requests succeed, but each rescued request has already waited out its timeout: the tail gets worse."}},
		Debrief: "A stop-the-world pause is invisible in averages and brutal at the tail. Retries and extra servers mask it at a cost; the real fix is a shorter pause. Watch p99 and the timeouts, not the mean.",
	})

	// 500 KB uploads into a queue that refuses bodies that big: the claim-check pattern
	uploads := func(fixed bool) engine.Diagram {
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 40.0, "readPct": 0.0, "reqKB": 500.0, "timeoutMs": 3000.0}).
			n("app1", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0}).
			n("q", engine.KQueue, cols[2], rowTop, nil).
			n("w", engine.KWorker, cols[3], rowTop, P{"concurrency": 8.0, "processMs": 40.0}).
			n("db", engine.KDatabase, cols[4], rowTop, nil).
			e("users", "app1").e("app1", "q").e("q", "w").e("w", "db")
		if fixed {
			b.n("files", engine.KObjectStore, cols[2], rowBot, nil).e("app1", "files")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "upload-500kb", Title: "The 500 KB upload", Difficulty: "Medium",
		Summary: "Every upload is refused by the queue. Why?",
		Story:   "Users upload photos of about 500 KB. The app server hands each upload to a message queue, and workers store it. Since the launch, nearly every upload fails and the queue is nearly empty. The queue's limit on message size is set by your provider and cannot be changed.",
		Diagram: uploads(false), Solution: uploads(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(800), MaxCostPerHour: f(0.9)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Open the failure breakdown. What is the reason, and what does the queue's own setting say about message size?",
			"A queue is for small messages (managed queues commonly stop at 256 KB). Where should a 500 KB body live instead?",
			"Add an Object Store and connect the app server to it. The app uploads the body there and queues only a pointer (the claim-check pattern).",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A queue accepts messages up to 256 KB. Users upload 500 KB files. What is the standard fix?", Options: []string{"Raise the queue's size limit", "Retry the failed uploads", "Store the file elsewhere and queue a pointer to it"}, Answer: 2,
			Explain: "Queues and databases are built for small records. Large bodies go to an object store; what travels through the queue is a reference to it."}},
		Debrief: "The claim-check pattern keeps large payloads out of the messaging path: the body is uploaded once to cheap, durable object storage and everyone downstream handles a small pointer. It also makes retries cheap, because a retry re-sends a pointer, not 500 KB.",
	})

	// 15 ─ Easy ────────────────────────────────────────────────────────────────────────
	edge := func(cdn bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 900.0, "readPct": 98.0, "keySpace": 3000.0, "zipfS": 1.1}).
			n("lb", engine.KLoadBalancer, cols[2], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, nil)
		if cdn {
			b.n("cdn", engine.KCDN, cols[1], rowMid, P{"capacityItems": 3000.0}).e("users", "cdn").e("cdn", "lb")
		} else {
			b.e("users", "lb")
		}
		return b.servers(3, cols[3], rowTop, 100, nil).done()
	}
	register(&Scenario{
		ID: "edge-cache", Title: "Serve it from the edge", Difficulty: "Easy",
		Summary: "Nearly everyone asks for the same pages, and every request still reaches your servers.",
		Story:   "A product page goes viral: 900 requests per second, almost all reads of a small set of popular items. Your three servers answer every one themselves and cannot keep up.",
		Diagram: edge(false), Solution: edge(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(250), MaxCostPerHour: f(1.2), StarP95Ms: f(175)},
		DurationSec: 45, WarmupSec: 6, Speed: 1,
		Hints: [3]string{
			"Why should a request for the same popular page travel all the way to your servers?",
			"A CDN keeps copies close to the users and only forwards what it does not have.",
			"Put a CDN between the users and the load balancer.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A CDN with a 60 second TTL serves a page that changes every few seconds. What is the catch?", Options: []string{"Users may see stale copies until the TTL expires", "The page loads slower", "Writes stop working"}, Answer: 0,
			Explain: "Cached copies are served until they expire, so freshness is traded for speed unless writes invalidate them."}},
		Debrief: "The cheapest request is one your servers never see. A CDN absorbs repeated reads at the edge; the price is freshness, which is why caching suits content that changes slowly.",
	})

	// 16 ─ Medium, config only ─────────────────────────────────────────────────────────
	lopsided := func(algorithm string) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 300.0, "readPct": 90.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, P{"algorithm": algorithm}).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "lb").servers(4, cols[2], rowTop, 80, nil).done()
	}
	register(&Scenario{
		ID: "one-slow-server", Title: "One slow server", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Round robin keeps feeding a server that has become eight times slower.",
		Story:   "One of four servers starts running eight times slower (a noisy neighbour, a bad disk). The load balancer still hands it a quarter of the traffic, and users feel it. The architecture is fixed.",
		Diagram: lopsided("roundRobin"), Solution: lopsided("leastConn"),
		Chaos:       []engine.ChaosEvent{{Type: "slow", NodeID: "app2", AtSec: 2, Factor: 8, DurationSec: 200}},
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(400)},
		DurationSec: 50, WarmupSec: 6, Speed: 1,
		Hints: [3]string{
			"Round robin treats every server as equal. Is that still true?",
			"The balancer can send new work to whichever server has the fewest requests in flight, or stop using a server that misbehaves.",
			"Switch the load balancer to least connections, or turn on outlier ejection.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Why does a least-connections balancer help when one server is slow?", Options: []string{"A slow server accumulates open requests, so it is picked less often", "It makes the slow server faster", "It restarts the slow server"}, Answer: 0,
			Explain: "Requests pile up on the slow server, so its in-flight count is high and new requests go elsewhere."}},
		Debrief: "Balancing is not about equal shares, it is about equal outcomes. Least connections adapts to slowness automatically; ejecting outliers goes further and rests a sick server entirely.",
	})

	// 17 ─ Hard ────────────────────────────────────────────────────────────────────────
	sharded := func(shards int) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 700.0, "readPct": 10.0, "keyDist": "uniform", "keySpace": 100000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil)
		db := P{"writeMs": 40.0, "connections": 16.0}
		if shards <= 1 {
			b.n("db", engine.KDatabase, cols[4], rowMid, db).e("users", "lb")
			return b.servers(4, cols[2], 0, 80, P{"threads": 16.0}).done()
		}
		b.n("router", engine.KShardRouter, cols[3], rowMid, nil)
		for k := 0; k < shards; k++ {
			id := fmt.Sprintf("shard%d", k+1)
			b.n(id, engine.KDatabase, cols[4], rowTop+float64(k)*140, db).e("router", id)
		}
		b.e("users", "lb")
		return b.servers(4, cols[2], 0, 80, P{"threads": 16.0}, "router").done()
	}
	register(&Scenario{
		ID: "split-the-database", Title: "Split the database", Difficulty: "Hard",
		Summary: "630 writes a second against a database that can commit about 400.",
		Story:   "A busy write-heavy service has outgrown its single database. Buying a bigger one is not an option, so the data has to be spread over several databases.",
		Diagram: sharded(1), Solution: sharded(2),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(500), MaxCostPerHour: f(2.5)},
		DurationSec: 45, WarmupSec: 6, Speed: 1,
		Hints: [3]string{
			"Count the writes per second and divide by what one database can commit.",
			"Shard the data: a router sends each key to one of several databases.",
			"Put a shard router between the servers and two databases, and connect each server to the router.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A shard router picks the database from the item's key. What goes wrong if a few keys are far hotter than the rest?", Options: []string{"One shard can get most of the traffic", "Nothing, sharding balances load perfectly", "Writes become reads"}, Answer: 0,
			Explain: "Hashing spreads keys, not requests. If a handful of keys dominate, whichever shard owns them becomes the bottleneck again."}},
		Debrief: "Sharding multiplies write capacity by splitting the data, at the price of complexity: queries across shards get hard and skew can recreate the hotspot you were escaping.",
	})

	// 18 ─ Hard capstone ───────────────────────────────────────────────────────────────
	capstone := func(hardened bool) engine.Diagram {
		clients := P{"readPct": 85.0, "keySpace": 8000.0, "timeoutMs": 1500.0, "retries": 2.0, "backoff": "exponential"}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, clients).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowMid, P{"readMs": 15.0})
		b.e("users", "lb")
		if !hardened {
			return b.servers(2, cols[2], rowTop, 120, P{"threads": 8.0}).done()
		}
		b.n("cache", engine.KCache, cols[3], rowTop, P{"capacityItems": 3000.0}).e("cache", "db")
		return b.servers(5, cols[2], 0, 80, P{"threads": 8.0, "callTimeoutMs": 500.0, "queueOrder": "lifo", "queueTimeoutMs": 400.0}, "cache").done()
	}
	register(&Scenario{
		ID: "launch-day-capstone", Title: "Launch day", Difficulty: "Hard",
		Summary: "A traffic ramp, a database slowdown and a crashed server, all in one afternoon.",
		Story:   "Traffic climbs from 100 to 800 requests per second. Midway the database slows down for a while, and shortly after a server crashes. Clients retry, as clients do. Design a system that keeps most users happy through all of it.",
		Diagram: capstone(false), Solution: capstone(true),
		Traffic:     map[string][]engine.TrafficPoint{"users": {{0, 100}, {10, 100}, {40, 800}, {75, 800}, {85, 150}, {100, 150}}},
		Chaos:       []engine.ChaosEvent{{Type: "slow", NodeID: "db", AtSec: 45, Factor: 4, DurationSec: 12}, {Type: "crash", NodeID: "random:appServer", AtSec: 60, DownSec: 15}},
		Goals:       engine.Goals{MinSuccessRate: 0.93, MaxP95Ms: f(700), MaxCostPerHour: f(1.8)},
		DurationSec: 100, WarmupSec: 8, Speed: 2,
		Hints: [3]string{
			"Three things go wrong at different times: load, a slow dependency and a lost server. Which parts of your design handle each?",
			"A cache takes load off the database, spare servers survive a crash, and timeouts with a short queue stop a slow dependency from tying up every thread.",
			"Try a load balancer, five servers with call timeouts and a short newest-first queue, and a cache in front of the database.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "The database becomes four times slower for twelve seconds. Which setting stops the slowdown from taking the whole system down?", Options: []string{"A call timeout on the app servers", "More threads", "Longer client timeouts"}, Answer: 0,
			Explain: "A timeout frees the thread instead of leaving it stuck behind the slow database, so servers keep answering what they can (from cache, for example)."}},
		Debrief: "Real outages stack: capacity, a slow dependency and a failure arrive together. The designs that survive layer defences: caches to cut load, redundancy for failures, and timeouts and short queues so slowness cannot spread.",
	})
}
