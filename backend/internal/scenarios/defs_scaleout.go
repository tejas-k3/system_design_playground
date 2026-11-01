package scenarios

import (
	"fmt"

	"sysdesign/internal/engine"
)

// registerScaleOut registers regions, shards, tail latency, tenants, fan-out, global reach and multi-step transactions.
func (c *catalog) registerScaleOut() {
	c.addMultiRegionReads()
	c.addAddAShard()
	c.addHeavyTail()
	c.addNoisyTenant()
	c.addNotificationFanOut()
	c.addGoGlobal()
	c.addPaymentSaga()
	c.addFrozenCheckout()
	c.addRestartTheCounter()
	c.addEveryoneAsksAuth()
	c.addWhereDoSessionsLive()
	c.addFrontDoor()
	c.addStaleRegistry()
	c.addColdStart()
	c.addRankedFeed()
	c.addChatOrder()
	c.addFailingRefund()
	c.addBooksBalance()
	c.addWhoIsOnline()
	c.addFriendsOfFriends()
}

func (c *catalog) addMultiRegionReads() {
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
	c.register(&Scenario{
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
}

func (c *catalog) addAddAShard() {
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
		for k := range 4 {
			id := fmt.Sprintf("db%d", k+1)
			b.n(id, engine.KDatabase, cols[3], float64(k)*100+30, P{"connections": 32.0, "readMs": 5.0}).e("shards", id)
		}
		return b.done()
	}
	c.register(&Scenario{
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
}

func (c *catalog) addHeavyTail() {
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
		for k := range 3 {
			id := fmt.Sprintf("replica%d", k+1)
			b.n(id, engine.KReplica, cols[2], float64(k)*100+60, P{"connections": 16.0, "readMs": 40.0, "variability": "heavy", "replicationLagMs": 50.0}).e("app", id)
		}
		return b.done()
	}
	c.register(&Scenario{
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
}

func (c *catalog) addNoisyTenant() {
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
	c.register(&Scenario{
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
}

func (c *catalog) addNotificationFanOut() {
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
	c.register(&Scenario{
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
}

func (c *catalog) addGoGlobal() {
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
	c.register(&Scenario{
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
}

func (c *catalog) addPaymentSaga() {
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
	c.register(&Scenario{
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
}

func (c *catalog) addFrozenCheckout() {
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
	c.register(&Scenario{
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

	// Nobody to wait for: the same checkout, but the stock must stay all-or-nothing and the services must not wait for a dead server
	nobodyToWait := func(fixed bool) engine.Diagram {
		d := frozenCheckout(false)
		for k := range d.Nodes {
			if d.Nodes[k].ID == "card" {
				d.Nodes[k].Params["errorPct"] = 0.0 // an undo that fails would leave an order stuck whichever protocol runs
			}
		}
		if fixed {
			for k := range d.Nodes {
				if d.Nodes[k].ID == "app" {
					d.Nodes[k].Params["workflow"] = "threePhase"
					d.Nodes[k].Params["termTimeoutMs"] = 100.0
				}
			}
		}
		return d
	}
	c.register(&Scenario{
		ID: "nobody-to-wait-for", Title: "Nobody to wait for", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Keep checkout all-or-nothing, but let the services decide alone when the server dies.",
		Story:   "Checkout is a two-phase commit across the stock database, the card processor and shipping, and when the server dies between the votes and the decision the services sit locked until it returns. Operations prefers not to use a saga, because other customers can then see stock reserved for an order that later vanishes. The services should instead settle on their own when the server stays silent, and the total time rows stay locked must stay small. Operations still wants every order to stay {{consistency-levels|consistent}}.",
		Diagram: nobodyToWait(false), Solution: nobodyToWait(true),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "app", AtSec: 15, DownSec: 8}},
		Goals:       engine.Goals{MinSuccessRate: 0.6, MaxInDoubt: i(0), MaxInconsistent: i(0), MaxBlockedSec: f(15), MaxP95Ms: f(900), MaxCostPerHour: f(1.6), StarP95Ms: f(860), StarCostPerHour: f(1.4)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the goals panel after the crash: how many transactions wait, and for how many lock-seconds?",
			"The services are stuck because after voting yes they cannot tell whether the others voted yes too. What if there were a step between the votes and the commit that removes that doubt?",
			"The app server's setting for writes that span services has a third kind of commit. It adds a round, and it has a timer: how long should the services wait before they decide alone?",
		},
		Answer: "The answer: use three-phase commit and shorten the time the services wait before deciding alone. A pre-commit round means no service is ever unsure while it holds a lock, so when the server dies they finish or abort by themselves after the timeout.",
		Predictions: []Prediction{{ID: "p1", Question: "What does three-phase commit cost compared with two-phase?", Options: []string{"Nothing: it is strictly better than two-phase commit", "A third round of calls, and a wrong decision if a slow server looks dead", "Lost data whenever the server restarts after a crash"}, Answer: 1,
			Explain: "The extra round makes the wait unnecessary, but the services must guess that a silent server is dead. In a partition the guess can be wrong and two sides can decide differently."}},
		Debrief: "Three-phase commit removes the uncertain state: after the pre-commit round everyone knows the outcome, so the services can finish alone when the coordinator dies. It pays a third round on every transaction, and it assumes bounded delays and crashes without partitions, which real networks do not guarantee. That is why practice replicates the coordinator with consensus, or avoids the distributed transaction with sagas.",
		Deeper:  "Two-phase commit blocks because a participant that voted yes cannot tell whether the others voted yes too, so it cannot decide alone. Three-phase commit adds a pre-commit round: once a participant has seen it, everybody has voted yes, so on silence it commits; if it has not seen it, nobody has committed, so it aborts. That removes the blocking on a coordinator crash, at the cost of a round on every transaction, but it works only if the network delivers within known bounds and a silent coordinator really is dead. With a partition or a long pause, one side can commit while the other times out and aborts, and the atomicity is gone, which is why it is rarely used. The practical routes are a replicated coordinator that agrees on its decision with a consensus protocol, so there is no single machine to wait for, or a saga that accepts that other readers see the intermediate states. An interviewer will ask what a participant does after it has voted yes and then lost the coordinator, and why timeouts alone cannot settle it.",
	})
}

func (c *catalog) addRestartTheCounter() {
	// Restart the counter: a stream job crashes and must catch up fast without sending outputs twice
	restartCounter := func(fixed bool) engine.Diagram {
		st := P{"windowSec": 2.0, "watermarkDelaySec": 1.0, "checkpointSec": 20.0}
		if fixed {
			st["recovery"] = "messageLog"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 0.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "queueCap": 500.0}).
			n("counter", engine.KStream, cols[2], rowMid, st).label("counter", "Orders per 2 seconds").
			e("users", "app").e("app", "counter").done()
	}
	c.register(&Scenario{
		ID: "restart-the-counter", Title: "Restart the counter", Difficulty: "Medium", ConfigOnly: true,
		Summary: "After a crash the stream job repeats outputs and takes too long to catch up.",
		Story:   "A stream job counts orders in two-second windows and saves its place every twenty seconds. When it crashes, it reloads the last save and handles everything since again: the billing system receives some totals twice, and the dashboard is stale for over two seconds. Billing wants no repeated totals and the dashboard wants the job caught up within one and a half seconds, without saving more often.",
		Diagram: restartCounter(false), Solution: restartCounter(true),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "counter", AtSec: 33, DownSec: 2}},
		Goals:       engine.Goals{MinSuccessRate: 0.95, MaxRepeatedOutputs: i(0), MaxRecoverySec: f(1.5), MaxP95Ms: f(200), MaxCostPerHour: f(1.0), StarP95Ms: f(150), StarCostPerHour: f(0.95)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the stream job's stats after the crash: how many outputs repeat and how long did the catch-up take?",
			"After a restart the job has the state of its last save. Everything it handled since then has to be handled again, at full cost, and its outputs go out a second time.",
			"If every event were written to a log before it was handled, the job could replay the log. The stream job's recovery setting has that option: what does it cost per event?",
		},
		Answer: "The answer: switch the job's recovery to a message log. Every event is written to the log first, so after a crash the job reloads its save and replays the log quickly and without sending outputs again, for a small write on every event.",
		Predictions: []Prediction{{ID: "p1", Question: "What does logging every event buy over saving more often?", Options: []string{"A replay of what happened since the last save, with no repeated outputs", "Nothing: a log is the same as a save, only slower", "A free recovery, since the log is written later"}, Answer: 0,
			Explain: "A save captures state at one moment; a log records everything after it, so the job can be brought back to just before the crash. The price is a log write on every event."}},
		Debrief: "Checkpointing alone restarts from the last saved state and repeats the work since, so a long gap between saves means a long recovery and repeated outputs. Message logging (a {{write-ahead-log|log}} of the input) records each event before it is handled, so recovery replays deterministically from the last save and receivers can discard what they already saw. You pay one log write per event; the alternative is saving more often, which also costs.",
	})
}

func (c *catalog) addEveryoneAsksAuth() {
	// Everyone asks auth: every request waits for the auth service, which is slow under load and then goes down
	everyoneAsksAuth := func(fixed bool) engine.Diagram {
		app := P{"threads": 32.0, "queueCap": 400.0, "authMode": "service", "tokenUsers": 500.0}
		if fixed {
			app["tokenCacheSec"] = 60.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 300.0, "readPct": 100.0, "keySpace": 5000.0, "keyDist": "uniform", "timeoutMs": 1500.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, app).
			n("auth", engine.KAuth, cols[2], 60, P{"checkMs": 20.0, "connections": 4.0, "queueCap": 50.0}).label("auth", "Auth service").
			n("db", engine.KDatabase, cols[2], 200, P{"connections": 16.0, "readMs": 3.0}).
			e("users", "app").e("app", "auth").e("app", "db").done()
	}
	c.register(&Scenario{
		ID: "everyone-asks-auth", Title: "Everyone asks auth", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Every request waits for the auth service, and it cannot keep up.",
		Story:   "Each request carries a token, and the app server asks the auth service whether it is valid before it does anything else. The auth service handles about 200 checks a second and 300 requests arrive, so it is the bottleneck. When it restarts for eight seconds the whole site is down with it. You may not let requests through unchecked, but 500 signed-in users make most of those checks repeats.",
		Diagram: everyoneAsksAuth(false), Solution: everyoneAsksAuth(true),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "auth", AtSec: 20, DownSec: 8}},
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxUnchecked: i(0), MaxP95Ms: f(300), MaxCostPerHour: f(1.5), StarP95Ms: f(200), StarCostPerHour: f(1.2)},
		DurationSec: 50, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the auth service in the diagram: how busy is it, and what happens to requests while it is down?",
			"There are only 500 different users. How many of the checks are about a user who was already checked a moment ago?",
			"The app server can remember a valid token for a while. How long can it be remembered without being unsafe, given that a revoked token is accepted until then?",
		},
		Answer: "The answer: have the app server remember a valid token for a minute. Each of the 500 users is checked once a minute instead of on every request, so the auth service is no longer the bottleneck and its outage only affects users whose token has expired.",
		Predictions: []Prediction{{ID: "p1", Question: "What is the risk of remembering a valid token for a minute?", Options: []string{"A token revoked in that minute is still accepted", "The auth service gets even more load", "Users are logged out more often"}, Answer: 0,
			Explain: "A remembered yes is not checked again until it expires, so a revocation takes up to a minute to take effect. A shorter time lowers the risk and raises the load."}},
		Debrief: "A service every request depends on is both a bottleneck and a single point of failure. Remembering its answers for a short time removes most of the load and rides out a short outage; verifying a signed token on each server removes the call altogether, at the price of noticing a revocation only when the token expires. Failing open keeps the site up by letting everyone in, which is rarely acceptable.",
	})
}

func (c *catalog) addWhereDoSessionsLive() {
	// Where login sessions live: two app servers keep sessions in memory; one restarts
	loginSessions := func(fixed bool) engine.Diagram {
		state := "local"
		if fixed {
			state = "shared"
		}
		app := P{"threads": 16.0, "queueCap": 200.0, "sessionState": state}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 100.0, "keySpace": 5000.0, "keyDist": "uniform", "timeoutMs": 1500.0, "retries": 0.0, "identities": 300.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, P{"healthCheckSec": 1.0}).
			n("app1", engine.KAppServer, cols[2], 40, app).
			n("app2", engine.KAppServer, cols[2], 200, app).
			n("db", engine.KDatabase, cols[3], 40, P{"connections": 16.0, "readMs": 3.0}).
			n("sessions", engine.KRedis, cols[3], 200, P{}).label("sessions", "Session store").
			e("users", "lb").e("lb", "app1").e("lb", "app2").e("app1", "db").e("app2", "db").e("app1", "sessions").e("app2", "sessions").done()
	}
	c.register(&Scenario{
		ID: "where-do-sessions-live", Title: "Where do sessions live?", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Users are signed out whenever their requests land on another server, or the server restarts.",
		Story:   "Each logged-in user's session is kept in the memory of the app server that signed them in. The load balancer sends the next request to either server, so a user keeps finding themselves signed out, and when one server restarts, everyone it held is signed out again. A Redis instance is already in the diagram. Keep the sign-outs at zero and the p95 under 250 ms, without adding servers.",
		Diagram: loginSessions(false), Solution: loginSessions(true),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "app1", AtSec: 20, DownSec: 4}},
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxReauth: i(0), MaxP95Ms: f(250), MaxCostPerHour: f(1.2), StarP95Ms: f(200), StarCostPerHour: f(1.0)},
		DurationSec: 45, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at which requests fail: are they the ones after the restart, or also before it?",
			"Making the balancer keep a user on one server fixes the cross-server misses. Where does that user's session go when the server restarts?",
			"Which component could every app server consult, so that a user's session does not depend on the server that signed them in?",
		},
		Answer: "The answer: keep the sessions in the shared store (the app servers' session setting). Either server finds the session, and a restarted server loses nothing, at the cost of one call to the store per request.",
		Predictions: []Prediction{{ID: "p1", Question: "The balancer is made sticky, then the server holding a user's session restarts. What happens to that session?", Options: []string{"Another server recovers it automatically", "It is lost unless another component kept a copy", "Stickiness copies it, so it survives"}, Answer: 1,
			Explain: "Stickiness only keeps a user's requests on the same server. It does not copy anything, so the memory of a restarted server is gone."}},
		Debrief: "A {{stateless-service|stateless}} server keeps nothing between requests, so any healthy server can take the next one. Keeping a session in one server's memory makes the user depend on that server. Sticky routing keeps the requests together but not the state: it loads servers unevenly and does not survive a restart. A shared store lets any server find the session, adding a call to every request and a dependency that must itself stay up; a session that expires when idle is soft state, which the user notices only when it is gone.",
	})
}

