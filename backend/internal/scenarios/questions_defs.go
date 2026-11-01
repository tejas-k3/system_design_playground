package scenarios

import (
	"fmt"

	"sysdesign/internal/engine"
)

const (
	secPerDay   = 86400.0
	secPerMonth = 30 * secPerDay
)

func (c *catalog) registerSizingQuestions() {
	// URL shortener: 100 M new URLs a month, 10 reads per write, 3x peak, 500 B a record, kept 5 years.
	writeQps := 100e6 / secPerMonth
	c.registerQuestion(&Question{
		build:     &buildSpec{field: "peakReadQps", perServer: 400, rps: 30 * writeQps, readPct: 91, seconds: 30},
		BuildHint: "Build it: one app server for every 400 requests a second of your peak read estimate (a cache and a database behind a balancer), then test it against the real peak.",
		ID:        "url-shortener-numbers", Title: "A URL shortener: how big?", Mode: ModeEstimate, Difficulty: "Easy",
		Summary: "Turn monthly volume into queries per second and terabytes.",
		Prompt:  "Before drawing anything, size the service. Use 30-day months and 86,400 seconds a day. Peak traffic is three times the average.",
		Facts: []Fact{
			{"New short links", "100 million a month"}, {"Reads per write", "10"}, {"Peak factor", "3 x average"},
			{"Record size", "500 bytes"}, {"Retention", "5 years"},
		},
		Fields: []EstimateField{
			{Key: "writeQps", Label: "Average writes per second", Unit: "per second", Expected: writeQps, Explain: fmt.Sprintf("100 million / (30 x 86,400 s) = %.1f writes per second.", writeQps)},
			{Key: "readQps", Label: "Average reads per second", Unit: "per second", Expected: 10 * writeQps, Explain: fmt.Sprintf("Ten reads for every write: 10 x %.1f = %.0f reads per second.", writeQps, 10*writeQps)},
			{Key: "peakReadQps", Label: "Peak reads per second", Unit: "per second", Expected: 30 * writeQps, Explain: fmt.Sprintf("Three times the average read rate: %.0f per second. Provision for the peak, not the average.", 30*writeQps)},
			{Key: "storageTB", Label: "Storage after 5 years", Unit: "TB", Expected: 100e6 * 12 * 5 * 500 / 1e12, Explain: "100 million x 12 months x 5 years = 6 billion records; x 500 bytes = 3 TB. Small enough for one database, which is the point of doing the sum."},
		},
	})

	// Photo feed: 10 M daily users view 50 photos of 200 KB; 90 % served by the CDN.
	reads := 10e6 * 50 / secPerDay
	egress := reads * 200e3 * 8 / 1e9
	c.registerQuestion(&Question{
		ID: "photo-feed-numbers", Title: "A photo feed: how much bandwidth?", Mode: ModeEstimate, Difficulty: "Medium",
		Summary: "Requests per second, then gigabits, then what the CDN saves you.",
		Prompt:  "Size the read path of a photo feed. Use 86,400 seconds a day and 8 bits to the byte. Spread the load evenly over the day for this exercise.",
		Facts: []Fact{
			{"Daily users", "10 million"}, {"Photos viewed per user per day", "50"}, {"Average photo", "200 KB"}, {"Served from the CDN", "90 %"},
		},
		Fields: []EstimateField{
			{Key: "readQps", Label: "Photo requests per second", Unit: "per second", Expected: reads, Explain: fmt.Sprintf("10 million x 50 / 86,400 s = %.0f requests per second.", reads)},
			{Key: "egressGbps", Label: "Total bandwidth to users", Unit: "Gbit/s", Expected: egress, Explain: fmt.Sprintf("%.0f per second x 200 KB x 8 bits = %.2f Gbit/s leaving the building: a lot for one data centre link.", reads, egress)},
			{Key: "originGbps", Label: "Bandwidth your servers must serve", Unit: "Gbit/s", Expected: 0.1 * egress, Explain: fmt.Sprintf("Only the 10%% the CDN misses reaches you: %.2f Gbit/s. This is what the CDN is for.", 0.1*egress)},
		},
	})

	// Chat: 50 M daily users send 40 messages of 100 B each; keep 3 years.
	msgs := 50e6 * 40 / secPerDay
	c.registerQuestion(&Question{
		ID: "chat-numbers", Title: "A chat service: how much to store?", Mode: ModeEstimate, Difficulty: "Medium",
		Summary: "Message rate, its peak, and three years of history.",
		Prompt:  "Size a chat service's write path and its storage. Use 86,400 seconds a day and 365 days a year. Peak traffic is three times the average.",
		Facts: []Fact{
			{"Daily users", "50 million"}, {"Messages sent per user per day", "40"}, {"Message size", "100 bytes"}, {"Retention", "3 years"}, {"Peak factor", "3 x average"},
		},
		Fields: []EstimateField{
			{Key: "msgPerSec", Label: "Average messages per second", Unit: "per second", Expected: msgs, Explain: fmt.Sprintf("50 million x 40 / 86,400 s = %.0f messages per second.", msgs)},
			{Key: "peakMsgPerSec", Label: "Peak messages per second", Unit: "per second", Expected: 3 * msgs, Explain: fmt.Sprintf("3 x %.0f = %.0f per second: size the write path for this.", msgs, 3*msgs)},
			{Key: "storageTB", Label: "Storage after 3 years", Unit: "TB", Expected: 50e6 * 40 * 365 * 3 * 100 / 1e12, Explain: "50 million x 40 = 2 billion messages a day; x 365 x 3 years = about 2.2 trillion; x 100 bytes = about 219 TB. Too big for one machine: sharding is not optional."},
		},
	})

	c.registerQuestion(&Question{
		ID: "daily-storage-estimate", Title: "How much storage arrives each day?", Mode: ModeEstimate, Difficulty: "Easy",
		Summary: "Turn a daily record count into storage per day.",
		Prompt:  "A service stores 20 million small records each day. Each record uses 500 bytes. How many gigabytes of new data does it store per day? Use 1,000,000,000 bytes per gigabyte.",
		Facts:   []Fact{{"Records stored each day", "20 million"}, {"Size per record", "500 bytes"}},
		Fields:  []EstimateField{{Key: "storageGBPerDay", Label: "New storage per day", Unit: "GB per day", Expected: 10, Explain: "20 million records x 500 bytes = 10,000,000,000 bytes; divide by 1,000,000,000 bytes per GB = 10 GB per day."}},
	})

	c.registerQuestion(&Question{
		ID: "daily-users-rps", Title: "How many requests arrive each second?", Mode: ModeEstimate, Difficulty: "Easy",
		Summary: "Turn daily users and their actions into an average request rate.",
		Prompt:  "A service has 8.64 million users each day, and each user makes 10 requests. Spread requests evenly across the day and use 86,400 seconds per day. How many requests arrive per second?",
		Facts:   []Fact{{"Daily users", "8.64 million"}, {"Requests per user per day", "10"}},
		Fields:  []EstimateField{{Key: "requestsPerSecond", Label: "Requests per second", Unit: "per second", Expected: 1000, Explain: "8.64 million users x 10 requests = 86.4 million requests per day; divide by 86,400 seconds = 1,000 requests per second."}},
	})

	c.registerQuestion(&Question{
		ID: "image-egress-bandwidth", Title: "How much bandwidth do image views use?", Mode: ModeEstimate, Difficulty: "Easy",
		Summary: "Convert image views per second and image size into outbound bandwidth.",
		Prompt:  "A service sends 2,500 images each second. Each image is 500 KB. Use 1,000 bytes per KB and 8 bits per byte. How many gigabits per second does it send?",
		Facts:   []Fact{{"Image views", "2,500 per second"}, {"Average image size", "500 KB"}},
		Fields:  []EstimateField{{Key: "bandwidthGbps", Label: "Outbound bandwidth", Unit: "Gbit/s", Expected: 10, Explain: "2,500 images/s x 500 KB x 1,000 bytes/KB x 8 bits/byte = 10,000,000,000 bits/s; divide by 1,000,000,000 = 10 Gbit/s."}},
	})

	c.registerQuestion(&Question{
		ID: "yearly-storage-replicated", Title: "How much storage does a year need?", Mode: ModeEstimate, Difficulty: "Easy",
		Summary: "Turn daily data into a year of storage, with every byte kept on several machines.",
		Prompt:  "A service adds 200 GB of new data each day, and every byte is kept on 3 copies so a machine can fail. Use 365 days per year and 1,000 GB per TB. How many terabytes of disk does one year of data need?",
		Facts:   []Fact{{"New data each day", "200 GB"}, {"Copies of every byte", "3"}},
		Fields:  []EstimateField{{Key: "storageTBPerYear", Label: "Disk needed for one year", Unit: "TB", Expected: 219, Explain: "200 GB x 3 copies = 600 GB a day; x 365 days = 219,000 GB; divide by 1,000 GB per TB = 219 TB. Replication multiplies the disk you buy, not the data you have."}},
	})

	c.registerQuestion(&Question{
		ID: "peak-load-estimate", Title: "How big is the peak?", Mode: ModeEstimate, Difficulty: "Easy",
		Summary: "Turn an average request rate and a peak factor into the load to design for.",
		Prompt:  "A service averages 1,200 requests per second over the day. Its busiest minutes run at 4 times the average. How many requests per second must it handle at the peak?",
		Facts:   []Fact{{"Average load", "1,200 requests per second"}, {"Peak factor", "4 x average"}},
		Fields:  []EstimateField{{Key: "peakRequestsPerSecond", Label: "Peak requests per second", Unit: "per second", Expected: 4800, Explain: "1,200 x 4 = 4,800 requests per second. Servers must be sized for this peak, because users arrive in bursts and a queue that overflows turns the excess into errors."}},
	})

	c.registerQuestion(&Question{
		ID: "cache-size-estimate", Title: "How big must the cache be?", Mode: ModeEstimate, Difficulty: "Easy",
		Summary: "Size a cache to hold the popular share of the data.",
		Prompt:  "A shop has 10 million items of 2 KB each. The hottest 20% of items get most of the reads, and the team wants all of them in the cache. Use 1,000,000 KB per GB. How many gigabytes of cache is that?",
		Facts:   []Fact{{"Items", "10 million"}, {"Size per item", "2 KB"}, {"Share to keep in the cache", "20%"}},
		Fields:  []EstimateField{{Key: "cacheGB", Label: "Cache size", Unit: "GB", Expected: 4, Explain: "20% of 10 million = 2 million items; x 2 KB = 4,000,000 KB; divide by 1,000,000 KB per GB = 4 GB. A machine's memory holds that easily, which is why caching the hot set works."}},
	})

	c.registerDiagnose("cut-off-threads", diagSource{"cut-off", 1, 60}, &Question{
		Title: "Everything slows, then stops", Difficulty: "Medium",
		Summary: "Success falls off a cliff and the latency chart goes blank. What is going on?",
		Prompt:  "This is one minute of a web shop's dashboard and one failed request. Most reads are served by a cache. Pick the explanation that fits all of it.",
		Options: []string{
			"The cache was flushed, so every request now hits the database.",
			"The application servers' threads are stuck waiting on a database that has stopped answering, so they cannot serve even the requests that never needed it.",
			"Traffic doubled and the servers ran out of CPU.",
			"The load balancer is sending all requests to one server.",
		},
		answer:      1,
		explanation: "Requests that need the database are not refused, they just never come back. With no call timeout, each one holds a thread for as long as the outage lasts, so all the threads fill up and even cache hits queue behind them. Success falls to near zero at once, and there is no latency to chart because nothing finishes. The fix is a call timeout (so threads are released) and a circuit breaker.",
	})

	c.registerDiagnose("retry-storm-recovery", diagSource{"retry-pileup", 1, 60}, &Question{
		Title: "The trigger is gone but the outage is not", Difficulty: "Medium",
		Summary: "Something briefly slowed the system at 16 seconds. A minute later it is still down.",
		Prompt:  "Clients wait up to 800 ms and retry failures. The dashboard shows success collapsing around 16 seconds and never coming back; the failed request below was attempted five times. Which explanation fits?",
		Options: []string{
			"The database crashed and never restarted.",
			"The system is stuck in a retry storm: retries keep the queues full of work whose users have already given up, so the load never drops enough to recover even after the original slowdown ended.",
			"The traffic grew steadily until it exceeded capacity.",
			"The cache is cold and will warm up by itself in a few minutes.",
		},
		answer:      1,
		explanation: "Offered load stays flat (about 380 requests a second), yet requests in flight jump from about 50 to about 600 and stay there. A trigger (a slow database) pushed the queues over, every failure was retried immediately, and the servers kept working on the oldest queued requests, whose users had already left. The effort is wasted and the retries keep the queues full: a metastable failure that outlives its cause. The fixes are retry limits with backoff, newest-first queues, a maximum queue wait, and a circuit breaker.",
	})

	c.registerDiagnose("watch-server-down", diagSource{"server-down", 1, 60}, &Question{
		Title: "Who is at fault when a server dies?", Difficulty: "Easy",
		Summary: "A server crashes and half the requests fail. Pick the component behind most of the errors.",
		Prompt:  "This is a minute of a web shop's dashboard: a load balancer, two app servers and a database. At 20 seconds one app server crashes for 25 seconds. The failed request below is the first one to fail. Which component's behaviour explains most of the errors?",
		Options: []string{
			"The database, which dropped its connections when a server died.",
			"The load balancer, which kept sending requests to the dead server.",
			"The surviving app server, which cannot carry the whole load alone.",
			"The users, whose traffic jumped just as the server died.",
		},
		answer:      2,
		explanation: "Arrivals stay near 350 a second the whole time, so there was no surge. While one server is down the successes fall to about 190 a second, which is all one server can do. Of the roughly 3,800 failures, about 3,500 are 'Queue full' on the surviving server and only about 350 are 'Node down' from the balancer still sending to the dead one for a moment. Losing one of two servers leaves half the capacity for the whole load: size the survivors to carry the peak.",
	})

	c.registerDiagnose("watch-slow-query", diagSource{"slow-query", 1, 30}, &Question{
		Title: "The error is here, the cause is not", Difficulty: "Easy",
		Summary: "Requests are refused at the front, yet the app server is not the problem.",
		Prompt:  "This is half a minute of a service with users, one app server and one database. The failed request below was refused at the app server. Which component is really the problem?",
		Options: []string{
			"The users, who are sending more than the service was built for.",
			"The app server, whose queue is full and refuses requests.",
			"The database, which answers slowly, so server threads sit waiting.",
			"The network between them, which is dropping some requests.",
		},
		answer:      2,
		explanation: "The refusal is reported by the app server, but the cause is further along. Latency climbs from about 130 ms at the start to about 570 ms and the number of requests in flight roughly triples, while arrivals stay near 150 a second: every request is spending its time waiting for the database. A thread that waits for the database cannot take a new request, so the app server's queue fills and the refusals show up at the front. Where a symptom appears is not where the cause is: ask what the busy component is waiting for.",
	})

	c.registerDiagnose("stampede-spikes", diagSource{"cache-stampede", 1, 60}, &Question{
		Title: "A short outage that fixes itself", Difficulty: "Hard",
		Summary: "Latency climbs for a few seconds, everything fails for a few more, then all is well.",
		Prompt:  "Around 10 to 18 seconds in, in-flight requests jump to about 700 and nothing succeeds. By 20 seconds the system is back to normal throughput, with no one touching it. A failed request timed out after 2 seconds. Which explanation fits?",
		Options: []string{
			"A cache stampede: many popular cache entries expired together, so a wave of identical misses hit the database at once.",
			"The primary database crashed and failed over to a replica.",
			"A network partition cut the application servers off from the database.",
			"The autoscaler removed servers and then added them back.",
		},
		answer:      0,
		explanation: "Nothing is down: arrivals are steady before and after. What changes is how many requests miss the cache at the same moment. The cache is filled all at once with the same lifetime, so it also expires all at once; every miss goes to the database, which slows to a crawl, requests time out and are retried, and it recovers only when the cache is refilled. Fixes: randomise lifetimes (TTL jitter) and let one request refill an entry while the others wait (request coalescing).",
	})
}

