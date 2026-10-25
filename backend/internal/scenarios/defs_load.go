package scenarios

import (
	"sysdesign/internal/engine"
)

// registerLoadAndLiveData registers slow vendors, write floods, crashes, and the live scoreboard and location challenges.
func registerLoadAndLiveData() {
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
		Story:   "Every write calls an email vendor that takes almost half a second. {{thread-pool|Threads}} pile up waiting, and even reads, which never touch the vendor, are starved.",
		Diagram: vendor(false), Solution: vendor(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(400), MinJobsCompleted: f(0.99), MaxJobP95Ms: f(20000), MaxCostPerHour: f(1.4)},
		DurationSec: 50, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The user does not need to wait for the email to be sent.",
			"Decouple the slow call: acknowledge the request now, do the vendor work later.",
			"What sits between the app servers and the vendor to take the slow call off the request path, and what then does the vendor work?",
		},
		Answer: "The answer: move the vendor call off the request path: app servers → queue → worker → vendor.",
		Predictions: []Prediction{{ID: "p1", Question: "If the vendor's latency doubles, what happens to the user once the vendor call is behind a queue?", Options: []string{"User latency doubles, since the vendor is still called", "Users see no change; the backlog grows instead", "Requests fail, as the queue cannot hold them"}, Answer: 1,
			Explain: "The queue absorbs the slowness. Users get a quick acknowledgement while workers fall behind."}},
		Debrief: "Anything slow, unreliable or non-essential for the response belongs behind a queue. You trade immediacy for resilience and must watch the backlog.",
		Deeper:  "A queue turns a latency problem into a backlog problem: users no longer wait for the vendor, but the age of the oldest message now tells you whether you are keeping up, so that is the number to alert on rather than the error rate. Workers need bounded concurrency, backoff and {{dead-letter-queue|somewhere for hopeless jobs to go}}, otherwise a vendor outage grows the queue and the recovery arrives as a burst that knocks the vendor over again. {{exactly-once-delivery|Queues deliver at least once}}, so the vendor call must be {{idempotency|idempotent}} or the user gets two emails. An interviewer will ask what the user is told while the email is late, and how you drain a backlog without flattening the vendor. The classic failure is the gap between committing the write and enqueueing the job, which silently drops the side effect; a transactional outbox closes it.",
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
		Story:   "Writes arrive at 300 per second for a short burst, but the database can only {{throughput|commit about 250 per second}}.",
		Diagram: spike(false), Solution: spike(true),
		Traffic:     map[string][]engine.TrafficPoint{"users": {{0, 100}, {10, 100}, {12, 300}, {32, 300}, {34, 100}, {50, 100}}},
		Goals:       engine.Goals{MinSuccessRate: 0.99, MinJobsCompleted: f(0.99), MaxJobP95Ms: f(40000), MaxCostPerHour: f(1.2)},
		DurationSec: 50, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The burst is temporary. Could the writes wait in line instead of hitting the database all at once?",
			"A queue plus workers lets the database work at its own pace.",
			"What could sit between the app server and the database to hold the writes that arrive faster than the database can take them, and what must then drain it?",
		},
		Answer: "The answer: insert a message queue and a worker between the app server and the database.",
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
			"If one server fails, how much must the others carry? Divide that by what one server handles and round up.",
		},
		Answer: "The answer: run enough servers that the survivors can carry 350 rps, for example four 8-thread servers.",
		Predictions: []Prediction{{ID: "p1", Question: "Why do a few requests still fail even in a well-sized system?", Options: []string{"The balancer takes a moment to notice the failure", "The database drops connections when a peer dies", "Clients keep the dead server's address cached"}, Answer: 0,
			Explain: "Health checks run periodically, so for a couple of seconds the balancer still sends traffic to a dead server."}},
		Debrief: "Redundancy only helps if the survivors have {{utilisation|headroom}}. Shorter health-check intervals shrink the blip, at the price of more checking.",
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
			"Which kind of component keeps data sorted in memory and answers rank queries in microseconds, and who must be connected to it?",
		},
		Answer: "The answer: add Redis and connect every app server to it.",
		Predictions: []Prediction{{ID: "p1", Question: "Redis runs commands on one thread. Is that a problem at 300 requests per second?", Options: []string{"Yes: commands are queued one at a time", "No: each command takes only microseconds", "Only for writes, as reads run in parallel"}, Answer: 1,
			Explain: "Cheap commands finish in about a tenth of a millisecond, so one thread handles thousands per second."}},
		Debrief: "Use the data structure that matches the question. Sorted sets make 'who is in the top 10' cheap where a relational sort is not.",
		Deeper:  "A sorted set answers rank and top-N in logarithmic time because the data is kept in the order the question asks for, which a relational sort recomputes on every read. The price is a second copy of the truth in memory, so you must decide which side wins after a partial failure and how Redis is rebuilt when it is lost. At larger scale one single-threaded instance becomes the ceiling, and you {{sharding|shard}} by board or season, or accept approximate ranks for very large boards. An interviewer will ask about ties, about friends-only boards that cannot use one global set, and about what happens to scores written while Redis was down. The common failure is drift: the database and the set disagree after a dropped write and nobody notices until a player complains.",
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
			"The database is scanning everything for a proximity query. Which database setting organises points by location?",
		},
		Answer: "The answer: turn on the spatial index in the database settings.",
		Predictions: []Prediction{{ID: "p1", Question: "A spatial index mainly improves…", Options: []string{"Write throughput", "Proximity searches", "Network latency"}, Answer: 1,
			Explain: "Spatial indexes organise points by location so a proximity query touches only nearby entries."}},
		Debrief: "The cheapest scaling fix is often a better index. Measure first: the database was doing 25× the work for every search.",
		Deeper:  "A spatial index (a grid, geohash prefixes, a quadtree or an R-tree) lets a proximity query visit only the cells around the rider instead of every row, which is why the work falls by an order of magnitude here. Moving drivers make the write side the real cost: every location update must maintain the index, so many systems keep live positions in memory grouped by cell and store history separately. A query near a cell edge has to look at the neighbouring cells too, and a dense cell such as a stadium exit becomes a {{hot-key|hot spot}} that needs splitting. An interviewer will ask how fresh the results must be, how often drivers report, and how you {{sharding|shard}} by geography. The failure pattern is a skewed city centre: sharding by region balances the map but not the demand.",
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
			"Which persistence setting on the in-memory store bounds the loss to a short window? Compare what each option costs.",
		},
		Answer: "The answer: switch Redis persistence to the append-only file: a crash then loses at most the last second of writes.",
		Predictions: []Prediction{{ID: "p1", Question: "With an append-only file, what is the most you can lose in a crash?", Options: []string{"Nothing at all, as every write is logged first", "About the last second of writes", "Everything since the last backup was taken"}, Answer: 1,
			Explain: "The file is flushed about once a second, so writes acknowledged since the last flush can still be lost."}},
		Debrief: "Memory is fast and volatile. Persistence trades a little speed for a bounded loss window: choose how many seconds of data you can afford to lose, then pick the setting that guarantees it.",
		Deeper:  "Persistence is a choice of how much you are willing to lose: an append-only file flushed about once a second bounds the loss to roughly that window, flushing on every write shrinks it at a latency cost, and periodic snapshots lose everything since the last one. A replica adds availability, not durability, because asynchronous replication can lose the same last moments. The first question is whether Redis is the source of truth at all; if the scores can be rebuilt from the database, a restart that comes back empty plus a rebuild may be the cheaper answer. An interviewer will ask for your recovery point and recovery time and how you would test them. A real failure pattern is the empty restart: the cache returns cold, every request falls through to the database, and {{metastable-failure|the recovery causes a second outage}}.",
	})
}