func (c *catalog) addFrontDoor() {
	// The front door: the balancer in front of plenty of servers is the first thing to fill up
	frontDoorFull := func(fixed bool) engine.Diagram {
		lb := P{"maxRps": 1200.0, "layer": "layer7"}
		if fixed {
			lb["layer"] = "layer4"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 2500.0, "readPct": 100.0, "keySpace": 5000.0, "keyDist": "uniform", "timeoutMs": 1500.0, "retries": 0.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, lb).label("lb", "Entry balancer").
			n("app1", engine.KAppServer, cols[2], 60, P{"threads": 32.0, "cpuMs": 5.0, "queueCap": 300.0}).
			n("app2", engine.KAppServer, cols[2], 200, P{"threads": 32.0, "cpuMs": 5.0, "queueCap": 300.0}).
			n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 64.0, "readMs": 1.0}).
			e("users", "lb").e("lb", "app1").e("lb", "app2").e("app1", "db").e("app2", "db").done()
	}
	c.register(&Scenario{
		ID: "front-door", Title: "The front door", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Half the traffic is turned away at the balancer while the servers behind it sit idle.",
		Story:   "All traffic enters through one load balancer that reads every request, which is what lets it route on content. Its {{throughput|throughput}} is about 1,200 requests a second. 2,500 arrive. Behind it two app servers could take 6,000 between them and are nearly idle, and the database is bored. Nothing here needs content-aware routing: every request goes to either server. Serve at least 97% of requests within the budget.",
		Diagram: frontDoorFull(false), Solution: frontDoorFull(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(300), MaxCostPerHour: f(3.0), StarCostPerHour: f(2.75)},
		DurationSec: 30, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Open the bottleneck check or look at each component's busy level: what is full, and what is not?",
			"More app servers will not help if the requests never reach them. What limits how many requests the balancer passes?",
			"A balancer that reads every request does more work per request than one that only forwards connections. Does anything in this design need the balancer to read requests?",
		},
		Answer: "The answer: switch the balancer to layer 4. It forwards connections without reading the request, so the same machine passes about four times as many and costs less. It can no longer route by content, which nothing here needs.",
		Predictions: []Prediction{{ID: "p1", Question: "Two more app servers are added behind the full balancer. What happens to the success rate?", Options: []string{"It stays about the same, since the balancer is the limit", "It doubles, since there is twice the capacity", "It falls, since traffic is split thinner"}, Answer: 0,
			Explain: "A request must pass the balancer before any server sees it, so capacity behind it cannot raise what gets through."}},
		Debrief: "Every request passes the entry point, so it is the first place a design can saturate, and adding servers behind it changes nothing. A layer-7 balancer reads the request, which allows routing by path, header or user, at the price of more work per request; a layer-4 balancer forwards connections and passes far more. Choose layer 7 only when you route on content, or run several balancers behind DNS or anycast when one is not enough.",
	})
}

