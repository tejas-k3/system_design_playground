package scenarios

import (
	"fmt"

	"sysdesign/internal/engine"
)

// These SDE2 open briefs expose requirements but keep each workload and its failure profile server-side.
func (c *catalog) registerOpenM1Briefs() {
	open := func(users P) engine.Diagram {
		return (&bld{}).n("users", engine.KClient, cols[0], rowMid, users).done()
	}
	shortener := func() engine.Diagram {
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 95.0, "keySpace": 30000.0, "keyDist": "zipf", "timeoutMs": 1200.0, "retries": 0.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).e("users", "lb")
		b.n("cache", engine.KCache, cols[3], rowTop, P{"ttlSec": 60.0, "ttlJitterPct": 20.0, "capacityItems": 4000.0})
		b.n("db", engine.KDatabase, cols[4], rowMid, P{"connections": 24.0})
		for i := range 3 {
			id := fmt.Sprintf("app%d", i+1)
			b.n(id, engine.KAppServer, cols[2], rowTop+float64(i)*70, P{"threads": 16.0, "callTimeoutMs": 400.0, "circuitBreaker": true}).e("lb", id).e(id, "cache").e(id, "db")
		}
		return b.done()
	}
	c.register(&Scenario{
		ID: "open-link-shortener", Title: "Design a link shortener", Difficulty: "Medium", Open: true,
		Summary: "Build a fast redirect service that keeps links durable and stays inside its budget.",
		Story:   "Build a link shortener for a product launch. At least 98% of requests must succeed, p95 must stay below 250 ms, and the service must cost no more than $1.60 an hour. A server may fail during the launch, and popular links must still redirect quickly. Start with the users and choose the rest of the design.",
		Diagram: open(P{"rps": 100.0, "readPct": 95.0, "keySpace": 30000.0, "keyDist": "zipf", "timeoutMs": 1200.0}), Solution: shortener(),
		Traffic: map[string][]engine.TrafficPoint{"users": {{0, 100}, {10, 100}, {20, 420}, {45, 420}, {55, 100}, {65, 100}}},
		Chaos:   []engine.ChaosEvent{{Type: "crash", NodeID: "random:appServer", AtSec: 30, DownSec: 12}},
		Goals:   engine.Goals{MinSuccessRate: 0.98, MaxP95Ms: f(250), MaxCostPerHour: f(1.6)}, DurationSec: 65, WarmupSec: 5,
		Hints: [3]string{
			"Most requests are redirects. Which part of the path handles each lookup, and where could repeat reads be answered?",
			"The identifier space is uneven: a few links are much more popular than the rest. A cache can reduce repeated reads, but it cannot be the only copy of a link.",
			"If one app server disappears during the launch, what remains on the path and where is the durable mapping kept?",
		},
		Answer:     "The answer: One workable design uses a balancer, several app servers, a cache for repeated redirects and a durable database for the mappings. Size the servers for the rush with one unavailable, then check the measured p95 and cost.",
		Debrief:    "The redirect path is mostly reads, so a good {{cache-hit-rate|cache hit rate}} can protect the database. The mapping still needs durable storage, and the app fleet needs enough spare capacity to serve traffic after a server fails.",
		Deeper:     "A short link is a key lookup: the read path can use a cache, while link creation and expiry still need a durable source of truth. A skewed popularity distribution makes the hottest links important, and a short TTL trades freshness against database load. Size for the peak after losing one server, then check the p95 and cost on a run. This model does not simulate custom alias collisions or expiration cleanup policy.",
		WeakPoints: []string{"The database is still one durable copy.", "A cold cache can send a sudden read burst to the database.", "Custom aliases and expiry cleanup need separate product rules."},
	})

	notifications := func(workers int, deadLetter bool) engine.Diagram {
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 60.0, "readPct": 0.0, "timeoutMs": 800.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0, "queueCap": 200.0}).
			n("events", engine.KQueue, cols[2], rowMid, P{"capacity": 3000.0, "maxAttempts": 4.0, "deadLetter": deadLetter})
		b.e("users", "app").e("app", "events")
		for i := range workers {
			id := fmt.Sprintf("sender%d", i+1)
			b.n(id, engine.KWorker, cols[3], rowTop+float64(i)*70, P{"concurrency": 8.0, "processMs": 80.0, "poisonPct": 4.0}).e("events", id)
		}
		b.n("provider", engine.KExternalAPI, cols[4], rowMid, P{"latencyMs": 50.0, "errorPct": 0.0})
		b.n("db", engine.KDatabase, cols[5], rowMid, P{"connections": 16.0})
		for i := range workers {
			b.e(fmt.Sprintf("sender%d", i+1), "provider").e(fmt.Sprintf("sender%d", i+1), "db")
		}
		return b.done()
	}
	c.register(&Scenario{
		ID: "open-notifications", Title: "Design notifications", Difficulty: "Medium", Open: true,
		Summary: "Accept notification requests quickly and deliver work despite a provider slowdown.",
		Story:   "A service sends email and push notifications after user actions. At least 98% of requests must be accepted, p95 acknowledgement latency must stay below 200 ms, at least 94% of accepted jobs must finish, no job may disappear without a recorded disposition, and the system must cost no more than $1.40 an hour. A delivery provider slows down during a traffic rush. Build the service from the users outward.",
		Diagram: open(P{"rps": 60.0, "readPct": 0.0, "timeoutMs": 800.0}), Solution: notifications(2, true),
		Traffic: map[string][]engine.TrafficPoint{"users": {{0, 60}, {10, 60}, {20, 140}, {40, 140}, {50, 60}, {60, 60}}},
		Chaos:   []engine.ChaosEvent{{Type: "slow", NodeID: "provider", AtSec: 25, Factor: 8, DurationSec: 12}},
		Goals:   engine.Goals{MinSuccessRate: 0.98, MaxP95Ms: f(200), MaxCostPerHour: f(1.4), MinJobsCompleted: f(0.94), MaxJobsLost: i(0)}, DurationSec: 60, WarmupSec: 5,
		Hints: [3]string{
			"The acknowledgement deadline is much shorter than a slow provider call. Which work must finish before the request can be acknowledged?",
			"A backlog can absorb a temporary slowdown, but workers need enough capacity to drain it later. Some malformed jobs may never succeed.",
			"Where can failed jobs wait before retrying, and what should happen after their final attempt?",
		},
		Answer:     "The answer: A workable design acknowledges after durable enqueue, uses enough workers to drain the backlog, and gives repeatedly failing jobs a dead-letter disposition. Check both request success and job completion after the provider slowdown.",
		Debrief:    "A queue separates fast request acknowledgement from slower delivery. It does not make work disappear: size the workers to drain normal load and send jobs that exhaust their retries to a {{dead-letter-queue|dead-letter queue}} for review.",
		Deeper:     "Queueing protects the request path from a slow provider, but it moves the wait into a backlog. A retry policy can recover transient failures; poison jobs need a dead-letter path so the same bad payload does not consume worker capacity forever. This run models acceptance and delivery counts, not provider-specific rate limits or email inbox placement.",
		WeakPoints: []string{"A large backlog can make notifications arrive late.", "A dead-lettered job still needs an operator or repair path.", "The provider can impose limits not represented in this run."},
	})

	feed := func() engine.Diagram {
		return (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 40.0, "readPct": 70.0, "workload": "feed", "keySpace": 5000.0, "followersMean": 50.0, "followersSkew": 1.2, "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "feedStrategy": "hybrid", "celebrityThreshold": 2000.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "queueCap": 500.0}).
			e("users", "app").e("app", "db").done()
	}
	c.register(&Scenario{
		ID: "open-news-feed", Title: "Design a news feed", Difficulty: "Medium", Open: true,
		Summary: "Balance feed write fan-out against the cost of building timelines on reads.",
		Story:   "Build a news feed for a growing community. At least 98% of requests must succeed, p95 must stay below 350 ms, and the system must cost no more than $1.50 an hour. Most authors have small audiences, but a few have far more followers. The service must keep serving ordinary readers without letting one large audience overwhelm the rest. Start with the users and choose the design.",
		Diagram: open(P{"rps": 40.0, "readPct": 70.0, "workload": "feed", "keySpace": 5000.0, "followersMean": 50.0, "followersSkew": 1.2}), Solution: feed(),
		Traffic: map[string][]engine.TrafficPoint{"users": {{0, 40}, {10, 40}, {20, 90}, {40, 90}, {50, 40}, {60, 40}}},
		Chaos:   []engine.ChaosEvent{{Type: "slow", NodeID: "db", AtSec: 30, Factor: 3, DurationSec: 8}},
		Goals:   engine.Goals{MinSuccessRate: 0.98, MaxP95Ms: f(350), StarP95Ms: f(280), MaxCostPerHour: f(1.5)}, DurationSec: 60, WarmupSec: 5,
		Hints: [3]string{
			"Compare the work caused by a post with the work caused by a timeline read. Which depends on the author's audience size?",
			"Pushing every post to every follower makes reads cheap, but a celebrity post can create a large write burst. Pulling everything reverses that cost.",
			"Can the design use one strategy for small audiences and another for very large ones? Which workload metric would show the trade-off?",
		},
		Answer:     "The answer: A workable feed uses push for ordinary authors and pulls large-audience posts when a reader builds a timeline. Compare fan-out writes and reads, then verify the latency and cost goals.",
		Debrief:    "Push makes timeline reads cheap but makes a post's write cost grow with its audience, so an author with a very large audience behaves like a {{hot-key|hot key}}. Pull makes writes cheap but moves work to each read. A hybrid uses different paths for each audience size.",
		Deeper:     "Fan-out on write pays once per follower and can make celebrity posts create huge bursts. Fan-out on read pays once per reader and can make timelines slower. A hybrid threshold is a trade-off, not a universal constant; change it and compare the measured fan-out counters. This model does not include ranking quality or freshness guarantees across all followers.",
		WeakPoints: []string{"The threshold can become wrong as follower counts change.", "A timeline read may still be slow when it pulls many authors.", "Ranking quality and stale posts are outside this simulation."},
	})

	nearby := func() engine.Diagram {
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 85.0, "workload": "geo", "keySpace": 5000.0, "hotKeyPct": 40.0, "timeoutMs": 1000.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).e("users", "lb").
			n("app1", engine.KAppServer, cols[2], rowTop, P{"threads": 16.0}).e("lb", "app1").
			n("app2", engine.KAppServer, cols[2], rowBot, P{"threads": 16.0}).e("lb", "app2").
			n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 32.0, "spatialIndex": true})
		b.e("app1", "db").e("app2", "db")
		return b.done()
	}
	c.register(&Scenario{
		ID: "open-nearby-search", Title: "Design nearby search", Difficulty: "Medium", Open: true,
		Summary: "Serve location searches quickly when demand concentrates in one busy area.",
		Story:   "A local service helps people find nearby couriers. At least 98% of searches must succeed, p95 must stay below 250 ms, and the system must cost no more than $1.50 an hour. Couriers send frequent location updates, and demand is concentrated in a busy city centre. Searches in the busy centre must stay fast while couriers keep sending updates. Build from the users outward.",
		Diagram: open(P{"rps": 100.0, "readPct": 85.0, "workload": "geo", "keySpace": 5000.0, "hotKeyPct": 40.0, "timeoutMs": 1000.0}), Solution: nearby(),
		Traffic: map[string][]engine.TrafficPoint{"users": {{0, 100}, {10, 100}, {20, 220}, {40, 220}, {50, 100}, {60, 100}}},
		Chaos:   []engine.ChaosEvent{{Type: "slow", NodeID: "db", AtSec: 30, Factor: 3, DurationSec: 8}},
		Goals:   engine.Goals{MinSuccessRate: 0.98, MaxP95Ms: f(250), MaxCostPerHour: f(1.5)}, DurationSec: 60, WarmupSec: 5,
		Hints: [3]string{
			"A nearby search is different from an exact key lookup. What work does the database do for each search?",
			"A spatial index can narrow the search to nearby locations, while frequent updates still have to keep the index current.",
			"Which database capability can avoid scanning every location, and what hot area would still need attention?",
		},
		Answer:     "The answer: Use an indexed location search behind enough app capacity to handle the peak. Check how a busy area and a slow database affect the run before buying more servers.",
		Debrief:    "A spatial index reduces the work for a nearby query. The busiest area can still concentrate updates, so measure that load and keep app capacity after a server failure.",
		Deeper:     "Location queries need a spatial index or a geographic partitioning scheme; an ordinary full-table scan scales with the whole data set. A hot area can remain a bottleneck even when the rest of the map is quiet, and moving locations make index writes part of the capacity calculation. This simulator models a spatial index and skewed location keys, not GPS accuracy or route distance.",
		WeakPoints: []string{"A dense area can remain a hot spot.", "Frequent location updates compete with searches.", "Distance and GPS accuracy are not modelled."},
	})

	chat := func(apps int, jitter float64) engine.Diagram {
		b := (&bld{}).n("users", engine.KClient, cols[0], rowMid, P{"arrival": "sessions", "connections": 900.0, "sessionSec": 900.0, "msgsPerMin": 6.0, "reconnectJitterSec": jitter, "readPct": 50.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).e("users", "lb")
		b.n("db", engine.KDatabase, cols[4], rowMid, P{"connections": 24.0})
		for i := range apps {
			id := fmt.Sprintf("app%d", i+1)
			b.n(id, engine.KAppServer, cols[2], rowTop+float64(i)*70, P{"threads": 16.0, "maxConns": 350.0, "handshakeMs": 20.0}).e("lb", id).e(id, "db")
		}
		return b.done()
	}
	c.register(&Scenario{
		ID: "open-chat", Title: "Design chat", Difficulty: "Medium", Open: true,
		Summary: "Keep chat connections available through a server restart and reconnect surge.",
		Story:   "Build a chat service for 900 connected users. At least 97% of requests must succeed, p95 must stay below 500 ms, and the service must cost no more than $1.60 an hour. When an app server restarts, disconnected users should reconnect without overwhelming the remaining servers. Build from the users outward.",
		Diagram: open(P{"arrival": "sessions", "connections": 900.0, "sessionSec": 900.0, "msgsPerMin": 6.0, "reconnectJitterSec": 5.0, "readPct": 50.0, "timeoutMs": 2000.0}), Solution: chat(4, 30),
		Traffic: map[string][]engine.TrafficPoint{"users": {{0, 20}, {10, 20}, {20, 40}, {40, 40}, {50, 20}, {60, 20}}},
		Chaos:   []engine.ChaosEvent{{Type: "crash", NodeID: "random:appServer", AtSec: 25, DownSec: 5}},
		Goals:   engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(500), MaxCostPerHour: f(1.6)}, DurationSec: 60, WarmupSec: 5,
		Hints: [3]string{
			"Long-lived connections consume capacity even between messages. How many connections must the service hold?",
			"A restart drops every connection on that server. A retry wave can hit the remaining servers together unless clients spread their reconnects.",
			"How many connections can each surviving app server hold, and how can reconnect attempts arrive over time instead of all at once?",
		},
		Answer:     "The answer: A workable design spreads connections across several app servers, gives the fleet room after one restart, and spreads reconnect attempts with jitter.",
		Debrief:    "A chat server spends capacity on long-lived connections as well as messages. When a server restarts, its connections return as new arrivals. Capacity for the remaining servers and a spread-out reconnect delay prevent one restart from creating a second overload.",
		Deeper:     "Long-lived connections consume per-server slots, and a crash can make many clients reconnect together. Add headroom for the remaining fleet and jitter the reconnect schedule. This open brief tests connection handling and reconnect load; message ordering and presence fan-out need separate modeled events and metrics.",
		WeakPoints: []string{"This run does not check per-conversation message ordering.", "Presence fan-out and stale presence are not modelled here.", "A reconnecting client may lose in-flight messages."},
	})
}