func (c *catalog) registerTradeoffQuestions() {
	base := func(id string) engine.Diagram { return c.byID[id].Diagram }

	pay := base("sync-or-async")
	c.registerTradeoff("payments-replication", "sync-or-async", &Question{
		Title: "Payments: sync or async replication?", Difficulty: "Medium",
		Summary: "Run both and see what the safe choice costs.",
		Prompt:  "A payment service tells users their payment succeeded, then its primary database dies and the replica takes over. Requirement: no acknowledged payment may be lost, and p95 latency stays under 260 ms. Run each design, then pick the one to ship.",
		answer:  1, explanation: "Asynchronous replication acknowledges before the replica has the write, so 253 acknowledged payments vanished with the primary. Synchronous replication waits for the replica, so none were lost; the price was about 45 ms at p95 (185 to 229 ms), which is inside the 260 ms budget. When the requirement is zero loss, the latency is a cost you pay, not a reason to refuse. Sync also makes writes depend on the replica being reachable.",
		rule: func(r RunSummary) bool { return r.LostWrites == 0 && r.P95 < 260 },
	},
		Candidate{Label: "Asynchronous replica", Detail: "The primary acknowledges first; the replica catches up about three seconds later.", diagram: pay},
		Candidate{Label: "Synchronous replica", Detail: "The primary acknowledges only after the replica has the write.", diagram: tweak(pay, engine.KReplica, P{"replication": "sync"})},
	)

	stale := base("stale-prices")
	c.registerTradeoff("cache-write-policy", "stale-prices", &Question{
		Title: "What should a write do to the cache?", Difficulty: "Medium",
		Summary: "Four ways to keep cached prices honest, side by side.",
		Prompt:  "Prices change often. Requirement: at most 2% of reads may be stale, and p95 stays under 300 ms. Run each policy, then pick the one to ship.",
		answer:  2, explanation: "Doing nothing left 47% of reads stale. A one-second lifetime still left 23%. Invalidating on write barely helped (45%): an invalidated entry is refilled from a replica that is itself behind, so the stale value comes straight back. Write-through put the new value into the cache with the write, and stale reads fell to zero at a cost of about 6 ms at p95.",
		rule: func(r RunSummary) bool { return r.StaleReadPct <= 0.02 && r.P95 < 300 },
	},
		Candidate{Label: "Do nothing on a write", Detail: "Entries live until they are evicted.", diagram: stale},
		Candidate{Label: "Invalidate on write", Detail: "A write removes the cached entry; the next read refills it.", diagram: tweak(stale, engine.KCache, P{"writePolicy": "invalidate"})},
		Candidate{Label: "Write through", Detail: "A write updates the cached entry as well.", diagram: tweak(stale, engine.KCache, P{"writePolicy": "writeThrough"})},
		Candidate{Label: "One-second lifetime", Detail: "Entries expire after one second whatever happens.", diagram: tweak(stale, engine.KCache, P{"ttlSec": 1.0})},
	)

	storm := base("retry-pileup")
	retried := tweak(storm, engine.KClient, P{"retries": 2.0, "backoff": "exponential"})
	c.registerTradeoff("retry-policy", "retry-pileup", &Question{
		Title: "How should clients retry?", Difficulty: "Hard",
		Summary: "Retries help against blips and wreck overloaded systems. Which design survives a slow database?",
		Prompt:  "The database slows down for ten seconds. Requirement: at least 85% of requests succeed over the run, and p95 stays under 1.5 s. Run each design, then pick the one to ship.",
		answer:  3, explanation: "Four immediate retries produced a storm: 18.5% success. Two retries with backoff changed nothing (18.4%) because the queues were already full of abandoned work. Giving up on the first failure did better (75%) simply by adding no load. The design that met the requirement (90% success, p95 about 865 ms) fixed the servers: it served the newest requests first and dropped anything that had waited too long, so the queue could never fill with requests whose users had left; backoff alone cannot do that.",
		rule: func(r RunSummary) bool { return r.Success >= 0.85 && r.P95 < 1500 },
	},
		Candidate{Label: "Four immediate retries", Detail: "Every failure is retried at once, up to four times.", diagram: storm},
		Candidate{Label: "Two retries with backoff", Detail: "Exponential backoff with jitter, at most two retries.", diagram: retried},
		Candidate{Label: "No retries", Detail: "A failed request is a failed request.", diagram: tweak(storm, engine.KClient, P{"retries": 0.0})},
		Candidate{Label: "Backoff, newest first, short queue wait", Detail: "Two retries with backoff, and servers serve the newest request first and drop anything that waited more than 400 ms.", diagram: tweak(retried, engine.KAppServer, P{"queueOrder": "lifo", "queueTimeoutMs": 400.0})},
	)

	cut := base("cut-off")
	c.registerTradeoff("dependency-timeout", "cut-off", &Question{
		Title: "How do you protect against a dependency that goes silent?", Difficulty: "Medium",
		Summary: "A timeout, a circuit breaker, or both?",
		Prompt:  "The database becomes unreachable for 25 seconds; requests to it simply never return. Requirement: at least 82% success over the run. If two designs meet it, ship the simpler one. Run each design, then pick.",
		answer:  1, explanation: "With no call timeout, every thread that touched the database was stuck for the whole outage: 67% success. A 300 ms call timeout released the threads and cache hits kept flowing: 87%. Adding a circuit breaker gave no further benefit here (86% in this run), so the timeout is the simpler design that meets the requirement. A breaker earns its place when a dependency stays down long enough that even failing after 300 ms wastes capacity.",
		rule:       func(r RunSummary) bool { return r.Success >= 0.82 },
		alsoPasses: []int{2},
	},
		Candidate{Label: "No call timeout", Detail: "Servers wait as long as it takes.", diagram: cut},
		Candidate{Label: "300 ms call timeout", Detail: "Give up on the database after 300 ms.", diagram: tweak(cut, engine.KAppServer, P{"callTimeoutMs": 300.0})},
		Candidate{Label: "Timeout and circuit breaker", Detail: "A 300 ms timeout, and stop calling for a few seconds after five failures in a row.", diagram: tweak(cut, engine.KAppServer, P{"callTimeoutMs": 300.0, "circuitBreaker": true})},
	)

	rush := base("morning-rush")
	c.registerTradeoff("scale-for-the-rush", "morning-rush", &Question{
		Title: "Capacity for a daily rush", Difficulty: "Medium",
		Summary: "A bigger machine, autoscaling, or autoscaling with a floor?",
		Prompt:  "Traffic climbs from 80 to 600 requests a second and a new server takes 20 seconds to boot. Requirement: at least 98% success and a total cost under $1.00 an hour. Run each design, then pick the one to ship.",
		answer:  2, explanation: "One big server (64 threads) coped but cost about $2.48 an hour all day. Autoscaling from a single server was cheap (about $0.66) but the first minutes of the climb outran the 20-second boot time: 92% success. Keeping at least three servers meant the fleet already had headroom when the ramp began: 98.9% success for about $0.71, inside both limits. Autoscaling reacts to load; a floor covers what it cannot react to in time.",
		rule: func(r RunSummary) bool { return r.Success >= 0.98 && r.CostPerHour < 1.0 },
	},
		Candidate{Label: "One big server", Detail: "A single app server with 64 threads, no autoscaling.", diagram: tweak(rush, engine.KAppServer, P{"threads": 64.0})},
		Candidate{Label: "Autoscale from one server", Detail: "Start with one server; add more when threads get busy.", diagram: tweak(rush, engine.KAppServer, P{"autoscale": true})},
		Candidate{Label: "Autoscale, at least three servers", Detail: "Never fewer than three; add more when threads get busy.", diagram: tweak(rush, engine.KAppServer, P{"autoscale": true, "minInstances": 3.0})},
	)
}