func (c *catalog) addStaleRegistry() {
	// A service registry: a dead server stays listed until its entry expires and the balancer reads the registry again
	registryPool := func(fixed bool) engine.Diagram {
		lb := P{"membership": "registry", "registryTtlSec": 15.0, "registryRefreshSec": 10.0}
		if fixed {
			lb["registryTtlSec"], lb["registryRefreshSec"] = 2.0, 1.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 120.0, "readPct": 100.0, "keySpace": 2000.0, "keyDist": "uniform", "timeoutMs": 1000.0, "retries": 0.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, lb).label("lb", "Balancer").
			n("app1", engine.KAppServer, cols[2], 60, P{"threads": 32.0, "cpuMs": 5.0}).
			n("app2", engine.KAppServer, cols[2], 200, P{"threads": 32.0, "cpuMs": 5.0}).
			n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 64.0, "readMs": 1.0}).
			e("users", "lb").e("lb", "app1").e("lb", "app2").e("app1", "db").e("app2", "db").done()
	}
	c.register(&Scenario{
		ID: "stale-registry", Title: "Still on the list", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The balancer keeps sending requests to a server the registry still lists.",
		Story:   "The balancer does not probe its servers. It reads a {{service-registry|service registry}} every 10 seconds, and a server's entry lives 15 seconds after its last heartbeat. At ten seconds one of the two app servers crashes. The other could carry all the traffic, but until the entry expires and the balancer reads the registry again, about half the requests still go to the dead one. Serve at least 95% of requests.",
		Diagram: registryPool(false), Solution: registryPool(true),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "app1", AtSec: 10, DownSec: 40}},
		Goals:       engine.Goals{MinSuccessRate: 0.95, MaxP95Ms: f(300), MaxCostPerHour: f(3.0), StarCostPerHour: f(2.9)},
		DurationSec: 50, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The second server is healthy. Where do the failures start, and for how long do they last?",
			"Two delays add up between a crash and the balancer's last request to the dead server. Find both on the balancer.",
			"Shorter times find a dead server sooner, but cost more registry traffic. How short can they be and still stay inside the budget?",
		},
		Answer: "The answer: shorten the registry entry lifetime and the balancer's read interval. A dead server stays in the pool for both times added together, so a few seconds each leaves only a short burst of failures.",
		Predictions: []Prediction{{ID: "p1", Question: "The entry lifetime is 15 s and the read interval is 10 s. At worst, how long after a crash does the balancer still send to the dead server?", Options: []string{"About 15 seconds", "About 25 seconds", "About 10 seconds"}, Answer: 1,
			Explain: "The entry must expire first, and the balancer only notices at its next read, so the two waits add."}},
		Debrief: "A {{service-registry|service registry}} lets servers join and leave a pool without anyone editing the balancer: each one announces itself and keeps its entry alive with a heartbeat. The price is delay. A crashed server stays listed until its entry expires, and the balancer sees that only at its next read; a restarted server is used from the next read as well. Short times shrink the window but add heartbeats and reads. A health check on the balancer reacts to a failed probe instead, which is quicker, but every balancer must be told about each server.",
	})
}

