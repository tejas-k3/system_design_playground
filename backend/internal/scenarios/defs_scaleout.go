package scenarios

import (
	"fmt"

	"sysdesign/internal/engine"
)

// registerScaleOut registers regions, shards, tail latency, tenants, fan-out, global reach and multi-step transactions.
func registerScaleOut() {
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
		Story:   "Last quarter the app servers moved close to the users, and every user is sent to the nearest one. The database is still a single primary in the US, so a read from Europe or Asia crosses an ocean twice to be answered, and the app server spends most of its time waiting for the answer. Reads are 98% of the traffic, and {{consistency-levels|a read may be at most a second out of date}}.",
		Diagram: multiRegion(false), Solution: multiRegion(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(200), MaxStaleReadPct: f(0.02), MaxCostPerHour: f(2.6), StarCostPerHour: f(2.4)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Compare the latency of the three groups of users. Where does the extra time in Europe and Asia go?",
			"Every read from Europe crosses to the US and back, about 90 ms of distance before the database does anything; Asia pays 150 ms. The app servers are already close, so the data has to come closer too.",
			"Where must the data live so that a read from Europe or Asia does not cross the ocean, and what must each regional server be wired to?",
		},
		Answer: "The answer: put a read replica in Europe and one in Asia, in their regions, and wire each region's app server to its replica. Reads are answered locally, writes still go to the US primary, and {{replication-lag|a replica runs a little behind}}, which a read that may be a second old can live with.",
		Predictions: []Prediction{{ID: "p1", Question: "A read replica in Asia will make reads there faster, and writes from Asia…", Options: []string{"Faster as well, since the replica is closer", "No faster: they still go to the primary", "Slower, as the replica must also copy them"}, Answer: 1,
			Explain: "A replica serves reads only. Every write still has to reach the one primary, so a write from Asia keeps paying the distance."}},
		Debrief: "Replicas near the readers fix read latency; they do not move the writes, and they add {{replication-lag|replication lag}} that grows with the distance to the primary (here about 190 ms to Europe and 250 ms to Asia). Moving writes close too needs several leaders, which brings conflicts, or {{sharding|sharding}} users by home region. The extra round trips show why {{distributed-system-pitfalls|latency is not zero}}.",
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
		Story:   "The {{sharding|four-shard}} database is getting full, so a fifth shard is added at 20 seconds while traffic keeps flowing. The router places each key on a shard by hashing it and taking the remainder after dividing by the number of shards, so adding one changes where almost every key lives. For ten seconds the data of every moved key is still travelling, and each request for one of them takes 400 ms longer.",
		Diagram: addShard(false), Solution: addShard(true),
		Chaos:       []engine.ChaosEvent{{Type: "addNode", NodeID: "shards", AtSec: 20}},
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(300), MaxCostPerHour: f(4.2), StarCostPerHour: f(4.0), StarP95Ms: f(200)},
		DurationSec: 60, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Watch the router when the new shard arrives: how many keys moved, and how many requests were slowed by the move?",
			"Placing a key by hash mod n changes the home of about four keys in five when n goes from 4 to 5. About 80% of requests wait for data that is still moving.",
			"Which key placement scheme moves only the keys that belong to the new shard, instead of nearly all of them?",
		},
		Answer: "The answer: place keys on a {{consistent-hashing|consistent-hashing ring}} instead (key placement on the shard router): a new shard takes over only about a fifth of the keys, so only about a fifth of the requests are slowed, and for a shorter time than the {{p95}} notices.",
		Predictions: []Prediction{{ID: "p1", Question: "Adding a fifth shard to a router that places keys by hash mod 4 moves about…", Options: []string{"About a fifth of the keys", "About four fifths of the keys", "None: the new shard starts empty"}, Answer: 1,
			Explain: "Changing the divisor changes the remainder for almost every key. A ring only reassigns the arc the new shard takes over, about one key in five."}},
		Debrief: "Hash mod n makes resizing a cluster as expensive as rebuilding it. {{consistent-hashing|Consistent hashing}} moves only about 1/n of the keys when a node joins or leaves, which is why caches and key-value stores use it.",
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
		Story:   "A product page reads from three identical read replicas, taking turns. Most reads take a few milliseconds, but the replicas have a long tail: now and then one read takes ten times as long, whichever replica serves it. The {{p50|median}} is great. {{p95|The slowest one in twenty}} is what customers feel, and adding threads or servers has not moved it.",
		Diagram: heavyTail(false), Solution: heavyTail(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(230), MaxCostPerHour: f(3.0), StarCostPerHour: f(2.8), StarP95Ms: f(190)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the latency distribution: the slow reads are the replicas' own long tail, not a queue. Are the threads or the replicas busy?",
			"Adding capacity does not shorten a slow read, because the slow read is slow by itself. But the other two replicas are fine: the same read would almost certainly be quick if asked of another.",
			"The slow read is slow by itself, but the other replicas are fine. What if a second replica were asked after a short wait, and how long should that wait be?",
		},
		Answer: "The answer: hedge the reads (advanced setting on the app server): if a read has not been answered after a short wait, ask another replica too and take the first answer. Wait about the time a normal read takes; hedging too early doubles the load for nothing.",
		Predictions: []Prediction{{ID: "p1", Question: "Hedging reads after 80 ms sends a second request for…", Options: []string{"Every read, so the load roughly doubles", "Only the slow ones, a small share of reads", "None, as it only works when a replica is down"}, Answer: 1,
			Explain: "Most reads finish before the wait is over, so only the few that are still waiting get a second request, a modest increase in load for a large cut in the tail."}},
		Debrief: "Hedged requests trade a little extra load for a much shorter tail: the chance that two independent replicas are both slow is the square of the chance that one is. Wait about the {{p95}} of a normal request before hedging, or you simply double your traffic.",
		Deeper:  "A hedged request sends a second copy to another replica after a short wait and takes whichever answers first; if slowness is independent between replicas, the chance both are slow is the square of the chance one is. Waiting about the {{p95}} of a normal read means only the slowest few percent of reads are duplicated, so the extra load is small, while hedging immediately simply doubles your traffic. It is only safe for requests that can run twice, so reads and {{idempotency|idempotent}} calls, and you cancel the loser when you can. The assumption breaks when slowness is shared, for example a failing network path or a server that is overloaded, and then {{retry-storm|hedging adds load to a system that is already struggling}}. An interviewer will ask why not retry after a timeout, how to pick the delay adaptively, and how hedging combines with ejecting a consistently slow replica.",
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
			"What decides whose requests the limiter counts together? Could each tenant be counted on its own?",
		},
		Answer: "The answer: make the rate limiter count per tenant (Limit per: each tenant): every tenant gets its own allowance, so the noisy one runs out without turning anyone else away.",
		Predictions: []Prediction{{ID: "p1", Question: "Doubling the shared limit to 200 requests a second would…", Options: []string{"Fix it for the quiet tenants, who get twice the room", "Help a little, but the noisy tenant still takes more", "Make no difference, since the noisy tenant is capped"}, Answer: 1,
			Explain: "A shared limit divides capacity in proportion to demand, so the loudest tenant keeps getting the most. A limit per tenant divides it fairly."}},
		Debrief: "A global rate limit protects the servers but not the tenants from each other. {{bulkhead|Limit per customer so one customer's burst runs into its own allowance}}, and add a shared limit on top if the total must be bounded too.",
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
			"A queue gives each message to one reader. Which messaging component gives every connected consumer its own copy?",
		},
		Answer: "The answer: replace the queue with a topic (publish/subscribe): every worker connected to it is a subscriber and gets its own copy of every message.",
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
			"If distance is the cost, where must the whole stack live? Which setting routes each group of users to the nearest copy, and what will that cost?",
		},
		Answer: "The answer: run a stack (balancer, app, database) in each region, connect every user group to all of them, and turn on 'Send users to the nearest region' for the users. Remember the stacks cost money too.",
		Predictions: []Prediction{{ID: "p1", Question: "Adding more servers in the US would make users in Asia…", Options: []string{"Much faster, as the queues in the US shrink", "No faster: the delay is the distance, not the load", "Slower, since the US servers then take more of the traffic"}, Answer: 1,
			Explain: "Latency to a far region is dominated by the distance each way. Capacity helps when servers are busy, not when the wait is the speed of light."}},
		Debrief: "To serve a global audience, put the stack near the users and route them to it. The cost is running several copies, and the hard part moves to {{consistency-levels|keeping their data consistent}} (which the multi-region database challenges are about). Do not assume {{distributed-system-pitfalls|latency is zero}}: users far from the stack pay for the distance.",
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
			"When a later step fails, what should happen to the earlier steps? Which server setting runs the opposite of each, newest first?",
		},
		Answer: "The answer: run the checkout as a saga (the app server's setting for writes that span services): when a later step fails, compensating calls undo the earlier steps, newest first. A few percent of orders still fail (declines are real), but none are left half done.",
		Predictions: []Prediction{{ID: "p1", Question: "With a saga, will fewer orders fail?", Options: []string{"Yes: the failed steps are retried until they pass", "No: the same orders fail, but each is cleanly rolled back", "Yes: the card processor declines fewer orders"}, Answer: 1,
			Explain: "A saga does not make steps succeed. It makes a failure safe by running the opposite of every earlier step, so the system ends up consistent."}},
		Debrief: "Without a distributed transaction, consistency across services comes from compensation: every step has an undo, and a failure runs the undos backwards. Compensations must be possible, repeatable ({{idempotency|idempotent}}), and watched: one that fails leaves a stuck order for a person.",
		Deeper:  "A saga replaces one distributed transaction with a chain of local ones, each with a compensating action, and it is {{consistency-levels|eventually consistent}}: other users can see the item reserved before the order completes, so you model a pending state and do not pretend there is isolation. Order the steps so the hardest to undo comes late, because a refund may itself fail or take time, and make every compensation {{idempotency|idempotent}} so that retrying it is safe. The saga's own progress has to be stored somewhere durable, otherwise a crash between charging and recording leaves exactly the half-done order you were avoiding; with an idempotency key the retry cannot charge twice. An interviewer will ask what happens when a compensation keeps failing, and the honest answer is an alert and a person, as the debrief says. The failure pattern is the orchestrator that dies mid-saga with no record of how far it got.",
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
			"Which approach holds nothing across steps, so that a dead coordinator cannot leave anything locked?",
		},
		Answer: "The answer: run the checkout as a saga instead (the app server's setting for writes that span services): no service holds anything across steps, and a failure is undone by compensating calls.",
		Predictions: []Prediction{{ID: "p1", Question: "Two-phase commit guarantees nothing is ever half done. What does it cost?", Options: []string{"Nothing: it is strictly better than a saga", "A second round of calls, and a stall if the coordinator dies", "Lost data, since a crash discards the votes"}, Answer: 1,
			Explain: "It buys atomicity with latency and blocking: every service holds its part until the decision arrives, and a coordinator that dies between the votes and the decision leaves them all stuck."}},
		Debrief: "Two-phase commit is atomic but blocking: its weak point is the coordinator. Production systems replicate the coordinator, or avoid distributed transactions and accept {{consistency-levels|eventual consistency}} with sagas, where each step is a small local transaction and failures are compensated.",
		Deeper:  "Two-phase commit is atomic but blocking: after a participant votes yes it must hold its locks until the coordinator's decision arrives, and a coordinator that dies in that gap leaves them in doubt and everything touching those rows waits. The fixes are to replicate the coordinator with a {{quorum|consensus protocol}}, as some distributed databases do, or to avoid the distributed transaction and use a saga with its own trade-offs. Two-phase commit also assumes every participant supports prepare, which a card processor or shipping service almost never does, so across services you do not control it is often not an option at all. An interviewer will ask when you would still choose it, and the answer is inside one system where the coordinator is replicated and the locks are short. The failure pattern is in-doubt transactions holding locks that block unrelated traffic until someone resolves them by hand.",
	})
}
