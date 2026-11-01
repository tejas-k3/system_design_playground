package scenarios

import "sysdesign/internal/engine"

// Open Hard briefs: a blank canvas, a requirement list, hidden load and failures. The witness only proves that the
// requirements can be met; it is never served.
func (c *catalog) registerOpenHardBriefs() {
	blank := func(users P) engine.Diagram {
		return (&bld{}).n("users", engine.KClient, cols[0], rowMid, users).done()
	}

	// The database dies: the answer is a replica that can take over, and the settings that say how fast
	dbUsers := P{"rps": 100.0, "readPct": 40.0, "keyDist": "uniform", "keySpace": 20000.0}
	c.register(&Scenario{
		ID: "open-database-failure", Title: "Survive the database dying", Difficulty: "Hard", Open: true,
		Summary: "Build a service whose only database is going to fail for a while.",
		Story:   "A service takes 100 requests a second, six in ten of them writes. Build it from a blank canvas: at least 85% of requests must succeed, p95 latency must stay under 800 ms, no more than 100 acknowledged writes may be lost, and it must cost no more than $2.50 an hour. You are not told when, or for how long, something will fail.",
		Diagram: blank(dbUsers),
		Solution: (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, dbUsers).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).e("users", "lb").
			n("db", engine.KDatabase, cols[4], rowTop, P{"onFailure": "promote", "detectSec": 3.0, "promoteSec": 5.0}).
			n("replica", engine.KReplica, cols[4], rowBot, P{"replicationLagMs": 500.0}).
			servers(2, cols[2], rowTop, 100, P{"threads": 16.0}, "replica").done(),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "random:database", AtSec: 25, DownSec: 40}},
		Goals:       engine.Goals{MinSuccessRate: 0.85, MaxP95Ms: f(800), MaxLostWrites: i(100), MaxCostPerHour: f(2.5)},
		DurationSec: 70, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Every request that writes needs the database. What does the service do if that one machine is gone for a while?",
			"A copy of the data on another machine can take over. What does the database need to be told to do when it fails, and how up to date is the copy?",
			"Two numbers set the damage: how long it takes to notice and switch, and how much the copy had not yet received. Both can be tuned, and both cost something.",
		},
		Answer:     "The answer: a load balancer, two app servers, the database with a read replica and the database set to promote the replica when it fails, with short detection and promotion times and a small replication lag. That is the witness the tests use; other shapes can pass too.",
		Debrief:    "A single database is a single point of failure for every write. A replica that can be promoted ({{failover|failover}}) turns an outage into a short pause, at a price in data: whatever it had not yet copied is lost. Detection time, promotion time and replication lag are the three dials.",
		Deeper:     "Failover has two separate costs. Time is detection plus promotion, during which writes fail; data is whatever the replica had not received when the primary died, because replication is asynchronous. Detecting faster risks promoting on a blip and creating two primaries, so the new primary must be fenced and the old one stopped before it rejoins. Synchronous replication removes the data loss and puts a second machine on the path of every write. Reads can keep working from the replica throughout. An interviewer will ask what you lose, how long it takes, and how you know the old primary is really gone.",
		WeakPoints: []string{"Two primaries are possible if the old one comes back and nobody stops it.", "Acknowledged writes that had not reached the replica are lost.", "The load balancer is still a single machine."},
	})

	// Pay once: slow writes make the clients retry, and the answer is a key that makes a retry harmless
	payUsers := P{"rps": 30.0, "readPct": 0.0, "timeoutMs": 450.0, "retries": 2.0, "backoff": "none"}
	c.register(&Scenario{
		ID: "open-pay-once", Title: "Charge once", Difficulty: "Hard", Open: true,
		Summary: "Build a payment path that survives a slow database without charging twice.",
		Story:   "A checkout takes 30 orders a second and charges a card for each. Clients give up after 450 ms and retry up to twice at once. Build it from a blank canvas: at least 95% of requests must succeed, no order may be charged twice, p95 latency must stay under 1.5 seconds, and it must cost no more than $1.50 an hour. You are not told what will go wrong.",
		Diagram: blank(payUsers),
		Solution: (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, payUsers).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 24.0, "idempotencyKeys": true}).
			n("pay", engine.KExternalAPI, cols[2], rowTop, P{"latencyMs": 70.0, "errorPct": 0.0}).label("pay", "Payments API").
			n("db", engine.KDatabase, cols[2], rowBot, P{"connections": 32.0}).
			e("users", "app").e("app", "pay").e("app", "db").done(),
		Chaos:       []engine.ChaosEvent{{Type: "slow", NodeID: "random:database", AtSec: 8, Factor: 6, DurationSec: 25}},
		Goals:       engine.Goals{MinSuccessRate: 0.95, MaxDuplicates: i(0), MaxP95Ms: f(1500), MaxCostPerHour: f(1.5)},
		DurationSec: 40, WarmupSec: 4, Speed: 1,
		Hints: [3]string{
			"When a write is slow, the client gives up and sends it again. What has happened to the first attempt?",
			"The first attempt is still running and will charge the card too. The retry cannot tell, unless something recognises it as the same order.",
			"Tag each order with an identifier and let the server skip an order it has already handled. Which server setting does that?",
		},
		Answer:     "The answer: an app server in front of the payments API and the database, with idempotency keys turned on, so a retry of an order that was already handled is recognised and not charged again. That is the witness the tests use; other shapes can pass too.",
		Debrief:    "Retries are only safe for operations that can be repeated. An {{idempotency|idempotency key}} makes a charge repeatable: the second attempt with the same key returns the first attempt's result. The key must be stored for as long as a client might retry, and checked before the charge, not after.",
		Deeper:     "A timeout does not mean the operation failed, only that the caller stopped waiting, so a retry may repeat work that is still in flight or already done. The defence is an idempotency key per logical operation, stored with the result, so the server returns the earlier result instead of acting again. The key store needs the same durability as the operation, and keys must live as long as a retry can arrive. Retrying with backoff and jitter limits the extra load; a deadline passed downstream stops work nobody is waiting for. An interviewer will ask where the key is checked, how long you keep it, and what happens if the payment provider has its own key.",
		WeakPoints: []string{"The keys are only as durable as their store.", "A payment provider that does not honour keys can still double charge.", "Nothing here undoes a charge when a later step fails."},
	})
}