func (c *catalog) addColdStart() {
	// The first request after a pause: a function that scales to zero starts cold
	coldFunction := func(fixed bool) engine.Diagram {
		app := P{"threads": 8.0, "cpuMs": 20.0, "coldStartMs": 1200.0, "idleSec": 0.3}
		if fixed {
			app["idleSec"] = 30.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 2.0, "readPct": 100.0, "keySpace": 100.0, "keyDist": "uniform", "timeoutMs": 3000.0, "retries": 0.0}).
			n("fn", engine.KAppServer, cols[1], rowMid, app).label("fn", "Function").
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 8.0, "readMs": 5.0}).
			e("users", "fn").e("fn", "db").done()
	}
	c.register(&Scenario{
		ID: "cold-start", Title: "The first request after a pause", Difficulty: "Medium", ConfigOnly: true,
		Summary: "A function that scales to zero makes nearly every user wait for it to start.",
		Story:   "An endpoint runs as a function that stops after 0.3 seconds without a request, so that nothing is paid for while it is idle. Starting it takes 1.2 seconds. Two requests a second arrive, with gaps that are often longer than 0.3 seconds, so many requests find it stopped. Users should see an answer within 400 ms at the 95th percentile, and the bill should stay under $0.20 an hour.",
		Diagram: coldFunction(false), Solution: coldFunction(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(0.2), StarP95Ms: f(300), StarCostPerHour: f(0.19)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the latency of a single request: where do the extra 1,200 ms come from?",
			"The function stops when it has been idle for the idle time. How often does a request arrive compared with that time?",
			"Two things keep it warm: not letting it stop so quickly, or paying for one instance that never stops. Which costs less here?",
		},
		Answer: "The answer: lengthen the idle time to a few seconds. Requests arrive twice a second, so the function never stops, and you still pay nothing while it is idle for longer. Keeping one instance warm also works, at a fixed cost.",
		Predictions: []Prediction{{ID: "p1", Question: "What decides whether a request finds a function cold?", Options: []string{"Whether it was idle longer than the idle time", "Whether it is the first request of the whole day", "Whether the database is slow just then"}, Answer: 0,
			Explain: "A function stops after its idle time, so a request after a longer gap pays the start-up; steady traffic keeps it warm."}},
		Debrief: "A function that {{autoscaling|scales to zero}} trades cost for latency: it costs nothing while idle, and the first request after an idle spell waits for it to start. Steady traffic keeps it warm; sparse traffic meets a cold start almost every time. The levers are the idle time, a smaller and quicker image, and paying for an instance that stays warm. A container starts in seconds, a virtual machine in tens of seconds, which is why functions run in containers.",
	})
}

