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
		Predictions: []Prediction{{ID: "p1", Question: "With one app server and one database, what limits throughput first?", Options: []string{"The app server's threads", "The database's disk speed", "Bandwidth between the two boxes"}, Answer: 0,
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
		Predictions: []Prediction{{ID: "p1", Question: "If you shrink the pool, what happens to requests that cannot get a connection?", Options: []string{"They are refused with an error at once", "They wait for a free connection", "They open a temporary extra connection"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "With oldest-first queues under overload, who gets served?", Options: []string{"Users who are still waiting for an answer", "Users who have already given up", "Whoever sent the cheapest request"}, Answer: 1,
			Explain: "The oldest requests are the ones whose callers have likely timed out. Serving them wastes capacity; newest-first or a queue timeout avoids it."}},
		Debrief: "A {{retry-storm|retry storm}} is a feedback loop: slowness causes timeouts, timeouts cause retries, retries cause slowness. Break it with backoff and jitter, bounded retries, {{load-shedding|load shedding}} and {{circuit-breaker|circuit breakers}}.",
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
		Predictions: []Prediction{{ID: "p1", Question: "Why does a 2 second replication lag matter even though the replica is 'a copy'?", Options: []string{"Reads may return versions older than a recent write", "Writes take longer because they wait for the replica", "The replica refuses reads until it catches up"}, Answer: 0,
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
		Predictions: []Prediction{{ID: "p1", Question: "What does an idempotency key guarantee?", Options: []string{"The write is applied at most once per key", "The request is sent to the server only once", "The request can never be lost on the way"}, Answer: 0,
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
		Predictions: []Prediction{{ID: "p1", Question: "Moving to a stronger isolation level will…", Options: []string{"Make every transfer slower, overlapping or not", "Make overlapping transfers wait or abort", "Stop transfers from ever being aborted"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "If the vendor's latency doubles, what happens to the user once the vendor call is behind a queue?", Options: []string{"User latency doubles, since the vendor is still called", "Users see no change; the backlog grows instead", "Requests fail, as the queue cannot hold them"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "What do you lose by putting a queue in front of the database?", Options: []string{"Throughput: each write takes an extra hop through it", "Immediate consistency: data lands a little later", "Atomicity: a write may be left half applied"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "Why do a few requests still fail even in a well-sized system?", Options: []string{"The balancer takes a moment to notice the failure", "The database drops connections when a peer dies", "Clients keep the dead server's address cached"}, Answer: 0,
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
		Predictions: []Prediction{{ID: "p1", Question: "Redis runs commands on one thread. Is that a problem at 300 requests per second?", Options: []string{"Yes: commands are queued one at a time", "No: each command takes only microseconds", "Only for writes, as reads run in parallel"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "A spatial index mainly improves…", Options: []string{"Write throughput", "Proximity searches", "Network latency"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "With an append-only file, what is the most you can lose in a crash?", Options: []string{"Nothing at all, as every write is logged first", "About the last second of writes", "Everything since the last backup was taken"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "The replica is 3 seconds behind and gets promoted. What happens to the writes from the last 3 seconds before the crash?", Options: []string{"They are safe: the primary had saved them", "They are lost: the replica never received them", "They are replayed when the old primary returns"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "A new server takes 20 seconds to boot and traffic climbs for a minute. When should the scaler start adding servers?", Options: []string{"As late as possible, to keep the monthly bill low", "Early, while servers still have spare room", "Only once requests have actually started failing"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "A server calls a database that is cut off, and 'Call timeout' is 0 (wait forever). What happens to the server?", Options: []string{"Calls to the database fail, but every other request is answered normally", "All its threads get stuck waiting, so even cache hits go unanswered", "The load balancer sees the stuck calls and stops sending to it"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "A server is slow half the time but never fails a health check. Which balancing rule protects users best?", Options: []string{"Round robin, so that every server gets its fair share", "Dropping servers whose latency is far above their peers'", "Weighting traffic towards the slow server so it catches up"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "The cache is warm and the hit rate is near 100%. Why can the database still be overwhelmed after ten seconds?", Options: []string{"The cache fills up and begins evicting the popular items", "All entries expire together, so nearly all requests miss", "The database slows down over time as its tables grow"}, Answer: 1,
			Explain: "A warm start gives every entry the same creation time, so they share one expiry time. Misses arrive as a wave, not a trickle."}},
		Debrief: "A cache hides load until the moment it cannot. Synchronised {{ttl|expiry}} turns a 99% hit rate into a 0% one for a few seconds. Jitter spreads the expiries out, and sharing concurrent misses keeps one popular item from being fetched a hundred times. Making the TTL very long also avoids the stampede, but it trades it for stale data, which is why this challenge limits staleness.",
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
		Predictions: []Prediction{{ID: "p1", Question: "What is the price of synchronous replication?", Options: []string{"Reads from the replica become slower and staler than before", "Every write waits for the replica, and stops if it is down", "Nothing: it is strictly better than asynchronous"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "The old primary restarts and nothing stops it. What happens to writes made to the new primary in the meantime?", Options: []string{"Merged into the old primary once it rejoins the cluster", "Lost: they exist only on the new primary, which clients abandon", "Kept: both primaries replicate to each other, so nothing is lost"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "A wave arrives every 40 seconds and a server takes 10 seconds to boot. Which setting helps most?", Options: []string{"Remove idle servers sooner so the monthly bill stays low", "Keep a floor of servers: reactive help comes too late", "Make the scaler wait longer before removing servers"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "Why can a closed-loop load test pass at a load that breaks the system in production?", Options: []string{"Production has more data, so queries are slower", "Users who wait for answers slow down as the system does", "Load tests run from inside the data centre"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "Average load is 70% of capacity. Why do users still see errors?", Options: []string{"The average is miscalculated, and load is really higher", "Rushes well above capacity overflow a short queue", "The database slows down once the cache fills up"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "One item gets 35% of reads. What does going from 4 shards to 8 do?", Options: []string{"Halves the hot shard's load, as the keys split across eight", "Helps a little, but the hot item still lives on one shard", "Nothing at all, because the router ignores new shards"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "Which eviction policy best protects a small popular set from a one-pass scan?", Options: []string{"FIFO: the oldest item goes first", "LFU: least frequently used out first", "Random: no item is favoured by the scan"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "Adding more app-server threads will…", Options: []string{"Cut the p95 a lot, as requests get served sooner", "Barely change it: the wait is on the shards", "Make each shard answer faster than before"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "Where does the work go if celebrities are pulled instead of pushed?", Options: []string{"Nowhere: the posts are simply never delivered to anyone", "To readers: each timeline read fetches a few celebrity posts", "To the author, who waits much longer for the post to publish"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "A CDN that can only hold a quarter of the pictures will…", Options: []string{"Cut the bill to almost nothing, as the CDN is free", "Cut it somewhat: popular pictures hit, the rest still pay", "Make no difference, since a quarter is too few"}, Answer: 1,
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
		Predictions: []Prediction{{ID: "p1", Question: "More app-server threads will…", Options: []string{"Fix it, since more uploads then run in parallel", "Not help: they all share the same saturated link", "Lower the bandwidth each upload needs, as work is shared"}, Answer: 1,
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
		Story:   "Each app server freezes for a second and a half every eight seconds or so (a stop-the-world garbage collection after a heap change). During a pause nothing on that server answers, and the clients give up after 800 ms. The {{p50|median latency}} is healthy, which is why nobody noticed.",
		Diagram: gcStorm(false), Solution: gcStorm(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.1), StarP95Ms: f(200)},
		DurationSec: 60, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at the failures: they are timeouts, in waves, and the latency of the requests that do succeed is fine. What freezes a whole server for a second?",
			"The failure breakdown and the events on the chart show GC pauses. Retries or more servers hide some of the damage but cost latency or money.",
			"Open an app server's advanced settings: shorten the GC pause (a smaller heap or a different collector). A pause shorter than the client's patience costs nothing.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Letting clients retry twice will…", Options: []string{"Fix it completely, since a retry never meets a pause", "Recover most requests, but make the slow ones very slow", "Make it worse, because retries double the load"}, Answer: 1,
			Explain: "A retry reaches a healthy server, so more requests succeed, but each rescued request has already waited out its timeout: the tail gets worse."}},
		Debrief: "A stop-the-world pause is invisible in averages and brutal at the tail. Retries and extra servers mask it at a cost; the real fix is a shorter pause. Watch {{p99}} and the timeouts, not the mean.",
	})

	// Disk full of writes: every secondary index is another disk write per row, against a disk with a fixed IOPS budget
	diskWrites := func(fixed bool) engine.Diagram {
		idx := 3.0
		if fixed {
			idx = 0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 80.0, "readPct": 20.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 32.0, "diskIops": 200.0, "indexes": idx}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "disk-writes", Title: "The disk full of writes", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The database is mostly idle on paper, yet writes queue and time out.",
		Story:   "A write-heavy service (80 requests a second, mostly writes) runs on a database whose disk does 200 operations a second. Years ago someone added three secondary indexes for a report nobody opens any more. CPU and connections look fine, but writes pile up and time out.",
		Diagram: diskWrites(false), Solution: diskWrites(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(1.5), StarCostPerHour: f(1.3), StarP95Ms: f(200)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Count the disk operations: each write needs one for the row and one more for every secondary index. How many writes a second is that, against 200?",
			"64 writes a second x (1 + 3 indexes) is about 256 operations a second against a disk that does 200, so the queue never drains.",
			"Nobody queries by those fields (the lookups share is zero), so drop the indexes. A faster disk would also work, but removing the waste is cheaper.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Removing the three unused indexes will…", Options: []string{"Slow reads down a lot, since lookups lose their index", "Cut the disk operations per write from four to one", "Free memory, but leave the disk work as it was"}, Answer: 1,
			Explain: "Each write must update every index as well as the table. With no reads using them, they only cost disk writes."}},
		Debrief: "Every index is paid for on every write. Check that an index is used before keeping it, and compare the write rate times the operations per write with the disk's budget.",
	})

	// A few expensive requests hold every thread: head-of-line blocking. Config only: give the server more threads.
	oneHeavy := func(fixed bool) engine.Diagram {
		threads := 16.0
		if fixed {
			threads = 64
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 250.0, "readPct": 100.0, "heavyPct": 5.0, "heavyMult": 40.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": threads, "cpuMs": 20.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 64.0, "readMs": 2.0}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "one-heavy-query", Title: "One heavy query", Difficulty: "Medium", ConfigOnly: true,
		Summary: "One request in twenty is expensive, and everyone suffers.",
		Story:   "A search page answers in 20 ms, except that one request in twenty runs a report that takes 800 ms of server time. The server has 16 threads. Most requests are light, yet the whole site goes slow in waves and some time out.",
		Diagram: oneHeavy(false), Solution: oneHeavy(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(4), StarCostPerHour: f(3.5), StarP95Ms: f(200)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Work out the threads' budget: the heavy requests are 5% of 250 a second, each holding a thread for 800 ms. How many threads is that on average?",
			"12 requests a second x 0.8 s is about 10 threads busy with heavy work alone, out of 16, so the 238 light requests a second have almost nothing left and queue behind them.",
			"Give the server enough threads that the heavy requests cannot take them all. The inspector shows p95 for light and heavy requests separately.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "The heavy requests are 5% of traffic. Their effect on the light ones is…", Options: []string{"About 5% slower, in line with their share of traffic", "Far worse: they hold threads the light ones need", "None at all, since they are separate requests"}, Answer: 1,
			Explain: "Every request shares the same threads. A few long requests occupy a large share of thread-time, so the quick ones wait behind them."}},
		Debrief: "Throughput is not the only budget: a small share of expensive requests can take most of the worker time and make the cheap majority slow. Look at latency by request type, and keep heavy work from sharing the pool the light work needs.",
	})

	// Write skew: each doctor leaves after checking the other is still on call. Config only: raise the isolation level.
	onCall := func(fixed bool) engine.Diagram {
		iso := "readCommitted"
		if fixed {
			iso = "serializable"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 30.0, "workload": "oncall", "keySpace": 40.0, "keyDist": "uniform", "timeoutMs": 3000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "isolation": iso}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "nobody-on-call", Title: "Nobody on call", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Every rule is checked, and the rule still breaks.",
		Story:   "A rota has pairs of doctors and the rule is that at least one of each pair stays on call. A doctor going off call first checks that the partner is still on. Nobody ever writes the partner's row, yet some nights both doctors in a pair are off at once.",
		Diagram: onCall(false), Solution: onCall(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxWriteSkew: i(0), MaxP95Ms: f(400), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.05), StarP95Ms: f(250)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Two requests for one pair overlap: each reads the pair before the other has written. What does the database promise about that?",
			"Moving to repeatable read (snapshot isolation) catches two writers of the SAME row, but these write different rows. Look at the numbers after you try it.",
			"Only serializable makes the two requests take turns on the pair. Set the database's isolation level to serializable and check the aborts it brings.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Will repeatable read (snapshot isolation) stop both doctors leaving?", Options: []string{"Yes: snapshot isolation aborts anything that conflicts", "No: they write different rows, so nothing clashes", "Only when load is low and few requests overlap"}, Answer: 1,
			Explain: "Snapshot isolation only detects two writes to the same row. Write skew is two writes to different rows justified by the same stale read, which only serializable execution rules out."}},
		Debrief: "Write skew is the anomaly snapshot isolation misses: a rule that spans several rows, checked by reading and enforced by writing one of them. Either serialize those transactions, lock the rows the rule reads, or make the rule a constraint the database enforces.",
	})

	// The slow query: most reads look an item up by a field other than its key, and there is no index for it
	slowQuery := func(fixed bool) engine.Diagram {
		idx := 0.0
		if fixed {
			idx = 1
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 150.0, "readPct": 90.0, "secondaryPct": 60.0, "timeoutMs": 2000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "indexes": idx}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "slow-query", Title: "The slow query", Difficulty: "Easy", ConfigOnly: true,
		Summary: "Most reads search by email, and the database reads every row to find it.",
		Story:   "Sixty percent of reads look an account up by its email address, not its id. The table has no index on email, so each of those reads scans the table and takes twenty times as long as a lookup by id. At the lunchtime peak the database queue fills and requests are refused.",
		Diagram: slowQuery(false), Solution: slowQuery(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(300), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.05), StarP95Ms: f(200)},
		DurationSec: 30, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"The database is busy but not because of how many requests there are: what is each lookup by email doing?",
			"Without an index on the field being searched, the database reads the whole table (see 'lookups by another field' on the users).",
			"Give the database a secondary index. It costs a little on every write, but this service is 90% reads.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "An index on email makes writes…", Options: []string{"Faster, because every write finds its row more quickly", "Slightly slower, as the index must be updated too", "Unchanged, as an index only helps queries, not writes"}, Answer: 1,
			Explain: "Every write also updates each index, so a read speed-up is paid for with a small write cost. Worth it when reads dominate."}},
		Debrief: "A lookup on an unindexed field is a full scan. An index trades a little write cost and storage for turning that scan into a seek. Check the access paths your queries really use.",
	})

	// Pick the store: a sensor firehose. The write path of the relational baseline is the bottleneck
	pickStore := func(fixed bool) engine.Diagram {
		model := "relational"
		if fixed {
			model = "timeSeries"
		}
		return (&bld{}).
			n("sensors", engine.KClient, cols[0], rowMid, P{"rps": 700.0, "readPct": 0.0, "timeoutMs": 1500.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 64.0, "queueCap": 500.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "model": model, "queueCap": 500.0}).
			e("sensors", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "pick-the-store", Title: "Pick the store", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Seven hundred sensor readings a second overwhelm a general-purpose database.",
		Story:   "Sensors send 700 readings a second and nothing ever reads them back in real time. The readings go into a relational database that spends the same effort on each insert as on an order, and a quarter of them are refused when its queue fills.",
		Diagram: pickStore(false), Solution: pickStore(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(3.0), StarCostPerHour: f(2.6), StarP95Ms: f(200)},
		DurationSec: 30, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"The workload is almost all appends and almost no reads. Is a general-purpose relational layout the right shape for it?",
			"More database connections do not help here: the pool each app server may open to the database is the limit.",
			"Change the database's data model (advanced settings) to one built for append-heavy writes: time-series, or wide-column.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "What makes a time-series store cheaper per insert?", Options: []string{"It skips durability, so nothing is written to disk", "Append-optimised storage avoids updating in place", "It keeps everything in memory instead of on disk"}, Answer: 1,
			Explain: "Appending to the end of a log-structured layout avoids random in-place updates, so each write costs a fraction of a general-purpose engine's."}},
		Debrief: "Match the store to the access pattern. Append-heavy, rarely-read data fits an append-optimised layout; paying a general-purpose engine's cost per row for a firehose is how a database becomes the bottleneck.",
	})

	// Poison pill: a malformed job is retried ten times, burning worker time, and then vanishes
	poisonPill := func(fixed bool) engine.Diagram {
		attempts, dlq := 10.0, false
		if fixed {
			attempts, dlq = 3, true
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 50.0, "readPct": 0.0, "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("q", engine.KQueue, cols[2], rowMid, P{"capacity": 20000.0, "maxAttempts": attempts, "deadLetter": dlq}).
			n("w", engine.KWorker, cols[3], rowMid, P{"concurrency": 8.0, "processMs": 100.0, "poisonPct": 12.0}).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "app").e("app", "q").e("q", "w").e("w", "db").done()
	}
	register(&Scenario{
		ID: "poison-pill", Title: "The poison pill", Difficulty: "Medium", ConfigOnly: true,
		Summary: "One in eight messages can never be processed, and everything else queues behind them.",
		Story:   "A bad deploy upstream means about 12% of the messages are malformed. The queue tries each one ten times, every try occupying a worker for its full processing time, and then silently drops it. Healthy messages wait behind the retries and nobody can find out what was dropped.",
		Diagram: poisonPill(false), Solution: poisonPill(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MinJobsCompleted: f(0.85), MaxJobP95Ms: f(1500), MaxJobsLost: i(0), MaxCostPerHour: f(1.3), StarCostPerHour: f(1.2), StarP95Ms: f(500)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Open the job counters: how many retries are there, and how long do healthy jobs wait?",
			"Retrying something that can never succeed only burns worker time. How many attempts does a job deserve, and where should the failures go instead of vanishing?",
			"On the queue (advanced settings): lower the attempts per job and turn on the dead-letter queue so the failed jobs are kept for inspection.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Lowering the attempts alone (no dead-letter queue) will…", Options: []string{"Fix everything, since the bad jobs are then gone", "Free the workers but lose the bad messages silently", "Keep the bad messages, just with fewer attempts each"}, Answer: 1,
			Explain: "Fewer retries stop the waste, but a job that used up its attempts is gone. A dead-letter queue keeps it where someone can look at it."}},
		Debrief: "Retries help against transient failures and hurt against permanent ones. Bound the attempts, back off, and route what still fails to a dead-letter queue so it is visible and recoverable instead of lost.",
	})

	// Consumer lag: eight consumers, four partitions
	consumerLag := func(fixed bool) engine.Diagram {
		parts := 4.0
		if fixed {
			parts = 16
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 60.0, "readPct": 0.0, "keySpace": 200.0, "keyDist": "uniform", "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("log", engine.KLog, cols[2], rowMid, P{"partitions": parts, "capacity": 50000.0}).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "app").e("app", "log")
		for k := 0; k < 8; k++ {
			id := fmt.Sprintf("c%d", k+1)
			b.n(id, engine.KWorker, cols[3], float64(k)*60, P{"concurrency": 2.0, "processMs": 100.0}).label(id, fmt.Sprintf("Consumer %d", k+1)).e("log", id).e(id, "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "consumer-lag", Title: "Consumer lag", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Eight consumers, and the backlog still grows.",
		Story:   "Events stream into a partitioned log read by eight consumers, each able to handle several messages at once. Yet the backlog grows all day and processing falls further and further behind. Adding yet more consumers was already tried.",
		Diagram: consumerLag(false), Solution: consumerLag(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MinJobsCompleted: f(0.97), MaxJobP95Ms: f(2000), MaxCostPerHour: f(1.5), StarCostPerHour: f(1.4), StarP95Ms: f(800)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at which consumers are busy. Do all eight have work?",
			"A partition is read by one member of a group at a time, and one message after another to keep order. How many partitions are there?",
			"Raise the number of partitions on the log (16 gives every consumer two). More consumers would not help: they would sit idle.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "With 4 partitions, what do consumers 5 to 8 do?", Options: []string{"Share the load equally with consumers 1 to 4", "Nothing: a partition has one reader per group", "Take over the slowest partitions from the others"}, Answer: 1,
			Explain: "Within a consumer group each partition is assigned to exactly one member, so parallelism is capped by the partition count."}},
		Debrief: "In a partitioned log the number of partitions is the ceiling on consumer parallelism. Choose it for the throughput you will need, since changing it later also changes how keys map to partitions.",
	})

	// Hot cell: location pings from one busy city all land on the shard that owns that map cell
	hotCell := func(fixed bool) engine.Diagram {
		by := "key"
		if fixed {
			by = "request"
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 500.0, "workload": "geo", "readPct": 0.0, "keySpace": 400.0, "keyDist": "uniform", "hotKeyPct": 70.0, "timeoutMs": 1000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "queueCap": 2000.0, "dbPool": 200.0}).
			n("shards", engine.KShardRouter, cols[2], rowMid, P{"shardBy": by}).
			e("users", "app").e("app", "shards")
		for k := 0; k < 4; k++ {
			id := fmt.Sprintf("r%d", k+1)
			b.n(id, engine.KRedis, cols[3], float64(k)*100+30, P{"scoreMs": 4.0}).e("shards", id)
		}
		return b.done()
	}
	register(&Scenario{
		ID: "hot-cell", Title: "The hot cell", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Four shards, and one of them is on fire.",
		Story:   "A ride-sharing app stores driver locations in four shards, split by map cell. Most pings come from the one busy city centre, so the shard that owns that cell takes most of the load while the other three idle. Adding shards was already tried: the hot cell stays on a single one.",
		Diagram: hotCell(false), Solution: hotCell(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(300), MaxCostPerHour: f(1.5), StarCostPerHour: f(1.3), StarP95Ms: f(200)},
		DurationSec: 30, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Compare the four shards in the inspector: is the load even?",
			"Each shard can take about 250 updates a second (4 ms each). The shard that owns the busy cell is asked for roughly three quarters of 500 a second.",
			"Change how the shard router chooses a shard: by the request itself rather than the cell, so the busy cell is spread over all four. (A real system pays for that on reads, which then have to ask every shard.)",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Adding a fifth and sixth shard, still split by cell, will…", Options: []string{"Fix the hot shard, as the cell spreads out", "Leave the hot cell on one shard, so nothing improves", "Make it worse, since the data must move first"}, Answer: 1,
			Explain: "One cell maps to one shard however many shards there are. A hot key cannot be spread by adding more partitions of the same key."}},
		Debrief: "Partitioning by a key that is itself skewed concentrates the skew. Spread hot keys (by sender, by a suffix, or by splitting the cell), and remember the price: reads of one key must then query every shard.",
	})

	// Restore drill: a bad migration corrupts the database; how much is lost and how long until it is back
	restoreDrill := func(fixed bool) engine.Diagram {
		every, mbps := 0.0, 200.0
		if fixed {
			every, mbps = 5, 600
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 60.0, "readPct": 50.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 32.0, "dataGB": 10.0, "backupEverySec": every, "restoreMBps": mbps}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "restore-drill", Title: "The restore drill", Difficulty: "Medium", ConfigOnly: true,
		Summary: "A bad migration corrupts the database. How much is lost, and for how long is it gone?",
		Story:   "At 25 seconds a bad migration starts silently corrupting rows. Someone notices three seconds later and restores the last backup from before the damage. The database holds 10 GB. There are no backups at all, and restoring streams 200 MB a second. The business can lose at most five seconds of writes and can be without the database for at most twenty seconds.",
		Diagram: restoreDrill(false), Solution: restoreDrill(true),
		Chaos:       []engine.ChaosEvent{{Type: "corrupt", NodeID: "db", AtSec: 25}, {Type: "restore", NodeID: "db", AtSec: 28}},
		Goals:       engine.Goals{MinSuccessRate: 0.7, MaxRpoSec: f(5), MaxRtoSec: f(20), MaxCostPerHour: f(1.1), StarCostPerHour: f(1.0)},
		DurationSec: 80, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Two numbers decide this: how much is lost (the recovery point) and how long you wait (the recovery time). Which setting moves each?",
			"A restore goes back to the last backup before the damage, so the data lost is the time since that backup: with no backups it is everything. And the time to restore is the data size divided by the restore speed: 10 GB at 200 MB/s is over 50 seconds.",
			"Turn on backups every few seconds on the database, and raise the restore speed. Each costs a little: a restore bandwidth that is higher than you need is wasted money.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Backing up every five seconds (and nothing else) will…", Options: []string{"Fix both the data lost and the time taken to restore", "Cut the data lost but leave the restore as slow", "Cut the time to restore but keep the same data loss"}, Answer: 1,
			Explain: "How often you back up sets how much you can lose; how fast a backup streams back sets how long you wait. They are separate levers."}},
		Debrief: "RPO and RTO are different dials: backup frequency sets the recovery point, restore bandwidth and data size set the recovery time. Pick them from what the business can bear, and then practise the restore.",
	})

	// Lost cart: two leaders accept writes for the same cart; last-writer-wins silently drops one of them
	lostCart := func(fixed bool) engine.Diagram {
		policy := "lastWriterWins"
		if fixed {
			policy = "merge"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 150.0, "readPct": 0.0, "keySpace": 100.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 32.0, "topology": "multiLeader", "leaders": 2.0, "syncLagMs": 300.0, "conflictPolicy": policy}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "lost-cart", Title: "The lost cart item", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Customers add items to a cart and some of them silently vanish.",
		Story:   "To keep checkout fast in two regions, the cart database has two leaders that each accept writes and pass them to the other 300 ms later. Two people adding to the same cart in that gap each succeed, and then one of the additions disappears: the database keeps whichever write is newer. Nothing errors, so nobody notices until customers complain.",
		Diagram: lostCart(false), Solution: lostCart(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxLostUpdates: i(0), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.0)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Count the lost updates on the goals panel: every request succeeds, so what is being lost, and when?",
			"A write to a cart at one leader and another write to the same cart at the other leader within 300 ms are concurrent: neither leader has seen the other's. With last-writer-wins one of them is thrown away.",
			"Choose what happens on a conflict: a merging data type (a counter or a set that adds) keeps both. Shortening the delay only makes conflicts rarer, and one leader would avoid them altogether at the cost of a slow region.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "If the leaders passed writes on in 50 ms instead of 300, lost updates would…", Options: []string{"Disappear altogether once writes cross quickly", "Become rarer but still happen", "Double, as more writes cross each other"}, Answer: 1,
			Explain: "The window in which two writes can be concurrent shrinks, so fewer collide, but any two writes inside it still conflict."}},
		Debrief: "With more than one writer, concurrent writes to the same item are normal, and the conflict rule decides who loses. Last-writer-wins is easy and loses data silently; data types that merge keep every update.",
	})

	// Quorum overlap: a key-value store that answers quickly but sometimes shows a client its own old data
	quorumOverlap := func(fixed bool) engine.Diagram {
		r, w := 1.0, 1.0
		if fixed {
			r, w = 2, 2
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 50.0, "keySpace": 20.0, "keyDist": "uniform", "consistency": "causal", "timeoutMs": 3000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "dbPool": 200.0}).
			n("kv", engine.KKVStore, cols[2], rowMid, P{"nodes": 5.0, "replicas": 3.0, "readQuorum": r, "writeQuorum": w, "readMs": 4.0, "writeMs": 60.0, "variability": "exponential", "concurrency": 32.0}).
			e("users", "app").e("app", "kv").done()
	}
	register(&Scenario{
		ID: "quorum-overlap", Title: "Read your own writes", Difficulty: "Hard", ConfigOnly: true,
		Summary: "The store is fast and never errors, but users sometimes lose sight of what they just saved.",
		Story:   "A five-node key-value store keeps three copies of every key. To be quick, a write returns as soon as one copy has it and a read returns the first copy that answers. Users save a setting, reload, and sometimes see the old value. Support has noticed that it also flips back and forth between old and new.",
		Diagram: quorumOverlap(false), Solution: quorumOverlap(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxSessionViolations: i(0), MaxP95Ms: f(600), MaxCostPerHour: f(1.6), StarCostPerHour: f(1.4)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The goals panel counts reads that returned something older than what the same client already knew. Where could an older copy come from?",
			"With three copies, a write acknowledged by one copy and a read answered by a different one need not meet: the read can reach copies the write has not got to yet.",
			"Make every read overlap every acknowledged write: the read quorum plus the write quorum must be more than the number of copies (3). Two and two does it; the price is waiting for the second-fastest copy.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "With three copies, a read quorum of 1 and a write quorum of 3 would…", Options: []string{"Still show old data, because a single read can miss the new write", "Never show old data, but make writes slower and fragile", "Be exactly as fast as 2 and 2, just reversed"}, Answer: 1,
			Explain: "1 + 3 is more than 3, so the read always meets a copy with the write, but a write now waits for the slowest copy and needs all of them to be up."}},
		Debrief: "R + W > N makes reads overlap writes, so a client sees its own writes and never goes backwards. You buy it with latency (the kth fastest copy instead of the fastest) and with availability (fewer failures tolerated). Choose R and W for the workload: read-heavy systems often pick a cheap read and an expensive write.",
	})

	// Lease expired: a frozen lock holder wakes up and books a seat someone else already took
	leaseExpired := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 600.0, "readPct": 0.0, "workload": "booking", "keySpace": 3000.0, "keyDist": "uniform", "timeoutMs": 4000.0, "retries": 0.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 64.0, "writeMs": 40.0, "conditionalWrite": fixed}).
			n("redis", engine.KRedis, cols[2], 320, nil).
			e("users", "lb")
		for k := 0; k < 3; k++ {
			id := fmt.Sprintf("app%d", k+1)
			b.n(id, engine.KAppServer, cols[2], float64(k)*100+30, P{"threads": 24.0, "queueCap": 2000.0, "lockTtlMs": 150.0, "lockWaitMs": 300.0, "dbPool": 100.0, "gcPauseMs": 600.0, "gcEverySec": 0.5}).e("lb", id).e(id, "db").e(id, "redis")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "lease-expired", Title: "The lease that ran out", Difficulty: "Hard", ConfigOnly: true,
		Summary: "The lock was held. The seat was booked twice anyway.",
		Story:   "Seats are booked under a Redis lock that expires after 150 ms, so a crashed server cannot hold one forever. The app servers freeze for 600 ms every so often (a garbage-collection pause). A server that froze while holding a lock wakes up long after it expired, another server has booked the seat in the meantime, and the first one books it anyway: two people have the same seat.",
		Diagram: leaseExpired(false), Solution: leaseExpired(true),
		Goals:       engine.Goals{MinSuccessRate: 0.93, MaxConflicts: i(0), MaxP95Ms: f(4000), StarP95Ms: f(500), MaxCostPerHour: f(3.2), StarCostPerHour: f(3.0)},
		DurationSec: 90, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at when the double bookings happen: they follow the garbage-collection pauses. What does a server that was frozen believe about its lock when it wakes up?",
			"It still thinks it holds the lock, but the lease ran out while it was frozen. Nothing the lock service does can reach into a process that is not running: the check has to happen where the write lands.",
			"Make the database book a seat only if it is still free (a conditional write, like a unique constraint). A fencing token from a lock service helps only when the stale write arrives after the newer one; the conditional write does not depend on order.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Why is a lock with an expiry not enough to guarantee that only one process acts at a time?", Options: []string{"The expiry is always set too short to be safe", "A paused process can act after its lease has been reissued", "Redis loses locks whenever it restarts"}, Answer: 1,
			Explain: "A lease bounds how long a dead holder blocks others, but a live holder that is frozen does not know its lease ended. Only the resource being written can refuse the stale write."}},
		Debrief: "Treat a lock as an optimisation, not a guarantee. Make the write itself safe: a conditional write (compare-and-set, unique constraint) refuses whatever arrives late, and a fencing token does the same when the resource can compare tokens.",
	})

	// Who is the leader: failover is decided by a coordination service that has itself lost its majority
	whoLeads := func(fixed bool) engine.Diagram {
		nodes := 3.0
		if fixed {
			nodes = 5
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 40.0, "keyDist": "uniform", "keySpace": 20000.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowTop, P{"onFailure": "promote", "electionBy": "coordinator", "detectSec": 3.0, "promoteSec": 5.0}).
			n("replica", engine.KReplica, cols[4], rowMid, P{"replicationLagMs": 500.0}).
			n("zk", engine.KCoordinator, cols[4], rowBot, P{"nodes": nodes, "electionTimeoutMs": 1000.0}).label("zk", "Coordination service").
			e("users", "lb").e("db", "zk").servers(2, cols[2], rowTop, 100, P{"threads": 16.0}, "replica").done()
	}
	register(&Scenario{
		ID: "who-is-the-leader", Title: "Who is the leader?", Difficulty: "Hard", ConfigOnly: true,
		Summary: "The primary is dead, a replica is ready, and nobody is allowed to promote it.",
		Story:   "Failover here is not left to a timer: promotion waits until the coordination service agrees on who the leader is, because promoting without agreement is how two primaries appear. Tonight two of the coordination service's three machines fail at the same time as the database primary. The service has no majority, so it elects nobody, so the replica waits. Writes fail the whole time.",
		Diagram: whoLeads(false), Solution: whoLeads(true),
		Chaos:       []engine.ChaosEvent{{Type: "clusterNodeDown", NodeID: "zk", AtSec: 15, DurationSec: 40, Factor: 2}, {Type: "crash", NodeID: "db", AtSec: 20, DownSec: 60}},
		Goals:       engine.Goals{MinSuccessRate: 0.85, MaxP95Ms: f(800), MaxLostWrites: i(100), MaxCostPerHour: f(2.5)},
		DurationSec: 70, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The replica is ready but the database is not promoting it. Who has to agree first, and can they?",
			"The coordination service needs a majority of its machines to elect anyone. Count how many it has and how many it lost.",
			"With 3 machines, losing 2 leaves no majority. Run 5 (a majority is 3): the same two failures leave a working quorum and the promotion goes ahead on schedule.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A 3-machine cluster loses two machines. Can the survivor keep serving as leader?", Options: []string{"Yes: one machine is enough to lead alone", "No: a majority (2 of 3) is needed for a leader", "Yes, but only for reads, not writes"}, Answer: 1,
			Explain: "A majority is what makes two leaders impossible: two groups cannot both hold more than half. The price is that the cluster stops when it loses its majority."}},
		Debrief: "Quorums trade availability for safety. N machines tolerate (N-1)/2 failures: 3 tolerate one, 5 tolerate two. Size the cluster for the failures you expect together, and spread it across failure domains so they do not happen together.",
	})

	// Launch day: nearly every write is to one row, and row locks make those writes queue one behind the other
	launchDay := func(fixed bool) engine.Diagram {
		app := P{"threads": 64.0, "queueCap": 2000.0, "dbPool": 100.0}
		if fixed {
			app["hotKeyMode"] = "split"
			app["splitShards"] = 8.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 200.0, "readPct": 0.0, "keySpace": 5000.0, "keyDist": "uniform", "hotKeyPct": 90.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, app).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 64.0, "writeMs": 20.0, "hotRowLock": true}).label("db", "Stock database").
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "launch-day", Title: "Launch day", Difficulty: "Hard", ConfigOnly: true,
		Summary: "The new phone goes on sale and every buyer decrements the same stock row.",
		Story:   "At noon the new phone goes on sale. Nine in ten orders change the same row, the stock counter, and the database locks a row while it writes it, so those writes go one after another: 20 ms each is 50 a second, and 180 a second are arriving. The queue behind that row holds database connections, and the buyers time out before their turn comes. The rest of the shop has nothing wrong with it.",
		Diagram: launchDay(false), Solution: launchDay(true),
		Goals:       engine.Goals{MinSuccessRate: 0.95, MaxP95Ms: f(500), MaxCostPerHour: f(4.0), StarCostPerHour: f(3.6), StarP95Ms: f(100)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Which row is slow, and what limits how fast one row can change?",
			"A row lock lets one write at a time through. With 20 ms per write one row takes 50 writes a second whatever you do to the database around it; more connections only give the waiting writes somewhere to wait.",
			"Stop writing to a single row: in the app server's hot-key setting, split the counter over several rows, so the writes go to different rows in parallel. Reading the total then costs adding the rows up.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Doubling the database connections will…", Options: []string{"Double the writes the hot row can take", "Not change what one row can take: the lock is the limit", "Halve the latency of every write to the row"}, Answer: 1,
			Explain: "A lock serialises the writes to its row. More connections let more of them wait at once, which does not make the row faster."}},
		Debrief: "A hot row is a throughput ceiling of one over the write time, however big the database. Split the counter (several sub-rows added up on read), batch the updates, or queue them so they are applied in order without making buyers wait.",
	})

	// Count the clicks: a stream job counts clicks per window, but some arrive long after they happened
	countClicks := func(fixed bool) engine.Diagram {
		st := P{"windowSec": 10.0, "watermarkDelaySec": 2.0, "lateEventPct": 15.0, "lateBySec": 20.0}
		if fixed {
			st["allowedLatenessSec"] = 30.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 200.0, "readPct": 0.0, "keySpace": 10000.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0, "queueCap": 500.0}).
			n("clicks", engine.KStream, cols[2], rowMid, st).label("clicks", "Clicks per 10 seconds").
			e("users", "app").e("app", "clicks").done()
	}
	register(&Scenario{
		ID: "count-the-clicks", Title: "Count the clicks", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The click counts are wrong because some clicks arrive late.",
		Story:   "A dashboard shows clicks per ten seconds, counted by a stream job by the time each click happened. About one click in seven comes from a phone that was offline and reaches the server twenty seconds after it happened. The job has already published the window by then and throws the click away, so the totals are always a little low. The dashboard must still show each window within three seconds of it ending.",
		Diagram: countClicks(false), Solution: countClicks(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxLateDropped: i(20), MaxResultDelaySec: f(3), MaxCostPerHour: f(1.0)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the stream job's stats: how many clicks does it drop, and why?",
			"A window is published when the watermark (the newest click time minus a delay) passes its end. A click that arrives after that is dropped, unless the job is told to accept late ones for a while.",
			"Waiting longer before publishing (the watermark delay) would catch them, but the dashboard must not wait more than three seconds. Instead, allow lateness: publish on time, then update the window when a late click arrives.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "To count clicks that arrive 20 seconds late, the dashboard must…", Options: []string{"Wait 20 seconds before showing anything at all, to be safe", "Show the window on time and correct it when late clicks arrive", "Ignore late clicks, since they are rare and make little difference"}, Answer: 1,
			Explain: "A longer watermark delay trades freshness for completeness. Allowed lateness gives both, at the price of results that change after they were shown."}},
		Debrief: "Event time is when something happened, not when you heard about it. The watermark is a guess about how late data can be: too tight and you drop data, too loose and you answer slowly. Allowed lateness, with updates to published windows, lets you answer early and still end up correct.",
	})

	// Unique visitors: counting different people exactly needs memory that grows with the audience
	uniqueVisitors := func(fixed bool) engine.Diagram {
		st := P{"counting": "exact"}
		if fixed {
			st = P{"counting": "hyperLogLog", "hllPrecision": 12.0}
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 200.0, "readPct": 0.0, "keySpace": 5000.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0, "queueCap": 500.0}).
			n("visitors", engine.KStream, cols[2], rowMid, st).label("visitors", "Unique visitors").
			e("users", "app").e("app", "visitors").done()
	}
	register(&Scenario{
		ID: "unique-visitors", Title: "Unique visitors", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Counting each visitor exactly needs a table that grows with the audience.",
		Story:   "The marketing dashboard shows how many different people visited. The stream job keeps every visitor id it has seen, so the table grows with the audience, and the job has a memory budget of 16 KB for it (in this small run; a real audience is thousands of times larger and so is the table). Marketing is happy with a number that is right to within a few percent.",
		Diagram: uniqueVisitors(false), Solution: uniqueVisitors(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxStateKB: f(16), MaxCountErrorPct: f(6), MaxCostPerHour: f(1.0)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the stream job's memory for counting. What makes it grow, and how fast?",
			"An exact count needs to remember every id to know whether it was seen before. If a number that is slightly off is acceptable, there are structures that count distinct items in a fixed, tiny amount of memory.",
			"Switch the job's visitor counting to HyperLogLog. Its precision sets the number of registers: more registers, less error, more memory. Pick one that fits both the 16 KB budget and the 6% error.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A HyperLogLog with twice the registers has…", Options: []string{"Half the error for twice the memory", "About 30% less error for twice the memory", "The same error, but it handles more visitors"}, Answer: 1,
			Explain: "The error falls with the square root of the registers: doubling them divides it by about 1.4, while memory doubles. Accuracy gets expensive quickly."}},
		Debrief: "Approximate structures trade a bounded, known error for memory that does not grow with the data: HyperLogLog counts distinct items, a Bloom filter answers 'seen before?', a count-min sketch counts how often. They are the answer whenever the exact version will not fit.",
	})

	// Top ten: the ten most popular items out of a huge catalogue, in a small amount of memory
	topTen := func(fixed bool) engine.Diagram {
		st := P{"topK": "exact"}
		if fixed {
			st = P{"topK": "countMin", "sketchWidth": 1000.0, "sketchDepth": 3.0}
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 300.0, "readPct": 0.0, "keySpace": 200000.0, "keyDist": "zipf", "zipfS": 1.1, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0, "queueCap": 500.0}).
			n("trending", engine.KStream, cols[2], rowMid, st).label("trending", "Trending items").
			e("users", "app").e("app", "trending").done()
	}
	register(&Scenario{
		ID: "top-ten", Title: "The top ten", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Find the ten most popular items out of 200,000 without a counter for each.",
		Story:   "The shop shows a trending list: the ten most viewed items out of a catalogue of 200,000. The stream job keeps an exact counter for every item it has seen, and the table keeps growing: it already needs far more memory than the 16 KB the job is allowed (in this small run; the real catalogue and traffic are far bigger). The list does not have to be perfect, but at least eight of the ten must be right.",
		Diagram: topTen(false), Solution: topTen(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MinTopKRecall: f(0.8), MaxStateKB: f(16), MaxCostPerHour: f(1.0)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the memory the stream job needs and at what it is counting. Does every item need its own counter?",
			"Most of the 200,000 items are seen once or never. Only the few very popular ones matter for a top ten, and a structure can estimate counts for all items in a fixed table, at the cost of items sharing counters.",
			"Switch the job to the count-min sketch. Its width and depth set the table (width x depth counters of 4 bytes each): a table too narrow mixes everything together and misses the top ten, one too big breaks the memory budget.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A count-min sketch estimates how often an item occurred. It can be wrong by…", Options: []string{"Counting too few: it can miss occurrences", "Counting too many: other items share its counters", "Either way, equally, depending on the table"}, Answer: 1,
			Explain: "Every occurrence of an item adds to all of its counters, so an estimate is never too low. Other items sharing a counter can only inflate it, and taking the smallest counter keeps that small."}},
		Debrief: "Heavy hitters are few, and a structure that only needs to be right about them can be tiny. A count-min sketch plus a short candidate list finds the top items in a fixed table; the width bounds the overcount and the depth bounds the chance of it. Exactness costs memory in proportion to the number of different items.",
	})

	// Polite crawler: the sites it fetches from refuse a crawler that comes in rushes
	politeCrawler := func(fixed bool) engine.Diagram {
		app := P{"threads": 16.0, "queueCap": 500.0, "hostChoice": "byKey"}
		if fixed {
			app["perHostRps"] = 8.0
		}
		b := (&bld{}).
			n("crawl", engine.KClient, cols[0], rowMid, P{"rps": 12.0, "readPct": 100.0, "arrival": "bursty", "burstFactor": 4.0, "burstSec": 2.0, "keySpace": 3000.0, "keyDist": "uniform", "timeoutMs": 8000.0, "retries": 0.0}).
			n("crawler", engine.KAppServer, cols[1], rowMid, app).
			n("pages", engine.KDatabase, cols[3], rowBot, P{"connections": 8.0}).label("pages", "Page store").
			e("crawl", "crawler").e("crawler", "pages")
		for k, h := range []string{"s1", "s2", "s3"} {
			b.n(h, engine.KExternalAPI, cols[2], float64(k)*110+40, P{"latencyMs": 100.0, "errorPct": 0.0, "calledFor": "all", "politeRps": 8.0}).label(h, "Site "+h[1:]).e("crawler", h)
		}
		return b.done()
	}
	register(&Scenario{
		ID: "polite-crawler", Title: "The polite crawler", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The sites keep answering 429 to the crawler, and it is about to be blocked.",
		Story:   "A crawler fetches pages from three sites. Its work arrives in rushes, so for a couple of seconds at a time it asks one site for 16 pages a second, while each site says it will serve a crawler at most 8 a second and answers 429 (too many requests) beyond that. Over a whole minute the crawler is nowhere near the limit, but the sites do not average: a crawler that keeps getting 429 gets blocked for good.",
		Diagram: politeCrawler(false), Solution: politeCrawler(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(7000), MaxCostPerHour: f(1.5), StarP95Ms: f(100)},
		DurationSec: 120, WarmupSec: 10, Speed: 1,
		Hints: [3]string{
			"Look at the failure reasons: who is refusing the crawler, and when does it happen?",
			"The sites count calls per second, not per minute. During a rush the crawler asks for twice what a site allows.",
			"Give the crawler server a limit on calls per second to any one site (its 'Calls per second to one service' setting, under Show advanced), equal to what the sites allow. The excess waits its turn instead of being refused.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Limiting the crawler to 8 calls a second per site (what the sites allow) will…", Options: []string{"Make it slower on average, since it cannot exceed 8", "Leave its speed unchanged, since the average is under 8", "Make each fetch faster, as sites then reply quicker"}, Answer: 0,
			Explain: "A limit is a ceiling: a rush that used to be refused now queues. Nothing is lost, but pages wait their turn, and a limit below the average load would make the queue grow without end."}},
		Debrief: "Politeness is a per-host rate limit on the crawler itself, usually with a queue per host so that waiting for one slow or strict site does not block the others. The average rate is what you plan capacity with, but the peak rate decides whether you get blocked.",
	})

	// Slow first token: a chat service on one GPU whose batch is far too small for the traffic
	slowFirstToken := func(fixed bool) engine.Diagram {
		llm := P{"gpus": 1.0, "batchSize": 4.0, "kvCacheTokens": 32000.0, "promptTokens": 500.0, "outputTokens": 150.0, "queueCap": 400.0}
		if fixed {
			llm["batchSize"] = 64.0
			llm["kvCacheTokens"] = 64000.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 3.0, "readPct": 0.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 30000.0, "retries": 0.0}).
			n("chat", engine.KAppServer, cols[1], rowMid, P{"threads": 64.0, "queueCap": 2000.0}).
			n("model", engine.KLLMServer, cols[2], rowMid, llm).label("model", "Chat model").
			e("users", "chat").e("chat", "model").done()
	}
	register(&Scenario{
		ID: "slow-first-token", Title: "Slow first token", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Users stare at a blank chat for seconds before the first word appears.",
		Story:   "A chat assistant runs on one GPU. Users do not mind the answer streaming in word by word, but they stare at a blank screen for many seconds before the first word appears: the time to first token is the whole problem. The GPU takes only four answers at a time, and three new requests arrive every second, each needing about one second of GPU time at that batch size. The budget is about six dollars an hour in all, which pays for one GPU, not two.",
		Diagram: slowFirstToken(false), Solution: slowFirstToken(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxTTFTMs: f(2500), MaxCostPerHour: f(6.0), StarCostPerHour: f(5.5), StarP95Ms: f(100)},
		DurationSec: 120, WarmupSec: 20, Speed: 1,
		Hints: [3]string{
			"Look at the model's queue and at how many answers it writes at once. Is the GPU keeping up with the requests?",
			"At four answers at a time one GPU finishes about one request a second; three arrive every second. A GPU writes a whole batch of answers in about the time it takes to write one, so a bigger batch means far more answers per second.",
			"Raise the number of answers at once: 32 or 64 is enough. Every answer in progress holds its prompt and answer tokens in the KV cache (about 650 each), so the cache caps the batch at roughly 49 whatever you ask for; raising the cache lifts that cap. Buying a second GPU would work but breaks the budget.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Writing 64 answers at once instead of 4 makes each answer…", Options: []string{"Sixteen times slower, since the GPU is shared sixteen ways", "Somewhat slower, but many more answers per second", "Just as fast, as GPUs run batches in parallel for free"}, Answer: 1,
			Explain: "Each decode step is a little longer with more answers sharing it, but a step produces a token for every answer in the batch. Batching trades a little speed per answer for a lot more answers per second."}},
		Debrief: "A GPU's throughput comes from batching: decoding is limited by reading the model, which is shared by every answer in the batch. The limits are the KV cache (memory for every answer in progress) and the per-answer slowdown. Time to first token is mostly queueing, so fix the throughput, not the prompt reading.",
	})

	// RAG under load: document search that finds everything but cannot keep up
	ragSearch := func(fixed bool) engine.Diagram {
		ef := 512.0
		if fixed {
			ef = 160
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 150.0, "readPct": 100.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 3000.0, "retries": 0.0}).
			n("api", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0, "queueCap": 500.0}).
			n("docs", engine.KVectorStore, cols[2], rowMid, P{"vectors": 20.0, "dims": 768.0, "efSearch": ef, "connections": 4.0, "queueCap": 300.0}).label("docs", "Document search").
			e("users", "api").e("api", "docs").done()
	}
	register(&Scenario{
		ID: "rag-under-load", Title: "Search under load", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The document search finds everything, and answers nobody in time.",
		Story:   "An assistant looks up documents in a vector index before it answers: twenty million passages, searched approximately. Whoever set it up asked for the best recall they could get, so every search examines hundreds of candidates and finds nearly all the true neighbours. At 150 searches a second the four search cores cannot keep up, the queue grows and users time out. Retrieval should still find at least nine of every ten true neighbours.",
		Diagram: ragSearch(false), Solution: ragSearch(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(250), MinRecall: f(0.9), MaxCostPerHour: f(2.0), StarP95Ms: f(100)},
		DurationSec: 60, WarmupSec: 10, Speed: 1,
		Hints: [3]string{
			"Look at the search component: how long does one search take, and how many can it run at once?",
			"Four searches at once, each taking about 30 ms, is about 135 a second: less than 150. A search takes a fixed time plus time for every candidate it examines.",
			"Examine fewer candidates. Recall falls as you do (it is about 94% at 128 and 75% at 64), so stay above the 90% line. More search cores would also work, but they cost money and the budget is tight.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Halving the candidates examined will…", Options: []string{"Halve the recall, as it halves the candidates", "Cost a little recall and save a lot of time", "Make no difference, since search time is fixed"}, Answer: 1,
			Explain: "Recall climbs steeply at first and then flattens: the last candidates add little. Time grows steadily with every candidate, so the extra recall at the top is the most expensive part."}},
		Debrief: "Approximate nearest-neighbour search is a dial between recall and latency, and the top of the dial is steep: 99% recall costs far more than 94%. Pick the recall the application needs, not the maximum, and spend the saved time on capacity.",
	})

	// The GPU budget: a few huge prompts stall the one GPU the budget pays for
	gpuBudget := func(fixed bool) engine.Diagram {
		lim := P{"limitRps": 1000.0, "burst": 1000.0}
		if fixed {
			lim = P{"unit": "tokens", "limitTokensPerMin": 56000.0}
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 2.0, "readPct": 0.0, "keySpace": 10000.0, "keyDist": "uniform", "timeoutMs": 60000.0, "retries": 0.0, "requestTokens": 500.0, "bigRequestPct": 10.0}).
			n("gate", engine.KRateLimiter, cols[1], rowMid, lim).label("gate", "Token limiter").
			n("chat", engine.KAppServer, cols[2], rowMid, P{"threads": 64.0, "queueCap": 2000.0}).
			n("model", engine.KLLMServer, cols[3], rowMid, P{"gpus": 1.0, "batchSize": 32.0, "kvCacheTokens": 200000.0, "outputTokens": 60.0, "queueCap": 400.0}).label("model", "Chat model").
			e("users", "gate").e("gate", "chat").e("chat", "model").done()
	}
	register(&Scenario{
		ID: "the-gpu-budget", Title: "The GPU budget", Difficulty: "Hard", ConfigOnly: true,
		Summary: "One user in ten pastes a book, and everyone else's first word takes seconds.",
		Story:   "A chat assistant runs on the one GPU the budget pays for. Most prompts are short, but one in ten carries twenty times as many tokens (someone pasted a whole document), and reading ten thousand tokens takes the GPU two and a half seconds during which it writes nothing for anyone. The request limiter in front counts requests, so it sees nothing unusual: two a second, as always. Everyone waits for the first word, and a second GPU is not in the budget.",
		Diagram: gpuBudget(false), Solution: gpuBudget(true),
		Goals:       engine.Goals{MinSuccessRate: 0.78, MaxTTFTMs: f(3000), MaxCostPerHour: f(6.0), StarCostPerHour: f(5.5), StarP95Ms: f(100)},
		DurationSec: 120, WarmupSec: 20, Speed: 1,
		Hints: [3]string{
			"Look at the time to first token and at what the model is doing while users wait. What kind of request keeps the GPU busy?",
			"The limiter counts requests, and all requests look alike to it. The cost of a request to the GPU is its prompt: a limiter that counts tokens would charge a pasted document for what it takes.",
			"Switch the limiter's count to tokens and give it an allowance of about 56,000 tokens a minute: the short prompts fit easily, and the document-sized ones run the allowance dry and are turned away (roughly one request in ten is refused by design).",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A limiter that counts tokens instead of requests will mostly refuse…", Options: []string{"Everyone a little, to stay under the cap", "The few huge requests that use most of the tokens", "The newest users, who have no history yet"}, Answer: 1,
			Explain: "A request is let through only if its whole weight fits in what is left. A small one almost always does; a huge one needs far more than the bucket usually holds, so the heavy few are refused and the many light ones carry on."}},
		Debrief: "Capacity in an LLM service is measured in tokens, not requests: a request's cost is its prompt and its answer. Limiting by tokens (and, in practice, capping the prompt length and billing by token) keeps a few heavy requests from consuming what everyone shares.",
	})

	// Shed the cheap traffic: more arrives than the server can do, and a deep queue makes every request wait out its timeout
	shedCheap := func(fixed bool) engine.Diagram {
		app := P{"threads": 8.0, "queueCap": 2000.0}
		if fixed {
			app["shedAboveQueueMs"] = 100.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 400.0, "readPct": 100.0, "keySpace": 5000.0, "priorityPct": 20.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, app).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 32.0}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "shed-the-cheap-traffic", Title: "Shed the cheap traffic", Difficulty: "Medium", ConfigOnly: true,
		Summary: "More traffic than the server can do, and a long queue makes everyone give up.",
		Story:   "Midday traffic is 400 requests a second and the app server can finish about 250. Excess requests wait in a queue that holds 2,000 of them, and a request that waited two seconds is abandoned by its user, so most of the work is spent on answers nobody is waiting for. One request in five is marked urgent (checkouts); the rest can be asked again later.",
		Diagram: shedCheap(false), Solution: shedCheap(true),
		Goals:       engine.Goals{MinSuccessRate: 0.4, MaxP95Ms: f(500), MaxCostPerHour: f(0.65), StarCostPerHour: f(0.6), StarP95Ms: f(200)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the queue and at how long a request waits in it. Is the server busy with requests whose users are still there?",
			"Past 250 a second a longer queue only makes every request older. Turning some requests away early keeps the queue short, so the ones that are admitted are answered in time.",
			"Set 'Shed above' on the app server to a few hundred milliseconds: when the oldest waiting request is older than that, ordinary requests are turned away at the door and the urgent ones are still admitted.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Turning away some requests at the door will make the share of requests answered…", Options: []string{"Lower, as every request turned away at the door is a failure by itself", "Higher, as work is no longer wasted on requests users abandoned", "Unchanged, since the server still does the same total work"}, Answer: 1,
			Explain: "An overloaded server with a deep queue serves requests so late that their users have left: very little of its work counts. Shedding spends the same capacity on requests that are still wanted."}},
		Debrief: "Overload is not fixed by queueing more. Bound the wait, shed the requests that can be retried, and keep the ones that matter (urgent, paid, already half done) flowing; the cap on the queue's age matters more than its length.",
	})

	// URL shortener: millions of reads of short links, and every new link takes its number from one counter row
	urlShortener := func(fixed bool) engine.Diagram {
		app := P{"threads": 64.0, "queueCap": 1000.0, "dbPool": 60.0}
		b := (&bld{}).
			n("visitors", engine.KClient, cols[0], rowTop, P{"rps": 1200.0, "readPct": 100.0, "keySpace": 50000.0, "keyDist": "zipf", "zipfS": 1.1, "timeoutMs": 1500.0, "retries": 0.0}).label("visitors", "Link visitors").
			n("creators", engine.KClient, cols[0], rowBot, P{"rps": 70.0, "readPct": 0.0, "keySpace": 50000.0, "keyDist": "uniform", "hotKeyPct": 100.0, "timeoutMs": 1500.0, "retries": 0.0}).label("creators", "Link creators")
		if fixed {
			app["hotKeyMode"] = "split"
			app["splitShards"] = 8.0
			b.n("app", engine.KAppServer, cols[1], rowMid, app).
				n("cache", engine.KCache, cols[2], rowMid, P{"capacityItems": 4000.0, "ttlSec": 300.0, "startWarm": true}).
				n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 12.0, "readMs": 20.0, "writeMs": 20.0, "hotRowLock": true}).label("db", "Link database").
				e("visitors", "app").e("creators", "app").e("app", "cache").e("cache", "db").e("app", "db")
		} else {
			b.n("app", engine.KAppServer, cols[1], rowMid, app).
				n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 12.0, "readMs": 20.0, "writeMs": 20.0, "hotRowLock": true}).label("db", "Link database").
				e("visitors", "app").e("creators", "app").e("app", "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "url-shortener", Title: "The URL shortener", Difficulty: "Medium",
		Summary: "Short links are read hundreds of times for every one created, and every new link takes its number from one counter row.",
		Story:   "Visitors follow short links at 1,200 a second, a few viral ones far more than the rest, and new links are created at 70 a second. Every read goes to the database, which can answer about 600 a second at best. Each new link also takes the next number from a single counter row, and the row lock lets one such write through every 20 ms: 50 a second, while 70 are asked for. The waiting writes hold database connections that the readers need.",
		Diagram: urlShortener(false), Solution: urlShortener(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(250), MaxCostPerHour: f(3.0), StarCostPerHour: f(2.7), StarP95Ms: f(100)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"There are two different problems here, one on the read path and one on the write path. Which one do the failures come from first?",
			"Reads: a few links are followed again and again, so a cache in front of the database answers most of them. Writes: one counter row is a ceiling of one write per 20 ms however big the database.",
			"Add a cache between the app server and the database, and in the app server's hot-key setting split the counter over several rows (or hand each server its own range of numbers).",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A cache in front of the database would fix…", Options: []string{"The slow reads, but not the counter", "Both the reads and the counter", "Only the counter, which is hit most"}, Answer: 0,
			Explain: "A cache absorbs repeated reads. Creating a link is a write, and every write to the counter row still has to take its turn at the database."}},
		Debrief: "A URL shortener is read-heavy with a skewed popularity: cache the popular links and the database is left with the misses. The write path has its own trap, a single sequential ID: hand out blocks of numbers to each server, or split the counter, so creating links does not queue on one row.",
	})

	// Rate limiter showdown: a partner's job fires a burst on the second, across the edge of the limiter's window
	var edgeSpikes []engine.TrafficPoint
	edgeSpikes = append(edgeSpikes, engine.TrafficPoint{0, 100})
	for k := 2; k <= 60; k += 2 {
		at := float64(k)
		edgeSpikes = append(edgeSpikes, engine.TrafficPoint{at - 0.1, 100}, engine.TrafficPoint{at - 0.09, 8000}, engine.TrafficPoint{at + 0.09, 8000}, engine.TrafficPoint{at + 0.1, 100})
	}
	limiterShowdown := func(algo string) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 100.0, "keySpace": 2000.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("rl", engine.KRateLimiter, cols[1], rowMid, P{"limitRps": 300.0, "burst": 300.0, "algorithm": algo, "windowSec": 1.0}).
			n("app", engine.KAppServer, cols[2], rowMid, P{"threads": 32.0, "queueCap": 2000.0}).
			n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 7.0, "readMs": 20.0}).
			e("users", "rl").e("rl", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "rate-limiter-showdown", Title: "Rate limiter showdown", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The limiter promises 300 requests a second, and the database sees twice that for a moment.",
		Story:   "A partner's batch job fires a burst of requests every other second, centred on the instant the clock ticks over, on top of a steady trickle of other users. The rate limiter promises no more than 300 requests a second to a database that does about 350, yet the database is drowning for a second after every burst. The limiter counts requests in fixed one-second windows that reset on the clock: half of each burst lands at the end of one window and half at the start of the next, and each half is given a full allowance. Most of the burst has to be turned away either way.",
		Diagram: limiterShowdown("fixedWindow"), Solution: limiterShowdown("slidingWindowLog"),
		Traffic:     map[string][]engine.TrafficPoint{"users": edgeSpikes},
		Goals:       engine.Goals{MinSuccessRate: 0.2, MaxP95Ms: f(800), MaxCostPerHour: f(1.1), StarCostPerHour: f(1.0)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at what the database receives in the second around a burst, not at what the limiter allows per window.",
			"A fixed window forgets everything at its edge. A burst that straddles the edge is admitted twice, once in each window, so up to twice the limit gets through in a fraction of a second.",
			"Change the limiter's algorithm to one that never lets more than the limit through in any window-length span: the sliding window log is exact, the sliding window counter is a cheap approximation.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A burst straddling the edge of a fixed one-second window with a limit of 300 lets through up to…", Options: []string{"At most 300 requests, exactly the limit", "Up to 600 requests, double the limit", "Between 300 and 450, depending on timing"}, Answer: 1,
			Explain: "Each window has its own count: the last moments of one window and the first moments of the next can each admit the whole allowance."}},
		Debrief: "Every rate-limiting algorithm makes a different promise at the boundary. A fixed window is cheap but allows up to twice the limit across its edge; a sliding log is exact and costs memory per request; a sliding counter is cheap and close; a token bucket allows a burst up to its size and then a steady rate. Pick the one whose worst case your backend can survive.",
	})

	// Multi-region reads: app servers sit near every user, but the one database is in the US
	multiRegion := func(fixed bool) engine.Diagram {
		b := &bld{}
		regions := []string{"us", "eu", "ap"}
		names := map[string]string{"us": "US", "eu": "Europe", "ap": "Asia"}
		b.n("db", engine.KDatabase, cols[3], rowTop, P{"connections": 32.0, "readMs": 8.0, "writeMs": 15.0, "region": "us"}).label("db", "Primary database (US)")
		for i, r := range regions {
			y := float64(i)*130 + 30
			users, app := "users-"+r, "app-"+r
			b.n(users, engine.KClient, cols[0], y, P{"rps": 60.0, "readPct": 98.0, "keySpace": 500.0, "keyDist": "uniform", "timeoutMs": 3000.0, "retries": 0.0, "region": r, "consistency": "bounded", "maxStaleMs": 1000.0, "geoRouting": true}).label(users, names[r]+" users")
			b.n(app, engine.KAppServer, cols[1], y, P{"threads": 24.0, "region": r}).label(app, names[r]+" app")
			b.e(users, app).e(app, "db")
			if fixed && r != "us" {
				rep := "replica-" + r
				b.n(rep, engine.KReplica, cols[2], y, P{"connections": 16.0, "readMs": 8.0, "replicationLagMs": 100.0, "region": r}).label(rep, names[r]+" replica")
				b.e(app, rep)
			}
		}
		return b.done()
	}
	register(&Scenario{
		ID: "multi-region-reads", Title: "Multi-region reads", Difficulty: "Medium",
		Summary: "App servers are in three regions, yet reads in Europe and Asia are still slow.",
		Story:   "Last quarter the app servers moved close to the users, and every user is sent to the nearest one. The database is still a single primary in the US, so a read from Europe or Asia crosses an ocean twice to be answered, and the app server spends most of its time waiting for the answer. Reads are 98% of the traffic, and a read may be at most a second out of date.",
		Diagram: multiRegion(false), Solution: multiRegion(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(200), MaxStaleReadPct: f(0.02), MaxCostPerHour: f(2.6), StarCostPerHour: f(2.4)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Compare the latency of the three groups of users. Where does the extra time in Europe and Asia go?",
			"Every read from Europe crosses to the US and back, about 90 ms of distance before the database does anything; Asia pays 150 ms. The app servers are already close, so the data has to come closer too.",
			"Put a read replica in Europe and one in Asia, in their regions, and wire each region's app server to its replica. Reads are answered locally, writes still go to the US primary, and a replica runs a little behind, which a read that may be a second old can live with.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "A read replica in Asia will make reads there faster, and writes from Asia…", Options: []string{"Faster as well, since the replica is closer", "No faster: they still go to the primary", "Slower, as the replica must also copy them"}, Answer: 1,
			Explain: "A replica serves reads only. Every write still has to reach the one primary, so a write from Asia keeps paying the distance."}},
		Debrief: "Replicas near the readers fix read latency; they do not move the writes, and they add replication lag that grows with the distance to the primary (here about 190 ms to Europe and 250 ms to Asia). Moving writes close too needs several leaders, which brings conflicts, or sharding users by home region.",
	})

	// Add a shard: capacity is added under load, and how keys are placed decides how many requests it slows
	addShard := func(fixed bool) engine.Diagram {
		hashing := "mod"
		if fixed {
			hashing = "ring"
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 100.0, "keySpace": 4000.0, "keyDist": "uniform", "timeoutMs": 5000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 48.0, "queueCap": 2000.0, "dbPool": 200.0}).
			n("shards", engine.KShardRouter, cols[2], rowMid, P{"hashing": hashing, "vnodes": 128.0, "rebalanceSec": 10.0, "rebalanceMs": 400.0}).
			e("users", "app").e("app", "shards")
		for k := 0; k < 4; k++ {
			id := fmt.Sprintf("db%d", k+1)
			b.n(id, engine.KDatabase, cols[3], float64(k)*100+30, P{"connections": 32.0, "readMs": 5.0}).e("shards", id)
		}
		return b.done()
	}
	register(&Scenario{
		ID: "add-a-shard", Title: "Adding a shard under load", Difficulty: "Medium", ConfigOnly: true,
		Summary: "More capacity, and for ten seconds everything gets slower.",
		Story:   "The four-shard database is getting full, so a fifth shard is added at 20 seconds while traffic keeps flowing. The router places each key on a shard by hashing it and taking the remainder after dividing by the number of shards, so adding one changes where almost every key lives. For ten seconds the data of every moved key is still travelling, and each request for one of them takes 400 ms longer.",
		Diagram: addShard(false), Solution: addShard(true),
		Chaos:       []engine.ChaosEvent{{Type: "addNode", NodeID: "shards", AtSec: 20}},
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(300), MaxCostPerHour: f(4.2), StarCostPerHour: f(4.0), StarP95Ms: f(200)},
		DurationSec: 60, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Watch the router when the new shard arrives: how many keys moved, and how many requests were slowed by the move?",
			"Placing a key by hash mod n changes the home of about four keys in five when n goes from 4 to 5. About 80% of requests wait for data that is still moving.",
			"Place keys on a consistent-hashing ring instead (key placement on the shard router): a new shard takes over only about a fifth of the keys, so only about a fifth of the requests are slowed, and for a shorter time than the p95 notices.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Adding a fifth shard to a router that places keys by hash mod 4 moves about…", Options: []string{"About a fifth of the keys", "About four fifths of the keys", "None: the new shard starts empty"}, Answer: 1,
			Explain: "Changing the divisor changes the remainder for almost every key. A ring only reassigns the arc the new shard takes over, about one key in five."}},
		Debrief: "Hash mod n makes resizing a cluster as expensive as rebuilding it. Consistent hashing moves only about 1/n of the keys when a node joins or leaves, which is why caches and key-value stores use it.",
	})

	// Heavy tail: identical replicas whose service times have a long tail; capacity cannot shorten it, a second ask can
	heavyTail := func(fixed bool) engine.Diagram {
		hedge := 0.0
		if fixed {
			hedge = 80
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 150.0, "readPct": 100.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 3000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 48.0, "queueCap": 2000.0, "dbPool": 100.0, "readSpread": "roundRobin", "hedgeReadsMs": hedge}).
			n("db", engine.KDatabase, cols[3], 20, P{"connections": 16.0}).
			e("users", "app").e("app", "db")
		for k := 0; k < 3; k++ {
			id := fmt.Sprintf("replica%d", k+1)
			b.n(id, engine.KReplica, cols[2], float64(k)*100+60, P{"connections": 16.0, "readMs": 40.0, "variability": "heavy", "replicationLagMs": 50.0}).e("app", id)
		}
		return b.done()
	}
	register(&Scenario{
		ID: "heavy-tail", Title: "The slow one in a hundred", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Three healthy read replicas, and one read in twenty is far slower than the rest.",
		Story:   "A product page reads from three identical read replicas, taking turns. Most reads take a few milliseconds, but the replicas have a long tail: now and then one read takes ten times as long, whichever replica serves it. The median is great. The slowest one in twenty is what customers feel, and adding threads or servers has not moved it.",
		Diagram: heavyTail(false), Solution: heavyTail(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(230), MaxCostPerHour: f(3.0), StarCostPerHour: f(2.8), StarP95Ms: f(190)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the latency distribution: the slow reads are the replicas' own long tail, not a queue. Are the threads or the replicas busy?",
			"Adding capacity does not shorten a slow read, because the slow read is slow by itself. But the other two replicas are fine: the same read would almost certainly be quick if asked of another.",
			"Hedge the reads (advanced setting on the app server): if a read has not been answered after a short wait, ask another replica too and take the first answer. Wait about the time a normal read takes; hedging too early doubles the load for nothing.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Hedging reads after 80 ms sends a second request for…", Options: []string{"Every read, so the load roughly doubles", "Only the slow ones, a small share of reads", "None, as it only works when a replica is down"}, Answer: 1,
			Explain: "Most reads finish before the wait is over, so only the few that are still waiting get a second request, a modest increase in load for a large cut in the tail."}},
		Debrief: "Hedged requests trade a little extra load for a much shorter tail: the chance that two independent replicas are both slow is the square of the chance that one is. Wait about the p95 of a normal request before hedging, or you simply double your traffic.",
	})

	// Noisy tenant: one customer of a shared service sends most of the traffic; a single shared limit punishes everyone
	noisyTenant := func(fixed bool) engine.Diagram {
		keyBy := "none"
		if fixed {
			keyBy = "tenant"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 300.0, "readPct": 100.0, "identities": 100.0, "identitySkew": 1.2, "tenants": 4.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("rl", engine.KRateLimiter, cols[1], rowMid, P{"limitRps": 100.0, "burst": 20.0, "keyBy": keyBy}).
			n("app", engine.KAppServer, cols[2], rowMid, P{"threads": 32.0, "queueCap": 1000.0}).
			n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 32.0}).
			e("users", "rl").e("rl", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "noisy-tenant", Title: "The noisy tenant", Difficulty: "Medium", ConfigOnly: true,
		Summary: "One customer is sending most of the traffic, and the others are the ones who suffer.",
		Story:   "A shared API serves four customers (tenants). One of them runs a heavy integration and sends most of the requests. To protect the servers there is a rate limit of 100 requests a second for everyone together. The heavy tenant uses it all up; the three quiet tenants, who never come near a fair share, get errors.",
		Diagram: noisyTenant(false), Solution: noisyTenant(true),
		Goals:       engine.Goals{MinSuccessRate: 0.45, MinWorstTenant: f(0.95), MaxP95Ms: f(400), MaxCostPerHour: f(1.6), StarCostPerHour: f(1.4)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at who is being rejected: the goals show how the worst-served quiet tenant is doing, not just the total.",
			"One limit for all traffic together is first come, first served: whoever sends the most takes the most. A higher shared limit helps the quiet tenants only by also letting the noisy one send even more.",
			"Make the rate limiter count per tenant (Limit per: each tenant): every tenant gets its own allowance, so the noisy one runs out without turning anyone else away.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Doubling the shared limit to 200 requests a second would…", Options: []string{"Fix it for the quiet tenants, who get twice the room", "Help a little, but the noisy tenant still takes more", "Make no difference, since the noisy tenant is capped"}, Answer: 1,
			Explain: "A shared limit divides capacity in proportion to demand, so the loudest tenant keeps getting the most. A limit per tenant divides it fairly."}},
		Debrief: "A global rate limit protects the servers but not the tenants from each other. Limit per customer so one customer's burst runs into its own allowance, and add a shared limit on top if the total must be bounded too.",
	})

	// Notification fan-out: one event must reach email, push and SMS. A queue gives each message to one worker only
	fanOut := func(fixed bool) engine.Diagram {
		kind := engine.KQueue
		if fixed {
			kind = engine.KTopic
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 40.0, "readPct": 0.0, "keySpace": 1000.0, "timeoutMs": 3000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("events", kind, cols[2], rowMid, P{"capacity": 50000.0}).
			n("log", engine.KDatabase, cols[4], rowMid, P{"connections": 16.0}).label("log", "Delivery log").
			e("users", "app").e("app", "events")
		for k, name := range []string{"Email", "Push", "SMS"} {
			id := fmt.Sprintf("ch%d", k+1)
			b.n(id, engine.KWorker, cols[3], float64(k)*110+60, P{"concurrency": 4.0, "processMs": 30.0}).label(id, name+" sender").e("events", id).e(id, "log")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "notification-fan-out", Title: "Tell everyone", Difficulty: "Medium",
		Summary: "An order is placed, and only one of the three notifications goes out.",
		Story:   "Every order has to produce an email, a push notification and an SMS, each sent by its own worker. The events go through a message queue, and the three workers all read from it. Orders are accepted and nothing errors, but each order produces just one notification: whichever worker got to the message first.",
		Diagram: fanOut(false), Solution: fanOut(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MinDeliveriesPerMessage: f(2.9), MaxP95Ms: f(400), MaxCostPerHour: f(1.4), StarCostPerHour: f(1.2)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the goals: deliveries per message. How many copies of each order does the system produce, and how many does it need?",
			"A queue hands each message to exactly one of the workers reading it, so the three workers share the work instead of each doing their own. Adding more workers makes the shared work faster, not wider.",
			"Replace the queue with a topic (publish/subscribe): every worker connected to it is a subscriber and gets its own copy of every message.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "If email, push and SMS are three workers on one queue, an order produces…", Options: []string{"Three notifications, one from each worker", "One notification, from whichever worker took it", "None until all three workers have finished"}, Answer: 1,
			Explain: "Workers on a queue compete for messages: each message is processed once. To give every consumer its own copy you need publish/subscribe."}},
		Debrief: "A queue is for sharing work among identical workers; a topic is for telling several different consumers about the same event. The two look alike on a diagram and behave very differently.",
	})

	// Go global: users on three continents, one stack in one region
	goGlobal := func(fixed bool) engine.Diagram {
		b := &bld{}
		regions := []string{"us", "eu", "ap"}
		names := map[string]string{"us": "US", "eu": "Europe", "ap": "Asia"}
		for i, r := range regions {
			b.n("users-"+r, engine.KClient, cols[0], float64(i)*120+30, P{"rps": 40.0, "readPct": 100.0, "timeoutMs": 3000.0, "retries": 0.0, "region": r, "geoRouting": fixed}).label("users-"+r, names[r]+" users")
		}
		stacks := []string{"us"}
		if fixed {
			stacks = regions
		}
		for i, r := range stacks {
			lb, app, db := "lb-"+r, "app-"+r, "db-"+r
			b.n(lb, engine.KLoadBalancer, cols[1], float64(i)*120+30, P{"region": r}).label(lb, names[r]+" balancer")
			b.n(app, engine.KAppServer, cols[2], float64(i)*120+30, P{"threads": 24.0, "region": r}).label(app, names[r]+" app")
			b.n(db, engine.KDatabase, cols[3], float64(i)*120+30, P{"connections": 16.0, "region": r}).label(db, names[r]+" database")
			b.e(lb, app).e(app, db)
			for _, u := range regions {
				b.e("users-"+u, lb)
			}
		}
		return b.done()
	}
	register(&Scenario{
		ID: "go-global", Title: "Go global", Difficulty: "Medium",
		Summary: "Customers in Europe and Asia find the product slow, and the servers are fine.",
		Story:   "The whole stack runs in the US. Customers in Europe and Asia get the same servers, the same database and the same code, but every request crosses an ocean twice, and the speed of light is not a setting you can change. The product team wants a snappy experience for everyone.",
		Diagram: goGlobal(false), Solution: goGlobal(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(180), MaxCostPerHour: f(3.0), StarCostPerHour: f(2.6), StarP95Ms: f(160)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Compare the latency of the three groups of users. What explains the difference when the servers are equally fast?",
			"Distance. A request from Asia to the US pays about 75 ms each way before any work is done. More capacity cannot shorten that: the stack has to be closer to the users.",
			"Run a stack (balancer, app, database) in each region, connect every user group to all of them, and turn on 'Send users to the nearest region' for the users. Remember the stacks cost money too.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Adding more servers in the US would make users in Asia…", Options: []string{"Much faster, as the queues in the US shrink", "No faster: the delay is the distance, not the load", "Slower, since the US servers then take more of the traffic"}, Answer: 1,
			Explain: "Latency to a far region is dominated by the distance each way. Capacity helps when servers are busy, not when the wait is the speed of light."}},
		Debrief: "To serve a global audience, put the stack near the users and route them to it. The cost is running several copies, and the hard part moves to keeping their data consistent (which the multi-region database challenges are about).",
	})

	// Payment saga: checkout spans stock, card and shipping; a failed step must not leave a half-done order
	paymentSaga := func(fixed bool) engine.Diagram {
		mode := "naive"
		if fixed {
			mode = "saga"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 50.0, "readPct": 0.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 4000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "queueCap": 1000.0, "dbPool": 50.0, "workflow": mode}).
			n("stock", engine.KDatabase, cols[2], 40, P{"connections": 16.0, "writeMs": 10.0}).label("stock", "Stock database").
			n("card", engine.KExternalAPI, cols[2], 160, P{"latencyMs": 40.0, "errorPct": 15.0}).label("card", "Card processor").
			n("ship", engine.KExternalAPI, cols[2], 280, P{"latencyMs": 30.0, "errorPct": 0.0}).label("ship", "Shipping service").
			e("users", "app").e("app", "stock").e("app", "card").e("app", "ship").done()
	}
	register(&Scenario{
		ID: "payment-saga", Title: "The half-finished order", Difficulty: "Hard", ConfigOnly: true,
		Summary: "When the card is declined, the stock stays reserved and nobody gets the item.",
		Story:   "Checkout does three things in a row: reserve the item in the stock database, charge the card, and book shipping. The card processor declines about one payment in seven. When it does, the order fails, but the item stays reserved forever: stock slowly drains into orders that never complete. Finance cannot reconcile either.",
		Diagram: paymentSaga(false), Solution: paymentSaga(true),
		Goals:       engine.Goals{MinSuccessRate: 0.8, MaxInconsistent: i(0), MaxP95Ms: f(500), MaxCostPerHour: f(1.6), StarCostPerHour: f(1.4)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Count the orders left half done on the goals panel. A declined card is a normal, expected failure: what should happen to the stock that was already reserved?",
			"Nothing undoes the reservation, because the steps are just run in a row. Retrying the whole order does not help: it would reserve more stock each time.",
			"Run the checkout as a saga (the app server's setting for writes that span services): when a later step fails, compensating calls undo the earlier steps, newest first. A few percent of orders still fail (declines are real), but none are left half done.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "With a saga, will fewer orders fail?", Options: []string{"Yes: the failed steps are retried until they pass", "No: the same orders fail, but each is cleanly rolled back", "Yes: the card processor declines fewer orders"}, Answer: 1,
			Explain: "A saga does not make steps succeed. It makes a failure safe by running the opposite of every earlier step, so the system ends up consistent."}},
		Debrief: "Without a distributed transaction, consistency across services comes from compensation: every step has an undo, and a failure runs the undos backwards. Compensations must be possible, repeatable (idempotent), and watched: one that fails leaves a stuck order for a person.",
	})

	// Frozen checkout: two-phase commit across stock, card and shipping, and the server dies between the votes and the decision
	frozenCheckout := func(fixed bool) engine.Diagram {
		mode := "twoPhase"
		if fixed {
			mode = "saga"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 50.0, "readPct": 0.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 4000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "queueCap": 1000.0, "dbPool": 50.0, "workflow": mode}).
			n("stock", engine.KDatabase, cols[2], 40, P{"connections": 16.0, "writeMs": 10.0}).label("stock", "Stock database").
			n("card", engine.KExternalAPI, cols[2], 160, P{"latencyMs": 40.0, "errorPct": 5.0}).label("card", "Card processor").
			n("ship", engine.KExternalAPI, cols[2], 280, P{"latencyMs": 30.0, "errorPct": 0.0}).label("ship", "Shipping service").
			e("users", "app").e("app", "stock").e("app", "card").e("app", "ship").done()
	}
	register(&Scenario{
		ID: "frozen-checkout", Title: "The frozen checkout", Difficulty: "Hard", ConfigOnly: true,
		Summary: "The checkout server dies mid-order and the stock rows stay locked until it comes back.",
		Story:   "To be sure no order is ever half done, checkout runs as a two-phase commit: the stock database, the card processor and shipping each vote yes and hold their part, then the server tells all of them to commit. It works until the server dies in the gap between the votes and the decision. The services that voted cannot decide alone, so they sit locked, waiting for a server that is not coming back for a while. Operations wants no order left waiting on a dead coordinator.",
		Diagram: frozenCheckout(false), Solution: frozenCheckout(true),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "app", AtSec: 15, DownSec: 8}},
		Goals:       engine.Goals{MinSuccessRate: 0.65, MaxInDoubt: i(0), MaxInconsistent: i(0), MaxP95Ms: f(500), MaxCostPerHour: f(1.6), StarCostPerHour: f(1.4)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the goals panel when the server crashes: how many orders are left waiting, and for whom?",
			"The services voted yes and promised to wait for the decision. If the coordinator dies, nobody can tell them to commit or abort. Waiting costs locks held on the stock rows.",
			"Run the checkout as a saga instead (the app server's setting for writes that span services): no service holds anything across steps, and a failure is undone by compensating calls.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Two-phase commit guarantees nothing is ever half done. What does it cost?", Options: []string{"Nothing: it is strictly better than a saga", "A second round of calls, and a stall if the coordinator dies", "Lost data, since a crash discards the votes"}, Answer: 1,
			Explain: "It buys atomicity with latency and blocking: every service holds its part until the decision arrives, and a coordinator that dies between the votes and the decision leaves them all stuck."}},
		Debrief: "Two-phase commit is atomic but blocking: its weak point is the coordinator. Production systems replicate the coordinator, or avoid distributed transactions and accept eventual consistency with sagas, where each step is a small local transaction and failures are compensated.",
	})

	// Reconnect storm: a server restarts and every user it held comes back in the same instant
	reconnect := func(fixed bool) engine.Diagram {
		jitter := 0.0
		if fixed {
			jitter = 30
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"arrival": "sessions", "connections": 1500.0, "sessionSec": 900.0, "msgsPerMin": 6.0, "reconnectJitterSec": jitter, "readPct": 100.0, "timeoutMs": 1000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0, "cpuMs": 5.0, "handshakeMs": 30.0, "queueCap": 2000.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 32.0}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "reconnect-storm", Title: "The reconnect storm", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The server comes back in seconds, and the outage lasts minutes.",
		Story:   "A chat server holds 1,500 live connections. It restarts for a few seconds, which is fine, but when it returns every client reconnects in the same instant. Setting up a connection costs real work, so the server cannot keep up, the clients give up and try again together, and it never catches its breath.",
		Diagram: reconnect(false), Solution: reconnect(true),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "app", AtSec: 15, DownSec: 3}},
		Goals:       engine.Goals{MinSuccessRate: 0.95, MaxP95Ms: f(400), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.1), StarP95Ms: f(200)},
		DurationSec: 60, WarmupSec: 10, Speed: 1,
		Hints: [3]string{
			"Watch the new-connections rate on the app server after the restart. How big is the spike, and how many connections can it set up per second?",
			"16 threads at 30 ms a handshake is about 500 connections a second, and 1,500 clients arrive in one go. Those that time out try again at the same moment.",
			"Spread the reconnects: give clients a random wait before they come back (the reconnect spread on the client). More servers would also work but cost more.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Spreading reconnects over 30 seconds will…", Options: []string{"Make the outage 30 seconds longer for everyone", "Turn one huge spike into a steady trickle", "Do nothing, as the same clients reconnect"}, Answer: 1,
			Explain: "The same number of connections are set up, but the peak rate falls from the whole population at once to about fifty a second."}},
		Debrief: "A restart is not the end of the incident: every client retrying in lockstep can keep the server down. Add random jitter (and backoff) to reconnects so the herd arrives as a stream.",
	})

	// The bad deploy: version 2 is rolled out to half the fleet at once and it serves errors
	badDeploy := func(fixed bool) engine.Diagram {
		canary := 0.0
		if fixed {
			canary = 5
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 80.0, "readPct": 100.0, "timeoutMs": 3000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, P{"healthCheckSec": 1.0, "canaryPct": canary}).
			n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 64.0}).
			e("users", "lb")
		for k := 0; k < 4; k++ {
			id := fmt.Sprintf("app%d", k+1)
			v := "v1"
			if k >= 2 {
				v = "v2"
			}
			b.n(id, engine.KAppServer, cols[2], float64(k)*100+30, P{"threads": 16.0, "version": v}).e("lb", id).e(id, "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "bad-deploy", Title: "The bad deploy", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Half the fleet runs the new version, and the new version is broken.",
		Story:   "Release day: two of the four servers already run version 2. Twelve seconds into the evening peak version 2 starts answering every request with an error, and it stays that way for half a minute before anyone rolls it back. Because the load balancer treats all four servers alike, half of all requests fail.",
		Diagram: badDeploy(false), Solution: badDeploy(true),
		Chaos:       []engine.ChaosEvent{{Type: "badDeploy", Kind: engine.KAppServer, Version: "v2", FractionPct: 100, Mode: "errors", AtSec: 12, DurationSec: 30}},
		Goals:       engine.Goals{MinSuccessRate: 0.92, MaxP95Ms: f(400), MaxCostPerHour: f(2.4), StarCostPerHour: f(2.3)},
		DurationSec: 50, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"You cannot stop the bug, but you can choose how many people meet it. Who decides how much traffic version 2 receives?",
			"The load balancer sends each server its fair share. Version 2 holds two servers out of four, so it gets half of the traffic.",
			"Give the load balancer a canary share (advanced settings): a small percentage of requests goes to version 2 until you trust it.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "With a 5% canary, roughly how many requests meet the bug?", Options: []string{"About half, as the canary is checked first", "About 5% of them, during the bad period", "Almost none, as a canary is always tested"}, Answer: 1,
			Explain: "A canary sends the new version only its small share, so a broken release hurts only that share until it is rolled back."}},
		Debrief: "Release safety is about blast radius. Rolling to half the fleet at once exposes half your users to every bug; a canary exposes a few, lets you watch the numbers, and only then widens the rollout.",
	})

	// Zone outage: every component sits in the default zone, so one zone failing is a total outage
	zoneOutage := func(fixed bool) engine.Diagram {
		zl, za, zb, zc, zd := "az-a", "az-a", "az-a", "az-a", "az-a"
		if fixed {
			zl, za, zb, zc, zd = "az-b", "az-a", "az-b", "az-c", "az-c"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 60.0, "readPct": 100.0, "timeoutMs": 2000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, P{"healthCheckSec": 1.0, "zone": zl}).
			n("app1", engine.KAppServer, cols[2], rowTop, P{"threads": 16.0, "zone": za}).
			n("app2", engine.KAppServer, cols[2], rowMid, P{"threads": 16.0, "zone": zb}).
			n("app3", engine.KAppServer, cols[2], rowBot, P{"threads": 16.0, "zone": zc}).
			n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 64.0, "zone": zd}).
			e("users", "lb").e("lb", "app1").e("lb", "app2").e("lb", "app3").e("app1", "db").e("app2", "db").e("app3", "db").done()
	}
	register(&Scenario{
		ID: "zone-outage", Title: "When a zone goes dark", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Three servers, a balancer and a database, and one data-centre zone fails.",
		Story:   "Everything runs in zone A of the region. At twelve seconds into the peak the whole zone loses power for twenty seconds: the load balancer, the three app servers and the database go with it. The architecture has plenty of redundancy on paper, but all of it sits in one place.",
		Diagram: zoneOutage(false), Solution: zoneOutage(true),
		Chaos:       []engine.ChaosEvent{{Type: "zoneDown", Zone: "az-a", AtSec: 12, DurationSec: 20}},
		Goals:       engine.Goals{MinSuccessRate: 0.9, MaxP95Ms: f(400), MaxCostPerHour: f(2.1), StarCostPerHour: f(2.0)},
		DurationSec: 45, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Which components share the failed zone? Select a few and look at their placement setting.",
			"Redundancy only helps if the copies do not fail together. Spreading the servers is not enough if the balancer or the database stays in the zone that fails.",
			"Set each component's availability zone (advanced settings): keep the balancer and the database out of zone A and spread the servers over different zones.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Why does the design survive when the balancer is moved to another zone but the servers stay put?", Options: []string{"It does not: all three servers are still in the failed zone", "The balancer caches responses from the servers", "The database takes over when the zone fails"}, Answer: 0,
			Explain: "A balancer in a healthy zone has nothing to send traffic to if every server it knows is in the failed zone. All the pieces a request needs must survive."}},
		Debrief: "Failure domains are only useful if you actually use them: spread replicas across zones, and put the single points (balancer entry, primary database) where a zone failure will not take them with it. Cross-zone calls cost about a millisecond or two, which is cheap insurance.",
	})

	// Retry storm: a five-second slowdown that never ends, because the clients' retries keep the database saturated
	retryStorm := func(fixed bool) engine.Diagram {
		budget := 0.0
		if fixed {
			budget = 10
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 250.0, "readPct": 0.0, "timeoutMs": 400.0, "retries": 3.0, "backoff": "none", "retryBudgetPct": budget}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 128.0, "queueCap": 2000.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "writeMs": 40.0, "queueCap": 2000.0}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "retry-storm", Title: "The retry storm", Difficulty: "Hard", ConfigOnly: true,
		Summary: "A five-second blip, and the site never recovers.",
		Story:   "The database slows down for five seconds. Each client gives up after 400 ms and tries again, up to three more times, immediately. By the time the database is healthy again it is buried under the retries of requests that timed out, which time out in turn and are retried again. Fifty seconds later it is still down.",
		Diagram: retryStorm(false), Solution: retryStorm(true),
		Chaos:       []engine.ChaosEvent{{Type: "slow", NodeID: "db", AtSec: 10, Factor: 3, DurationSec: 5}},
		Goals:       engine.Goals{MinSuccessRate: 0.7, MaxP95Ms: f(400), MaxCostPerHour: f(2.7), StarCostPerHour: f(2.5), StarP95Ms: f(300)},
		DurationSec: 60, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at the success rate over time: it drops during the blip and then stays down long after the database is healthy. What is still arriving?",
			"Every timeout becomes up to three more requests, so the load is several times what the users really send. The database cannot ever catch up.",
			"Limit the damage retries can do: give the users a retry budget (advanced settings), or reduce the retries. A retry budget keeps retrying cheap failures but stops a storm.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Why does adding exponential backoff to the retries not fix it here?", Options: []string{"It does: backoff always prevents retry storms", "The load stays several times the real traffic", "Backoff makes the database slower to answer"}, Answer: 1,
			Explain: "Backoff spreads retries out but does not reduce how many there are. A cap on the total (a budget) is what keeps the extra load bounded."}},
		Debrief: "Retries multiply load exactly when a dependency is weakest. Keep them bounded: a retry budget as a share of normal traffic, backoff with jitter, and a limit on attempts. Without that, a short blip turns into a long outage.",
	})

	// Bot flood: a scraper on three accounts, and a limiter that cannot tell it from a customer
	botFlood := func(fixed bool) engine.Diagram {
		keyBy, limit, burst := "none", 150.0, 50.0
		if fixed {
			keyBy, limit, burst = "user", 5.0, 5.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowTop, P{"rps": 100.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0, "identities": 1000.0}).label("users", "Customers").
			n("bot", engine.KClient, cols[0], rowBot, P{"rps": 600.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0, "abuse": true, "identities": 3.0}).label("bot", "Scraper").
			n("rl", engine.KRateLimiter, cols[1], rowMid, P{"limitRps": limit, "burst": burst, "keyBy": keyBy}).
			n("app", engine.KAppServer, cols[2], rowMid, P{"threads": 64.0, "queueCap": 500.0}).
			n("cache", engine.KCache, cols[3], rowMid, P{"capacityItems": 1000.0, "ttlSec": 0.0, "startWarm": true}).
			n("db", engine.KDatabase, cols[4], rowMid, P{"connections": 16.0, "readMs": 20.0, "queueCap": 500.0}).
			e("users", "rl").e("bot", "rl").e("rl", "app").e("app", "cache").e("cache", "db").done()
	}
	register(&Scenario{
		ID: "bot-flood", Title: "The bot flood", Difficulty: "Medium", ConfigOnly: true,
		Summary: "A scraper is burying the database, and the limiter keeps turning customers away.",
		Story:   "A scraper asks for item after item nobody has cached, at 600 requests a second from just three accounts, on top of 1,000 customers who send 100 a second between them. The database is saturated. Someone put a rate limiter in front of everything, but it counts all traffic together, so customers are rejected right along with the bot. Only the customers' experience counts here.",
		Diagram: botFlood(false), Solution: botFlood(true),
		Goals:       engine.Goals{MinCustomerSuccess: f(0.97), MaxCostPerHour: f(2.8), StarCostPerHour: f(2.6), StarP95Ms: f(300)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Compare how the customers fare with how the scraper fares. The limiter sees only a stream of requests: what does it count?",
			"The scraper sends 600 requests a second from three accounts; a customer sends about one every ten seconds. One shared allowance cannot tell them apart.",
			"Make the limiter count per user (advanced settings) and set the allowance to what a customer plausibly needs, a few requests a second.",
		},
		Predictions: []Prediction{{ID: "p1", Question: "Why does raising the shared limit not fix it?", Options: []string{"The customers become slower because limits add delay", "The bot's extra requests also get through to the database", "The bot just opens more accounts to get around it"}, Answer: 1,
			Explain: "A shared limit that is high enough for the customers lets most of the bot through too. The two have to be told apart by who they are."}},
		Debrief: "A global limit protects the system but not the people. Limit per identity so a heavy or abusive account runs out of allowance on its own, and size the per-user rate to legitimate use rather than to total capacity.",
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
		Predictions: []Prediction{{ID: "p1", Question: "A queue accepts messages up to 256 KB. Users upload 500 KB files. What is the standard fix?", Options: []string{"Raise the queue's size limit past 500 KB", "Retry the failed uploads in smaller pieces", "Store the file elsewhere and queue a pointer to it"}, Answer: 2,
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
		Predictions: []Prediction{{ID: "p1", Question: "A CDN with a 60 second TTL serves a page that changes every few seconds. What is the catch?", Options: []string{"Users may see stale copies until the TTL expires", "The CDN cannot serve pages that change at all", "Every request still has to reach the origin"}, Answer: 0,
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
		Predictions: []Prediction{{ID: "p1", Question: "Why does a least-connections balancer help when one server is slow?", Options: []string{"A slow server accumulates open requests, so it is picked less", "It makes the slow server faster by easing its load", "It restarts the slow server automatically"}, Answer: 0,
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
		Predictions: []Prediction{{ID: "p1", Question: "A shard router picks the database from the item's key. What goes wrong if a few keys are far hotter than the rest?", Options: []string{"One shard can get most of the traffic", "Nothing: sharding balances load perfectly", "Writes slow down because they become reads"}, Answer: 0,
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
		Predictions: []Prediction{{ID: "p1", Question: "The database becomes four times slower for twelve seconds. Which setting stops the slowdown from taking the whole system down?", Options: []string{"A call timeout on the app servers", "More threads on each app server", "Longer timeouts on the clients"}, Answer: 0,
			Explain: "A timeout frees the thread instead of leaving it stuck behind the slow database, so servers keep answering what they can (from cache, for example)."}},
		Debrief: "Real outages stack: capacity, a slow dependency and a failure arrive together. The designs that survive layer defences: caches to cut load, redundancy for failures, and timeouts and short queues so slowness cannot spread.",
	})
}