func (c *catalog) registerReview(id string, d engine.Diagram, q *Question, defects ...Defect) {
	q.ID, q.Mode, q.Diagram, q.defects = id, ModeReview, &d, defects
	c.registerQuestion(q)
}

// reviewShape is the layout the review designs share: users, a balancer, two servers, a cache, a database and a replica.
func reviewShape(users, app, cache, db P, replica P) engine.Diagram {
	return (&bld{}).
		n("users", engine.KClient, cols[0], rowMid, users).
		n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
		n("app1", engine.KAppServer, cols[2], rowTop, app).
		n("app2", engine.KAppServer, cols[2], rowBot, app).
		n("cache", engine.KCache, cols[3], rowTop, cache).
		n("db", engine.KDatabase, cols[4], rowMid, db).
		n("rep", engine.KReplica, cols[4], rowBot, replica).
		e("users", "lb").e("lb", "app1").e("lb", "app2").
		e("app1", "cache").e("app2", "cache").e("cache", "db").e("app1", "db").e("app2", "db").e("app1", "rep").e("app2", "rep").
		done()
}

func (c *catalog) registerReviewQuestions() {
	// A web shop with four flaws, one on each of four components.
	c.registerReview("review-web-shop",
		reviewShape(
			P{"rps": 120.0, "timeoutMs": 800.0, "retries": 4.0, "backoff": "none", "readPct": 90.0},
			P{"threads": 8.0, "queueCap": 400.0, "callTimeoutMs": 400.0, "dbPool": 100.0},
			P{"ttlSec": 30.0, "ttlJitterPct": 0.0, "startWarm": true},
			P{"maxConnections": 150.0}, nil),
		&Question{
			Title: "Review: a web shop", Difficulty: "Medium",
			Summary: "Four things will bite this design. Find them.",
			Prompt:  "A shop expects traffic to spike at its next sale. Mark the components you would change before it ships; mark only what you are sure is wrong. You score a point for each flaw you find and lose one for each component you mark that is fine.",
		},
		Defect{ID: "retries", Nodes: []string{"users"}, Rules: []string{"retries-without-backoff"}, Explain: "Clients retry up to four times with no backoff. Retries arrive exactly when the system is struggling, multiplying the load. Use exponential backoff with jitter and fewer retries."},
		Defect{ID: "queues", Nodes: []string{"app1", "app2"}, Rules: []string{"retry-amplification", "queue-longer-than-patience"}, Explain: "Each server will queue up to 400 requests with eight threads, about a second of work, longer than users wait (800 ms). Queued requests go stale, users retry, and the queue never drains. Shorten the queue or set a maximum queue wait."},
		Defect{ID: "expiry", Nodes: []string{"cache"}, Rules: []string{"synchronised-expiry"}, Explain: "The cache starts full and every entry lives exactly 30 seconds, so they all expire together and the database takes every miss at once. Add TTL jitter."},
		Defect{ID: "pools", Nodes: []string{"db"}, Rules: []string{"pool-exceeds-db"}, Explain: "Two servers may each open 100 connections, 200 in total, but the database accepts 150, so under load connections are refused. Shrink the pools or raise the limit."},
	)

	// A booking service with three flaws.
	c.registerReview("review-booking",
		reviewShape(
			P{"rps": 60.0, "readPct": 70.0, "consistency": "strong", "retries": 3.0, "backoff": "none"},
			P{"threads": 16.0, "callTimeoutMs": 400.0},
			P{"ttlSec": 0.0, "writePolicy": "none"},
			P{"onFailure": "promote", "fencing": false}, P{"replication": "sync"}),
		&Question{
			Title: "Review: a booking service", Difficulty: "Medium",
			Summary: "Customers need fresh data and no double bookings. What undermines that?",
			Prompt:  "Customers insist on seeing current availability, and the database is allowed to fail over to its replica. Mark the components you would change; mark only what you are sure is wrong.",
		},
		Defect{ID: "retries", Nodes: []string{"users"}, Rules: []string{"retries-without-backoff"}, Explain: "Three immediate retries per failure add load at the worst moment. Back off exponentially with jitter."},
		Defect{ID: "stale-cache", Nodes: []string{"cache"}, Rules: []string{"cache-no-ttl-with-writes"}, Explain: "Readers demand fresh data and writes happen, but this cache never expires entries and does nothing when a write arrives, so it serves old availability forever. Invalidate or write through on writes, or give entries a lifetime."},
		Defect{ID: "split-brain", Nodes: []string{"db"}, Rules: []string{"unfenced-failover"}, Explain: "After a failover the old primary can come back and accept writes next to the new one (split brain): two people can book the same seat. Fence the old primary."},
	)

	// A search service with three flaws.
	c.registerReview("review-search",
		reviewShape(
			P{"rps": 200.0, "timeoutMs": 1000.0},
			P{"threads": 16.0, "circuitBreaker": true, "dbPool": 120.0},
			P{"ttlSec": 20.0, "ttlJitterPct": 0.0, "startWarm": true},
			P{"maxConnections": 200.0}, nil),
		&Question{
			Title: "Review: a search service", Difficulty: "Hard",
			Summary: "It has a circuit breaker, so it must be safe. Is it?",
			Prompt:  "This design was reviewed last year and given a tick for resilience. Find what is still wrong; mark only what you are sure of.",
		},
		Defect{ID: "dead-breaker", Nodes: []string{"app1", "app2"}, Rules: []string{"breaker-without-timeout", "no-call-timeout"}, Explain: "The servers have a circuit breaker but no call timeout. A breaker trips on failures, and a call that never times out never fails, so it can never trip: threads simply wait forever on a silent dependency."},
		Defect{ID: "expiry", Nodes: []string{"cache"}, Rules: []string{"synchronised-expiry"}, Explain: "A warm cache with one fixed 20-second lifetime expires all at once; the database gets every miss together. Add jitter."},
		Defect{ID: "pools", Nodes: []string{"db"}, Rules: []string{"pool-exceeds-db"}, Explain: "Two servers with 120 connections each is 240, but the database accepts 200."},
	)
}

// tweak returns a copy of d with params merged onto every component of a kind (or with the given id).
func tweak(d engine.Diagram, match string, p P) engine.Diagram {
	out := d
	out.Nodes = append([]engine.Node(nil), d.Nodes...)
	for i, n := range out.Nodes {
		if n.Kind != match && n.ID != match {
			continue
		}
		np := P{}
		for k, v := range n.Params {
			np[k] = v
		}
		for k, v := range p {
			np[k] = v
		}
		out.Nodes[i].Params = np
	}
	d2, _ := engine.Sanitize(out)
	return d2
}

func (c *catalog) registerTradeoff(id, scenarioID string, q *Question, cands ...Candidate) {
	q.ID, q.Mode, q.scenarioID, q.Candidates = id, ModeTradeoff, scenarioID, cands
	c.registerQuestion(q)
}

func (c *catalog) registerDiagnose(id string, src diagSource, q *Question) {
	q.ID, q.Mode, q.source = id, ModeDiagnose, src
	q.Symptoms = loadSymptoms(id)
	c.registerQuestion(q)
}