func (c *catalog) addRankedFeed() {
	// The ranked feed: a model scores thousands of candidate posts for every read
	rankedFeed := func(fixed bool) engine.Diagram {
		cand := 5000.0
		if fixed {
			cand = 300
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 40.0, "readPct": 95.0, "keySpace": 5000.0, "keyDist": "uniform", "timeoutMs": 1500.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 8.0, "cpuMs": 10.0, "queueCap": 100.0, "ranking": true, "rankCandidates": cand, "rankUsPerCandidate": 100.0}).label("app", "Feed service").
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 32.0, "readMs": 3.0}).
			e("users", "app").e("app", "db").done()
	}
	c.register(&Scenario{
		ID: "ranked-feed", Title: "The feed that thinks too long", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Ranking 5,000 posts for every read leaves the feed service far behind.",
		Story:   "A feed is ranked before it is shown: for every read the service scores 5,000 candidate posts with a model, 0.1 ms each, so a read costs half a second of work and the service has eight {{thread-pool|threads}}. Forty reads a second arrive and most time out. Product wants the feed ranked, so the model stays. Keep at least 97% of reads answered, within 250 ms at the 95th percentile, and the bill under $0.70 an hour.",
		Diagram: rankedFeed(false), Solution: rankedFeed(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(250), MaxCostPerHour: f(0.7), StarP95Ms: f(200), StarCostPerHour: f(0.6)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the feed service's busy level and a single request's time: where does a read spend it?",
			"The model's cost grows with the number of candidates it scores, and a hundred times as many candidates is a hundred times the work. Does it need to score 5,000 posts to find the best few?",
			"A cheap first step can narrow thousands of posts to a few hundred before the model scores them. Which setting says how many candidates the model sees?",
		},
		Answer: "The answer: let a cheap retrieval step cut the candidates to a few hundred (the candidates scored per read) and rank only those. Adding threads buys capacity at a higher bill and leaves every read slow.",
		Predictions: []Prediction{{ID: "p1", Question: "Scoring ten times as many candidates costs…", Options: []string{"About ten times the model time per read", "About the same, since the model is fast", "About the square root of ten times as much"}, Answer: 0,
			Explain: "Each candidate is scored once, so the work grows in step with the number of candidates."}},
		Debrief: "A ranking stage multiplies the cost of a read by the number of candidates it scores. Production feeds work in stages: a cheap candidate generator narrows millions of posts to hundreds, and the expensive model ranks only those. The number of candidates trades quality of choice against latency and cost, so choose it from the latency budget.",
	})
}

