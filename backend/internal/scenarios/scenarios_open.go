package scenarios

import "sysdesign/internal/engine"

const guidedID = "first-design-links"

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
		Deeper:  "Start with arithmetic: peak requests per second divided by what one server handles, plus a spare so that losing one does not drop you below the load, and then check the cost against the budget before adding anything clever. The cache absorbs the repeated reads of the same results, and a little jitter on expiry stops all entries going stale together; timeouts and a circuit breaker keep a failing dependency from holding every thread. The part that is easy to miss is the story's warning that today's rate is a poor guide, so you size for the rush, and you test with a failure injected rather than only on the happy path. An interviewer will ask what you would cut first if the bill doubled, what you would pre-warm before nine o'clock, and whether a static page served from a CDN would remove most of the problem. The failure pattern is a design that works on the happy path and falls over when the first server fails at the peak.",
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
		Deeper:  "With a write-heavy mix the database is the capacity question: a cache helps the reads but every write still reaches the database, so its concurrency has to match the write rate before extra servers help at all. A write-through cache keeps what it serves consistent with what was written, at the cost of every write paying for both; the servers need timeouts and a circuit breaker so that a slow database produces fast errors and not a pile of stuck threads. At larger scale you would queue the writes that need not be synchronous, shard the order data, and shed the least important traffic first. An interviewer will ask what you do with checkout while the database is slow, whether you accept orders and process them later, and how you avoid charging twice when clients retry. The failure pattern is the slow dependency: it holds threads, the retries pile on, and a stumble becomes an outage.",
	})

	links := P{"rps": 80.0, "readPct": 90.0, "keySpace": 20000.0, "keyDist": "zipf"}
	register(&Scenario{
		ID: guidedID, Title: "Your first design: a link shortener", Difficulty: "Easy", Open: true,
		Summary:  "A guided blank page: place the first components, then size them.",
		Story:    "People paste a long web address and get a short one back; anyone who opens the short one is sent to the long one. Nine in ten requests are people following links. Build it from a blank canvas: at least 95% of requests must succeed, p95 latency must stay under 500 ms, and it must cost no more than $0.55 an hour. The checklist shows what any design like this needs; how many of each, and how they are joined, is up to you. A server can fail while people are using it.",
		Diagram:  (&bld{}).n("users", engine.KClient, cols[0], rowMid, links).done(),
		Solution: guidedDesign(3),
		Starter: []StarterStep{
			{Need: "Something that sends the requests", Kind: engine.KClient, Nudge: "It is already on the canvas: the people using your service."},
			{Need: "Something that runs your code for every request", Kind: engine.KAppServer, Nudge: "Look in the Compute group of the library. Each request holds one of its threads until it is finished."},
			{Need: "Somewhere to keep every link for good", Kind: engine.KDatabase, Nudge: "Look in the Data group. It must survive a restart, so memory alone is not enough."},
		},
		Traffic:     map[string][]engine.TrafficPoint{"users": {{0, 80}, {10, 80}, {25, 300}, {45, 300}, {55, 80}, {60, 80}}},
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "random:appServer", AtSec: 30, DownSec: 15}},
		Goals:       engine.Goals{MinSuccessRate: 0.95, MaxP95Ms: f(500), MaxCostPerHour: f(0.55)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Estimate first: one 8-thread server handles roughly how many requests a second? Compare that with a rush of about 300.",
			"A server can fail while the rush is on. If one is lost, will the others still carry the load?",
			"Each server costs money all day. Which number of servers keeps the success rate up but stays inside the budget?",
		},
		Answer:  "The answer: a load balancer, three 8-thread app servers and one database. Two servers cannot carry the rush when one of them fails, and a fourth goes over the budget. That is the reference witness the tests use; other shapes can pass too.",
		Debrief: "The first design has three parts: something that takes requests, something that runs the code, and something that keeps the data. Then comes the sizing: divide the peak by what one server handles, add one so a failure does not hurt, and check the bill before adding anything more.",
	})
}

// guidedDesign is the link shortener with n app servers behind a balancer, all using one database.
func guidedDesign(n int) engine.Diagram {
	return (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 80.0, "readPct": 90.0, "keySpace": 20000.0, "keyDist": "zipf"}).
		n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).e("users", "lb").
		n("db", engine.KDatabase, cols[4], rowMid, nil).
		servers(n, cols[2], rowTop, 70, P{"threads": 8.0}).done()
}
