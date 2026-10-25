package scenarios

import (
	"fmt"

	"sysdesign/internal/engine"
)

// registerResilienceAndCapstones registers reconnect storms, bad deploys, zone outages, retry storms, bots, uploads and the capstones.
func registerResilienceAndCapstones() {
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
		Story:   "A chat server holds 1,500 live connections. It restarts for a few seconds, which is fine, but when it returns {{thundering-herd|every client reconnects in the same instant}}. Setting up a connection costs real work, so the server cannot keep up, the clients give up and try again together, and it never catches its breath.",
		Diagram: reconnect(false), Solution: reconnect(true),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "app", AtSec: 15, DownSec: 3}},
		Goals:       engine.Goals{MinSuccessRate: 0.95, MaxP95Ms: f(400), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.1), StarP95Ms: f(200)},
		DurationSec: 60, WarmupSec: 10, Speed: 1,
		Hints: [3]string{
			"Watch the new-connections rate on the app server after the restart. How big is the spike, and how many connections can it set up per second?",
			"{{littles-law|16 threads at 30 ms a handshake is about 500 connections a second}}, and 1,500 clients arrive in one go. Those that time out try again at the same moment.",
			"All the clients return at the same moment. What could each client do so that they do not, and where is that set?",
		},
		Answer: "The answer: spread the reconnects: give clients a random wait before they come back (the reconnect spread on the client). More servers would also work but cost more.",
		Predictions: []Prediction{{ID: "p1", Question: "Spreading reconnects over 30 seconds will…", Options: []string{"Make the outage 30 seconds longer for everyone", "Turn one huge spike into a steady trickle", "Do nothing, as the same clients reconnect"}, Answer: 1,
			Explain: "The same number of connections are set up, but the peak rate falls from the whole population at once to about fifty a second."}},
		Debrief: "A restart is not the end of the incident: {{retry-storm|every client retrying in lockstep}} can keep the server down. Add random jitter (and backoff) to reconnects so the {{thundering-herd|herd}} arrives as a stream.",
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
			"Which load balancer setting sends only a small share of traffic to the new version, and where is it?",
		},
		Answer: "The answer: give the load balancer a canary share (advanced settings): a small percentage of requests goes to version 2 until you trust it.",
		Predictions: []Prediction{{ID: "p1", Question: "With a 5% canary, roughly how many requests meet the bug?", Options: []string{"About half, as the canary is checked first", "About 5% of them, during the bad period", "Almost none, as a canary is always tested"}, Answer: 1,
			Explain: "A canary sends the new version only its small share, so a broken release hurts only that share until it is rolled back."}},
		Debrief: "Release safety is about {{bulkhead|blast radius}}. Rolling to half the fleet at once exposes half your users to every bug; a canary exposes a few, lets you watch the numbers, and only then widens the rollout.",
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
			"For each component on the path, which zone does it sit in? Which single zone failure would still cut the path?",
		},
		Answer: "The answer: set each component's availability zone (advanced settings): keep the balancer and the database out of zone A and spread the servers over different zones.",
		Predictions: []Prediction{{ID: "p1", Question: "Why does the design survive when the balancer is moved to another zone but the servers stay put?", Options: []string{"It does not: all three servers are still in the failed zone", "The balancer caches responses from the servers", "The database takes over when the zone fails"}, Answer: 0,
			Explain: "A balancer in a healthy zone has nothing to send traffic to if every server it knows is in the failed zone. All the pieces a request needs must survive."}},
		Debrief: "{{bulkhead|Failure domains}} are only useful if you actually use them: spread replicas across zones, and put the single points (balancer entry, primary database) where a zone failure will not take them with it. Cross-zone calls cost about a millisecond or two, which is cheap insurance.",
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
		Story:   "The database slows down for five seconds. Each client gives up after 400 ms and tries again, up to three more times, immediately. By the time the database is healthy again it is {{retry-storm|buried under the retries}} of requests that timed out, which time out in turn and are retried again. {{metastable-failure|Fifty seconds later it is still down}}.",
		Diagram: retryStorm(false), Solution: retryStorm(true),
		Chaos:       []engine.ChaosEvent{{Type: "slow", NodeID: "db", AtSec: 10, Factor: 3, DurationSec: 5}},
		Goals:       engine.Goals{MinSuccessRate: 0.7, MaxP95Ms: f(400), MaxCostPerHour: f(2.7), StarCostPerHour: f(2.5), StarP95Ms: f(300)},
		DurationSec: 60, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at the success rate over time: it drops during the blip and then stays down long after the database is healthy. What is still arriving?",
			"Every timeout becomes up to three more requests, so the load is several times what the users really send. The database cannot ever catch up.",
			"What limits the total extra load that retries can add, whatever the delays? Look at the users' retry settings.",
		},
		Answer: "The answer: limit the damage retries can do: give the users a retry budget (advanced settings), or reduce the retries. A retry budget keeps retrying cheap failures but stops {{retry-storm|a storm}}.",
		Predictions: []Prediction{{ID: "p1", Question: "Why does adding exponential backoff to the retries not fix it here?", Options: []string{"It does: backoff always prevents retry storms", "The load stays several times the real traffic", "Backoff makes the database slower to answer"}, Answer: 1,
			Explain: "Backoff spreads retries out but does not reduce how many there are. A cap on the total (a budget) is what keeps the extra load bounded."}},
		Debrief: "{{retry-storm|Retries multiply load exactly when a dependency is weakest}}. Keep them bounded: a retry budget as a share of normal traffic, backoff with jitter, and a limit on attempts. Without that, a short blip turns into a long outage.",
		Deeper:  "{{retry-storm|Retries multiply load exactly when the dependency is weakest}}, and the multiplication compounds across layers: if three layers each try an operation four times, one user action can become sixty-four calls at the bottom. The state is {{metastable-failure|metastable}}, meaning the system stays broken after the original trigger is gone, as in this story where it is still down fifty seconds after a five-second slowdown. The remedies work together: a retry budget as a share of normal traffic, backoff with jitter so retries do not arrive in step, retries at one layer only, deadlines that travel with the request, and {{load-shedding|shedding load}} so the queue does not hold dead requests. The cost is that a budget gives up on some requests that would have succeeded. An interviewer will ask which layer should retry and why the operation must be {{idempotency|idempotent}}.",
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
		Story:   "A scraper asks for item after item nobody has cached, at 600 requests a second from just three accounts, on top of 1,000 customers who send 100 a second between them. The database is saturated. Someone put {{load-shedding|a rate limiter}} in front of everything, but it counts all traffic together, so customers are rejected right along with the bot. Only the customers' experience counts here.",
		Diagram: botFlood(false), Solution: botFlood(true),
		Goals:       engine.Goals{MinCustomerSuccess: f(0.97), MaxCostPerHour: f(2.8), StarCostPerHour: f(2.6), StarP95Ms: f(300)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Compare how the customers fare with how the scraper fares. The limiter sees only a stream of requests: what does it count?",
			"The scraper sends 600 requests a second from three accounts; a customer sends about one every ten seconds. One shared allowance cannot tell them apart.",
			"The limiter counts all traffic together. What could it count by instead, so that the scraper runs out and the customers do not?",
		},
		Answer: "The answer: make the limiter count per user (advanced settings) and set the allowance to what a customer plausibly needs, a few requests a second.",
		Predictions: []Prediction{{ID: "p1", Question: "Why does raising the shared limit not fix it?", Options: []string{"The customers become slower because limits add delay", "The bot's extra requests also get through to the database", "The bot just opens more accounts to get around it"}, Answer: 1,
			Explain: "A shared limit that is high enough for the customers lets most of the bot through too. The two have to be told apart by who they are."}},
		Debrief: "{{load-shedding|A global limit}} protects the system but not the people. Limit per identity so a heavy or abusive account runs out of allowance on its own, and size the per-user rate to legitimate use rather than to total capacity.",
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
			"Where could a large body live while only a small reference travels through the queue, and which component would you add for it?",
		},
		Answer: "The answer: add an Object Store and connect the app server to it. The app uploads the body there and queues only a pointer (the claim-check pattern).",
		Predictions: []Prediction{{ID: "p1", Question: "A queue accepts messages up to 256 KB. Users upload 500 KB files. What is the standard fix?", Options: []string{"Raise the queue's size limit past 500 KB", "Retry the failed uploads in smaller pieces", "Store the file elsewhere and queue a pointer to it"}, Answer: 2,
			Explain: "Queues and databases are built for small records. Large bodies go to an object store; what travels through the queue is a reference to it."}},
		Debrief: "The claim-check pattern keeps large payloads out of the messaging path: the body is uploaded once to cheap, durable object storage and everyone downstream handles a small pointer. It also makes {{retry-storm|retries cheap}}, because a retry re-sends a pointer, not 500 KB.",
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
			"Which component sits closest to the users and can answer without touching your servers, and where in the path does it belong?",
		},
		Answer: "The answer: put a CDN between the users and the load balancer.",
		Predictions: []Prediction{{ID: "p1", Question: "A CDN with a 60 second TTL serves a page that changes every few seconds. What is the catch?", Options: []string{"Users may see stale copies until the TTL expires", "The CDN cannot serve pages that change at all", "Every request still has to reach the origin"}, Answer: 0,
			Explain: "Cached copies are served until they expire, so freshness is traded for speed unless writes invalidate them."}},
		Debrief: "The cheapest request is one your servers never see. A CDN absorbs repeated reads at the edge; the price is {{ttl|freshness}}, which is why caching suits content that changes slowly.",
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
			"The balancer can send new work to whichever server has {{littles-law|the fewest requests in flight}}, or stop using a server that misbehaves.",
			"Which load balancer rule reacts to how many requests a server has in flight, and which setting takes a misbehaving server out of rotation?",
		},
		Answer: "The answer: switch the load balancer to least connections, or turn on outlier ejection.",
		Predictions: []Prediction{{ID: "p1", Question: "Why does a least-connections balancer help when one server is slow?", Options: []string{"A slow server accumulates open requests, so it is picked less", "It makes the slow server faster by easing its load", "It restarts the slow server automatically"}, Answer: 0,
			Explain: "Requests pile up on the slow server, so its in-flight count is high and new requests go elsewhere."}},
		Debrief: "Balancing is not about equal shares, it is about equal outcomes. Least connections adapts to slowness automatically; {{circuit-breaker|ejecting outliers}} goes further and rests a sick server entirely.",
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
		Story:   "A busy write-heavy service has outgrown its single database. Buying a bigger one is not an option, so the data has to be {{sharding|spread over several databases}}.",
		Diagram: sharded(1), Solution: sharded(2),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(500), MaxCostPerHour: f(2.5)},
		DurationSec: 45, WarmupSec: 6, Speed: 1,
		Hints: [3]string{
			"Count the writes per second and divide by what one database can commit.",
			"{{sharding|Shard the data}}: a router sends each key to one of several databases.",
			"What component picks a database from a key, and what must the servers be wired to instead of the database itself?",
		},
		Answer: "The answer: put a {{sharding|shard router}} between the servers and two databases, and connect each server to the router.",
		Predictions: []Prediction{{ID: "p1", Question: "A shard router picks the database from the item's key. What goes wrong if a few keys are far hotter than the rest?", Options: []string{"One shard can get most of the traffic", "Nothing: sharding balances load perfectly", "Writes slow down because they become reads"}, Answer: 0,
			Explain: "Hashing spreads keys, not requests. If a handful of keys dominate, whichever shard owns them becomes the bottleneck again."}},
		Debrief: "{{sharding|Sharding}} multiplies write capacity by splitting the data, at the price of complexity: queries across shards get hard and skew can recreate the {{hot-key|hotspot}} you were escaping.",
		Deeper:  "{{sharding|Sharding}} multiplies write capacity by splitting the data, but you pay in operations: the shard key decides which queries stay on one shard and which fan out, cross-shard transactions and joins become difficult, and {{hot-key|a key that is hot}} or a tenant that is large recreates the hot spot you were escaping. Hashing spreads keys evenly and loses range scans, ranges keep scans and make monotonic keys hot, and a {{consistent-hashing|consistent-hashing ring}} or fixed virtual shards makes adding capacity move only a fraction of the data. Resharding a live system needs copying, double writes and a cut-over, so choose more logical shards than machines at the start. An interviewer will ask why you did not first try read replicas, a bigger machine or a different data model, and how you generate unique ids across shards. The failure pattern is a shard key that looks even in testing and skews in production.",
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
		Story:   "Traffic climbs from 100 to 800 requests per second. Midway the database slows down for a while, and shortly after a server crashes. {{retry-storm|Clients retry}}, as clients do. Design a system that keeps most users happy through all of it.",
		Diagram: capstone(false), Solution: capstone(true),
		Traffic:     map[string][]engine.TrafficPoint{"users": {{0, 100}, {10, 100}, {40, 800}, {75, 800}, {85, 150}, {100, 150}}},
		Chaos:       []engine.ChaosEvent{{Type: "slow", NodeID: "db", AtSec: 45, Factor: 4, DurationSec: 12}, {Type: "crash", NodeID: "random:appServer", AtSec: 60, DownSec: 15}},
		Goals:       engine.Goals{MinSuccessRate: 0.93, MaxP95Ms: f(700), MaxCostPerHour: f(1.8)},
		DurationSec: 100, WarmupSec: 8, Speed: 2,
		Hints: [3]string{
			"Three things go wrong at different times: load, a slow dependency and a lost server. Which parts of your design handle each?",
			"A cache takes load off the database, spare servers survive a crash, and timeouts with a short queue stop a slow dependency from tying up {{thread-pool|every thread}}.",
			"Tick off what the story demands: a cache for the reads, spare servers for a crash, and protection from a slow database. Which component or setting covers each?",
		},
		Answer: "The answer: a load balancer, five servers with call timeouts and a short newest-first queue, and a cache in front of the database.",
		Predictions: []Prediction{{ID: "p1", Question: "The database becomes four times slower for twelve seconds. Which setting stops the slowdown from taking the whole system down?", Options: []string{"A call timeout on the app servers", "More threads on each app server", "Longer timeouts on the clients"}, Answer: 0,
			Explain: "A timeout frees the thread instead of leaving it stuck behind the slow database, so servers keep answering what they can (from cache, for example)."}},
		Debrief: "Real outages stack: capacity, a slow dependency and a failure arrive together. The designs that survive layer defences: caches to cut load, redundancy for failures, and timeouts and short queues so slowness cannot spread.",
		Deeper:  "Each defence handles one failure and introduces a cost or a dependency: a cache cuts load but returns cold after a restart, spare servers cost money all day, timeouts convert a stall into errors that you must handle, and a short newest-first queue discards the oldest requests, which are the ones whose callers probably left, at the price of fairness. {{retry-storm|Client retries turn a brief slowdown into more load}}, so backoff matters as much as server settings. At larger scale you load test the combination and not each piece, since the interesting failures appear when capacity, a slow dependency and a crash arrive together. An interviewer will ask you to walk through the timeline of this outage and say what each layer does at each moment, and what you would alert on. The failure pattern is a cascade, where the first slow call holds threads, the retries add load, and the crash removes the headroom you were counting on.",
	})
}