func (c *catalog) addChatOrder() {
	// The chat that arrives out of order: messages of one conversation spread over several consumers
	chatOrder := func(fixed bool) engine.Diagram {
		by := "roundRobin"
		if fixed {
			by = "key"
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 60.0, "readPct": 0.0, "keySpace": 50.0, "keyDist": "uniform", "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0}).
			n("log", engine.KLog, cols[2], rowMid, P{"partitions": 4.0, "partitionBy": by, "capacity": 50000.0}).label("log", "Message log").
			n("db", engine.KDatabase, cols[4], rowMid, nil).label("db", "Message store").
			e("users", "app").e("app", "log")
		for k := range 4 {
			id := fmt.Sprintf("c%d", k+1)
			b.n(id, engine.KWorker, cols[3], float64(k)*70, P{"concurrency": 1.0, "processMs": 40.0}).label(id, fmt.Sprintf("Delivery %d", k+1)).e("log", id).e(id, "db")
		}
		return b.done()
	}
	c.register(&Scenario{
		ID: "chat-order", Title: "Messages out of order", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Replies show up before the question they answer.",
		Story:   "A chat service puts every message on a {{sharding|partitioned}} log read by four delivery workers. Each conversation is a key. Messages are spread evenly over the partitions, so two messages of one conversation are often delivered by different workers at the same moment, and the later one sometimes overtakes the earlier. Users see replies before the question. No more than five messages in a run may be handled out of order, and all must still be delivered.",
		Diagram: chatOrder(false), Solution: chatOrder(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxOrderViolations: i(5), MinJobsCompleted: f(0.97), MaxCostPerHour: f(0.8), StarCostPerHour: f(0.7)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Open the log's counters: how many messages were handled after a newer message of the same conversation?",
			"A partition is read by one worker in order. If the messages of one conversation are spread over all partitions, which workers handle them?",
			"The log's partitioning setting decides where a message goes. Which choice keeps all messages of one key in the same place?",
		},
		Answer: "The answer: partition the log by key (the conversation). All messages of one conversation then go to one partition and one worker, in order. The price is that one very busy conversation cannot be spread over several workers.",
		Predictions: []Prediction{{ID: "p1", Question: "Messages are spread evenly over four partitions. What does that do to ordering within one conversation?", Options: []string{"It is lost, since different workers handle them at once", "It is kept, since each partition is ordered", "It is kept, if there are enough workers"}, Answer: 0,
			Explain: "A partition is ordered on its own. Messages of one conversation in different partitions are handled by different workers, in no particular order."}},
		Debrief: "A log orders messages within a partition, not across partitions. To keep a conversation in order, send all its messages to the same partition by key. That caps how fast one key can be handled, because one worker reads its partition. A sequence number on each message lets the receiver detect and reorder what still arrives late.",
	})
}

func (c *catalog) addFailingRefund() {
	// The refund that fails: a saga whose undo calls can also be declined
	failingRefund := func(fixed bool) engine.Diagram {
		retries := 0.0
		if fixed {
			retries = 5
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 50.0, "readPct": 0.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 4000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "queueCap": 1000.0, "dbPool": 50.0, "workflow": "saga", "compensationRetries": retries}).
			n("stock", engine.KDatabase, cols[2], 40, P{"connections": 16.0, "writeMs": 10.0}).label("stock", "Stock database").
			n("card", engine.KExternalAPI, cols[2], 160, P{"latencyMs": 40.0, "errorPct": 25.0}).label("card", "Card processor").
			n("ship", engine.KExternalAPI, cols[2], 280, P{"latencyMs": 30.0, "errorPct": 20.0}).label("ship", "Shipping service").
			e("users", "app").e("app", "stock").e("app", "card").e("app", "ship").done()
	}
	c.register(&Scenario{
		ID: "failing-refund", Title: "The refund that fails", Difficulty: "Hard", ConfigOnly: true,
		Summary: "The saga undoes a charge, and the undo is declined too.",
		Story:   "Checkout is already a saga: reserve the item, charge the card, book shipping, and undo in reverse when a step fails. Today shipping fails one time in five. The undo of the charge is a refund to the same card processor, which declines one call in four, and when the refund fails the order is stuck: the customer is charged and has nothing. Support is drowning. At most three orders may end up stuck, and a failed order should still fail within the half-second budget.",
		Diagram: failingRefund(false), Solution: failingRefund(true),
		Goals:       engine.Goals{MinSuccessRate: 0.55, MaxInconsistent: i(3), MaxP95Ms: f(500), MaxCostPerHour: f(1.6), StarCostPerHour: f(1.4)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the orders left half done on the goals panel: the saga is on, so why are some still stuck?",
			"A saga only helps if its undo calls succeed. The undo of the charge goes through the same flaky processor. What would you do when a refund fails?",
			"A failed refund can be tried again, since it can be made safe to repeat. The saga has a setting for how many times it tries an undo again.",
		},
		Answer: "The answer: have the saga try a failed undo again a few times (the setting for how many times it tries an undo again). Each retry has the same chance of success, so a few retries make a stuck order rare; a real undo must also be safe to repeat, which this simulation assumes.",
		Predictions: []Prediction{{ID: "p1", Question: "A refund is declined one time in four. With three retries, about how many refunds still fail?", Options: []string{"About one in two hundred and fifty", "About one in eight, give or take", "About one in four, same as before"}, Answer: 0,
			Explain: "All four tries must fail: a quarter to the fourth power is about 1 in 256, if failures are independent."}},
		Debrief: "A saga moves the problem from the transaction to the undo, which can fail like any step. Make each compensation {{idempotency|idempotent}} so it can be retried safely, retry it with a limit, and send what still fails to a queue a person works through. Count the orders that end up stuck, because that is the real size of the problem.",
		Deeper:  "In a saga every step has a compensation, and a compensation is a call to another service, so it can fail, time out or succeed without the caller knowing. The ways to make that safe are to make compensations idempotent (an idempotency key per order) so a retry never refunds twice, to retry with a limit and backoff, to persist the saga's state so a crash does not forget which undos remain, and to route the ones that still fail to a dead-letter queue or a manual queue with an alert. Compensations that cannot be undone, such as an email sent, belong at the end of the saga. The cost of retrying is a longer failed order and more load on a processor that is already struggling, so a circuit breaker belongs here too. An interviewer will ask what happens when the compensation itself fails and how you know an order is stuck.",
	})
}

