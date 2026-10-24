package scenarios

import "sysdesign/internal/engine"

// Open challenges give a blank canvas and a list of requirements. The traffic curve and the failures are hidden
// from the client (see the API) and no reference solution is ever served; Solution here is a witness that
// proves the requirements can be met, used only by the tests.
func init() {
	blank := func(users P) engine.Diagram {
		return (&bld{}).n("users", engine.KClient, cols[0], rowMid, users).done()
	}
	witness := func(users P, n int, app, cache, db P) engine.Diagram {
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, users).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).e("users", "lb").
			n("db", engine.KDatabase, cols[4], rowMid, db).
			n("cache", engine.KCache, cols[3], rowTop, cache)
		return b.servers(n, cols[2], rowTop, 70, app, "cache").done()
	}

	students := P{"rps": 100.0, "readPct": 80.0}
	register(&Scenario{
		ID: "ten-thousand-students", Title: "Ten thousand students", Difficulty: "Hard", Open: true,
		Summary: "Results day: build a site that survives it, within budget.",
		Story:   "A university will publish exam results at nine o'clock and ten thousand students will refresh the page. Build the system that serves them from a blank canvas: at least 97% of requests must succeed, p95 latency must stay under 500 ms, and it must cost no more than $1.50 an hour to run. You are not told how traffic will behave, only that today's rate is a poor guide, and that servers have a habit of failing at the worst moment.",
		Diagram: blank(students),
		Solution: witness(students, 4, P{"threads": 16.0, "callTimeoutMs": 400.0, "circuitBreaker": true, "dbPool": 20.0},
			P{"ttlSec": 30.0, "ttlJitterPct": 20.0}, P{"maxConnections": 500.0}),
		Traffic:     map[string][]engine.TrafficPoint{"users": {{0, 100}, {10, 100}, {25, 900}, {50, 900}, {60, 200}, {70, 200}}},
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "random:appServer", AtSec: 35, DownSec: 20}},
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(500), MaxCostPerHour: f(1.5)},
		DurationSec: 70, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Estimate first: one app server with 16 threads handles very roughly how many requests a second? Compare that with what a rush could ask for.",
			"Most requests are reads of the same results: something in front of the database can absorb them. And a server can fail: how many do you need if one is gone?",
			"Keep spending in check: every server you run costs money all day. If you pass the success goal but miss the budget, you have too many; if you miss success, too few.",
		},
		Answer:  "The answer: a load balancer, four 16-thread app servers with a call timeout of about 400 ms and a circuit breaker, and a cache with a 30 second TTL and some jitter in front of the database. That is the reference witness the tests use; other shapes can pass too.",
		Debrief: "There is no single right answer, but the shape of one: a balancer, enough app servers that losing one still leaves headroom, a cache for the popular reads, and call timeouts so a failure stays a failure instead of a stall. Size by arithmetic (requests a second divided by what one server handles, plus one), then check it with a run.",
	})

	sale := P{"rps": 60.0, "readPct": 60.0}
	register(&Scenario{
		ID: "checkout-during-a-sale", Title: "Checkout during a sale", Difficulty: "Hard", Open: true,
		Summary: "A flash sale, a write-heavy checkout and a database that stumbles.",
		Story:   "A shop is starting a flash sale. Four in ten requests change something (baskets, stock, orders). Build the whole system from a blank canvas: at least 94% of requests must succeed, p95 latency must stay under 800 ms, and it must cost less than $1.90 an hour. The database is the part you cannot fully trust.",
		Diagram: blank(sale),
		Solution: witness(sale, 3, P{"threads": 16.0, "callTimeoutMs": 500.0, "circuitBreaker": true},
			P{"ttlSec": 30.0, "ttlJitterPct": 20.0, "writePolicy": "writeThrough"}, P{"connections": 48.0}),
		Traffic:     map[string][]engine.TrafficPoint{"users": {{0, 60}, {10, 60}, {20, 500}, {50, 500}, {60, 100}}},
		Chaos:       []engine.ChaosEvent{{Type: "slow", NodeID: "db", AtSec: 30, Factor: 6, DurationSec: 10}},
		Goals:       engine.Goals{MinSuccessRate: 0.94, MaxP95Ms: f(800), MaxCostPerHour: f(1.9)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Writes cannot be served from a cache. Which part of your design has to do them, and how many can it do at once?",
			"The database's 'Concurrent queries' setting decides how many writes it works on at the same time. The default is small.",
			"What stops a slow database from taking the servers down with it? More of everything gets expensive quickly: check the budget.",
		},
		Answer:  "The answer: a load balancer, three 16-thread app servers with a call timeout of about 500 ms and a circuit breaker, a write-through cache, and the database's concurrent queries raised to 48. That is the reference witness the tests use; other shapes can pass too.",
		Debrief: "Writes are the capacity question: a cache does not remove them, so the database's concurrency has to match the write rate, and the servers have to stay healthy while it stumbles. Raising the database's concurrent queries was worth more than adding servers.",
	})
}