func (c *catalog) addBooksBalance() {
	// The books: every charge needs its order and every order its charge
	books := func(fixed bool) engine.Diagram {
		retries := 0.0
		if fixed {
			retries = 4
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 40.0, "readPct": 0.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 4000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "queueCap": 1000.0, "dbPool": 50.0, "workflow": "saga", "compensationRetries": retries}).
			n("orders", engine.KDatabase, cols[2], 40, P{"connections": 16.0, "writeMs": 10.0}).label("orders", "Orders").
			n("card", engine.KExternalAPI, cols[2], 160, P{"latencyMs": 40.0, "errorPct": 15.0}).label("card", "Card processor").
			n("ledger", engine.KExternalAPI, cols[2], 280, P{"latencyMs": 25.0, "errorPct": 20.0}).label("ledger", "Ledger service").
			e("users", "app").e("app", "orders").e("app", "card").e("app", "ledger").done()
	}
	c.register(&Scenario{
		ID: "books-balance", Title: "The books must balance", Difficulty: "Hard", ConfigOnly: true,
		Summary: "At midnight finance finds charges with no order behind them.",
		Story:   "Checkout reserves the order, charges the card and posts the entry to the ledger service, and undoes the steps already done when one fails. The ledger service declines one call in five, and so does the undo of a charge on the card processor sometimes. Every night finance checks that each charge has its order and each order its charge. Entries that do not balance have to be fixed by hand. Keep them at none while at least 55% of orders succeed.",
		Diagram: books(false), Solution: books(true),
		Goals:       engine.Goals{MinSuccessRate: 0.55, MaxUnbalanced: i(0), MaxP95Ms: f(500), MaxCostPerHour: f(1.6), StarCostPerHour: f(1.4)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Open the goals: how many entries do not balance, and which step was running when each was left?",
			"An undo can fail like any other call. What does the checkout do when the undo of a charge is declined?",
			"The checkout's saga has a setting for repeating an undo that fails. How many tries does it need before none is left over?",
		},
		Answer: "The answer: let the saga retry a failed undo (the compensation retries setting). Charges that are undone cleanly leave no entry without its partner; without retries each declined refund strands one for finance to fix by hand.",
		Predictions: []Prediction{{ID: "p1", Question: "Checkout is switched from a saga to two-phase commit, with no other change. Will the books balance?", Options: []string{"Yes, since a commit is all or nothing", "No, the release that undoes a step can still be declined", "Yes, since it never undoes anything"}, Answer: 1,
			Explain: "Two-phase commit still has to release or undo what the services prepared, and a call that fails there leaves its entry behind. It does not retry them either."}},
		Debrief: "A ledger balances when every entry has its counterpart, and the nightly reconciliation is the check that finds the ones that do not. In a distributed workflow the usual leaks are a step done with no partner, a write that was saved but never announced, and an undo that failed. Undo calls have to be as reliable as the steps they undo: repeat them, make them {{idempotency|safe to repeat}}, and keep a list of the entries still open so that a person or a job can finish them.",
		Deeper:  "Reconciliation compares two records of the same facts, here the orders and the charges, and reports the entries that exist on one side only. Whatever the cause, the cure has the same shape: a durable record of what was started, a repeat that can do no harm when it runs twice, and a sweeper that finishes what the first attempt left open. An undo with retries is that shape inside one request. Money systems add a second line, a periodic job that matches the two sides and raises the differences, because no protocol removes every leak. Counting what the sweep finds, as this goal does, shows whether the design leaks at all.",
	})
}

func (c *catalog) addWhoIsOnline() {
	// Presence: a heartbeat from every user is a write, and it does not need to survive a restart
	presence := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 1500.0, "readPct": 10.0, "keySpace": 20000.0, "keyDist": "uniform", "timeoutMs": 1000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 64.0, "cpuMs": 1.0, "queueCap": 400.0})
		if fixed {
			return b.n("presence", engine.KKVStore, cols[2], rowMid, P{"nodes": 5.0, "writeQuorum": 1.0, "readQuorum": 1.0}).label("presence", "Presence").
				e("users", "app").e("app", "presence").done()
		}
		return b.n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "readMs": 2.0, "writeMs": 15.0}).label("db", "Presence").
			e("users", "app").e("app", "db").done()
	}
	c.register(&Scenario{
		ID: "who-is-online", Title: "Who is online", Difficulty: "Medium",
		Summary: "Every user's heartbeat is a database write, and the design cannot keep up.",
		Story:   "A chat app shows a green dot next to friends who are online. Each connected user sends a heartbeat every few seconds, 1,500 a second in all, and the app writes each one to the database. Most of the traffic is writes of a value that is useless a minute later; the occasional read asks who is online. The database does a write in 15 ms and has 16 connections. Serve at least 97% of requests with a {{p95|95th percentile}} under 300 ms.",
		Diagram: presence(false), Solution: presence(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(300), MaxCostPerHour: f(3.5), StarCostPerHour: f(3.0)},
		DurationSec: 30, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at what is full. Is the app server short of threads, or the place the heartbeats are written?",
			"Does a heartbeat need to survive a restart? It is replaced by the next one a few seconds later.",
			"A key-value store spreads writes over several nodes and can acknowledge after one copy. Would that be enough for a value that is replaced every few seconds?",
		},
		Answer: "The answer: keep presence in a key-value store that acknowledges after one copy, instead of the database. A heartbeat is a short-lived, constantly replaced value, so a lost copy costs a few seconds of wrong green dots, while a durable write per heartbeat costs more than one database can do. Sending heartbeats less often also passes, at the price of dots that stay green longer.",
		Predictions: []Prediction{{ID: "p1", Question: "Heartbeats are sent every 10 seconds instead of every 5. What happens to the writes?", Options: []string{"They halve, and a departure is noticed later", "They halve, and a departure is noticed sooner", "They stay the same, since the same users are connected"}, Answer: 0,
			Explain: "Each user sends half as many heartbeats, so there are half as many writes. The last heartbeat of a user who left is older for longer before it counts as missing, so the dot stays green longer."}},
		Debrief: "Presence is soft state: it is rewritten every few seconds, it is wrong the moment it is read, and losing it costs nothing lasting. Paying for durable writes on it wastes the database. A store that spreads writes over several nodes and acknowledges after one fits. In a real store an expiry per entry (not modelled here) removes a user whose heartbeats stop, and its length sets how long a green dot stays wrong. The interval is a trade-off in its own right: a shorter heartbeat finds departures sooner and multiplies the writes, and telling every friend on each change multiplies them again, so large systems send changes only to friends who are looking.",
	})
}

func (c *catalog) addFriendsOfFriends() {
	// Friends of friends: every hop multiplies the rows a read touches
	friendsOfFriends := func(fixed bool) engine.Diagram {
		db := P{"connections": 16.0, "readMs": 2.0}
		if fixed {
			db["model"] = "graph"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 120.0, "readPct": 100.0, "keySpace": 1000000.0, "keyDist": "uniform", "timeoutMs": 1500.0, "retries": 0.0, "friendHops": 2.0, "friendsEach": 200.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 64.0, "cpuMs": 1.0}).
			n("db", engine.KDatabase, cols[2], rowMid, db).label("db", "Social graph").
			e("users", "app").e("app", "db").done()
	}
	c.register(&Scenario{
		ID: "friends-of-friends", Title: "Friends of friends", Difficulty: "Hard", ConfigOnly: true,
		Summary: "\"People you may know\" reads 40,000 rows for every user.",
		Story:   "The suggestions page lists the friends of a user's friends. A user has about 200 friends, so one hop touches 200 rows and two hops touch 40,000. The social graph lives in a relational database that joins them, and 120 users a second ask. Answer at least 97% of requests with a {{p95|p95}} under 400 ms.",
		Diagram: friendsOfFriends(false), Solution: friendsOfFriends(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(400), MaxCostPerHour: f(3.0), StarCostPerHour: f(2.6)},
		DurationSec: 30, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the database: how busy is it, and what does each read cost compared with a plain read?",
			"The work grows with the number of friends to the power of the hops. Which of the two can the design change?",
			"Is a database that joins tables the best place for data whose main query is \"follow the links\"? The data model setting says how a store handles that, and a cap on the neighbours followed bounds the work.",
		},
		Answer: "The answer: store the graph in a graph database (the data model setting). Following a link there is a pointer, a tenth of the cost of a join row, so two hops cost a tenth as much. Capping the neighbours followed per hop also works, by returning a sample instead of every friend of a friend.",
		Predictions: []Prediction{{ID: "p1", Question: "Users have 200 friends each. How many more rows does a read touch at three hops than at two?", Options: []string{"About 200 times as many", "About 1.5 times as many", "The same, since the friends overlap"}, Answer: 0,
			Explain: "Each hop multiplies the rows by the number of friends, so a third hop reaches about 200 times as many rows as the second."}},
		Debrief: "Following links makes the work grow with the number of friends to the power of the hops, so a few popular users and one more hop dominate everything else. A relational database pays a join for every row on the way. A graph database stores each user's neighbours next to the user and follows them as pointers, which is much cheaper per row but still grows the same way with every hop. The lasting fixes bound the work: cap the neighbours per hop and return a sample, limit the depth, or precompute the result for the users who ask most and keep it fresh with a cache.",
		Deeper:  "Graph queries are a different shape from key lookups: the cost depends on the neighbourhood that is reached, not on the size of the data. Real systems cap the fan-out per hop, rank the neighbours so that the sample is the useful part, and keep suggestions as a precomputed list refreshed in the background, because computing a two-hop answer per request would not survive one user with a hundred thousand followers. A graph store makes the traversal cheap; it does not make a celebrity cheap.",
	})
}
