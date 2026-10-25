package scenarios

import (
	"fmt"

	"sysdesign/internal/engine"
)

// registerFailureAndElasticity registers failover, partitions, gray failures, stampedes, fleet scaling, hot shards and scan pollution.
func registerFailureAndElasticity() {
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
		Story:       "Six in ten requests write to the database. At 20 seconds the primary crashes and stays down for a minute. There is a read replica, {{replication-lag|three seconds behind}}. Keep most requests succeeding and lose as few acknowledged writes as you can. The architecture is fixed: fix it with configuration.",
		Diagram:     failover(P{}, P{"replicationLagMs": 3000.0}),
		Solution:    failover(P{"onFailure": "promote", "detectSec": 3.0, "promoteSec": 5.0}, P{"replicationLagMs": 500.0}),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "db", AtSec: 20, DownSec: 60}},
		Goals:       engine.Goals{MinSuccessRate: 0.85, MaxP95Ms: f(800), MaxLostWrites: i(100)},
		DurationSec: 70, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"For a minute every write fails. Is there another machine that already has most of the data?",
			"The database can {{failover|promote a replica}} when it fails. Look at its 'If it fails' setting, and at how long it takes to notice and to switch.",
			"Three things set the outage and the loss: what the database does on failure, how quickly it notices and switches, and how far behind the replica is. Which have you not touched?",
		},
		Answer: "The answer: {{failover|promote a replica}}, shorten the detection and promotion times, and shrink the replica's {{replication-lag|replication lag}}: whatever it had not copied yet is lost.",
		Predictions: []Prediction{{ID: "p1", Question: "The replica is 3 seconds behind and gets promoted. What happens to the writes from the last 3 seconds before the crash?", Options: []string{"They are safe: the primary had saved them", "They are lost: the replica never received them", "They are replayed when the old primary returns"}, Answer: 1,
			Explain: "Asynchronous replication acknowledges a write before the replica has it, so a failover keeps only what had already been copied."}},
		Debrief: "{{failover|Failover}} has two costs. Time: while the failure is noticed and a replica takes over, nothing can write. Data: with asynchronous replication, whatever the replica had not yet copied is gone. Both are settings you choose: faster detection risks false alarms, and a smaller {{replication-lag|lag}} costs the primary more work.",
		Deeper:  "A {{failover|failover}} costs time and data, and they are separate dials: time is detection plus promotion, data is whatever the replica had not yet received when the primary died. Detecting faster risks promoting on a blip, which creates an outage that did not need to happen, and a smaller {{replication-lag|replication lag}} costs the primary more work or, if you make replication synchronous, makes every write wait for the replica. At larger scale you promote the most caught-up replica rather than any replica and let clients find the new primary through a service name instead of a cached address. An interviewer will ask what happens to the old primary when it returns, which is a later challenge in this set, and how you would reconcile writes it accepted that were never copied. The usual failure is promoting a lagging replica and then silently discarding the unreplicated tail.",
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
			"One server has {{thread-pool|a fixed number of threads}}. What if {{autoscaling|the number of servers could follow the traffic}}?",
			"Open the App Server's settings, choose 'Show advanced' and look at the Scaling group.",
			"A new server needs time to boot. Which setting makes the scaler order servers while the existing ones still have room?",
		},
		Answer: "The answer: turn on {{autoscaling|autoscaling}}, then lower the target busy level: new servers take 20 seconds to boot, so they must be ordered while the old ones still have room.",
		Predictions: []Prediction{{ID: "p1", Question: "A new server takes 20 seconds to boot and traffic climbs for a minute. When should the scaler start adding servers?", Options: []string{"As late as possible, to keep the monthly bill low", "Early, while servers still have spare room", "Only once requests have actually started failing"}, Answer: 1,
			Explain: "Capacity ordered now only arrives in 20 seconds. A scaler that waits until the servers are full has already started failing requests by the time help arrives."}},
		Debrief: "{{autoscaling|Autoscaling}} trades money for time. A low target busy level orders servers early and costs more; a high one is cheap and late. Boot time sets how far ahead you must look, and a wait before removing servers stops the fleet flapping. The very large fixed fleet works too, but costs all day what you only need at the peak.",
		Deeper:  "Reactive {{autoscaling|autoscaling}} always lags demand by the measuring window plus the boot time, which here is 20 seconds, so the target busy level is really a decision about how far ahead you pay for capacity: lower means earlier and dearer, higher means cheaper and late. When the wave is predictable, scheduled or predictive scaling and a pool of servers kept ready beat any threshold, and scaling on queue depth or latency reacts to the symptom that users feel rather than to CPU. The downstream side matters too, because every new server {{connection-pool|opens its own connections}}, so a fleet that grows quickly can overload the database that was fine a minute earlier. An interviewer will ask what the maximum should be and what happens when the cloud cannot supply the servers you ordered. The failure pattern is flapping: scaling in too eagerly removes the capacity the next wave needs.",
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
			"A crashed database refuses connections at once. A cut-off one never answers. What does a server {{thread-pool|thread}} do while it waits?",
			"Every waiting request holds a thread. Look at the app server's 'Call timeout': zero means wait forever.",
			"Which setting limits how long a call may hold a thread, and what would stop the servers trying at all while the database is unreachable?",
		},
		Answer: "The answer: give calls to the database a timeout of a few hundred milliseconds so threads come back, and turn on the {{circuit-breaker|circuit breaker}} so servers stop trying while the database is unreachable.",
		Predictions: []Prediction{{ID: "p1", Question: "A server calls a database that is cut off, and 'Call timeout' is 0 (wait forever). What happens to the server?", Options: []string{"Calls to the database fail, but every other request is answered normally", "All its threads get stuck waiting, so even cache hits go unanswered", "The load balancer sees the stuck calls and stops sending to it"}, Answer: 1,
			Explain: "Each waiting request holds a thread for good. Within a second or two none are left, and cache hits queue behind them."}},
		Debrief: "Silence is worse than an error. A crashed dependency refuses quickly, but a partitioned one makes callers wait, and waiting callers use up {{thread-pool|threads}} until the healthy parts of the system starve too. Timeouts turn silence into a fast failure, and a {{circuit-breaker|circuit breaker}} stops you spending a thread on every attempt.",
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
			"Two load balancer behaviours react to a server that is slow rather than dead. Look at both the health group and the balancing rule.",
		},
		Answer: "The answer: turn on 'Eject outliers' (servers much slower than their peers leave rotation for a while), or switch to least connections: a stalling server piles up unfinished requests, so it is given fewer new ones.",
		Predictions: []Prediction{{ID: "p1", Question: "A server is slow half the time but never fails a health check. Which balancing rule protects users best?", Options: []string{"Round robin, so that every server gets its fair share", "Dropping servers whose latency is far above their peers'", "Weighting traffic towards the slow server so it catches up"}, Answer: 1,
			Explain: "A slow server is worse than a dead one: a dead one is skipped, a slow one keeps being given work it cannot finish."}},
		Debrief: "Gray failures pass the checks that catch dead servers. Comparing each server with its peers (latency and errors) is what catches the ones that are merely sick, and least-connections balancing does it implicitly, because {{littles-law|a sick server is the one with the most unfinished work}}.",
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
		Story:       "You warmed the cache before launch, so the database sees almost nothing. Entries {{ttl|live ten seconds}}, because prices change and users must not see data older than about fifteen. At the first expiry {{stampede|the whole popular set misses at the same moment}} and four hundred reads a second land on a database that can answer eighty. Keep it standing without letting data go stale. The architecture is fixed: fix it with configuration.",
		Diagram:     stampede(P{"capacityItems": 2000.0, "ttlSec": 10.0, "startWarm": true}),
		Solution:    stampede(P{"capacityItems": 2000.0, "ttlSec": 10.0, "startWarm": true, "ttlJitterPct": 50.0, "coalesce": true}),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(500), MaxStaleReadPct: f(0.12)},
		DurationSec: 50, WarmupSec: 3, Speed: 1,
		Hints: [3]string{
			"Every entry was created at the same moment with the same lifetime. What happens when the lifetime ends?",
			"Open the cache's settings: look at Freshness and at 'Show advanced'. You need the entries to stop expiring together, and concurrent misses for one item to stop piling onto the database.",
			"Two problems, two settings: what makes every entry expire at the same moment, and what lets {{thundering-herd|many misses for one item all reach the database at once}}?",
		},
		Answer: "The answer: add TTL jitter so entries expire at different times, and turn on 'Share concurrent misses'. Do not simply raise the TTL: the data would go stale.",
		Predictions: []Prediction{{ID: "p1", Question: "The cache is warm and the hit rate is near 100%. Why can the database still be overwhelmed after ten seconds?", Options: []string{"The cache fills up and begins evicting the popular items", "All entries expire together, so nearly all requests miss", "The database slows down over time as its tables grow"}, Answer: 1,
			Explain: "A warm start gives every entry the same creation time, so they share one expiry time. Misses arrive as a wave, not a trickle."}},
		Debrief: "A cache hides load until the moment it cannot. Synchronised {{ttl|expiry}} turns a 99% {{cache-hit-rate|hit rate}} into a 0% one for a few seconds. Jitter spreads the expiries out, and sharing concurrent misses keeps one popular item from being fetched a hundred times. Making the TTL very long also avoids the {{stampede|stampede}}, but it trades it for stale data, which is why this challenge limits staleness.",
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
		Story:       "Every request is a write that users were told succeeded. At 20 seconds the primary database dies and {{failover|the replica is promoted}}. The replica is {{replication-lag|three seconds behind}}, so it never saw the last three seconds of writes. Lose at most twenty acknowledged writes and keep the slowest ten percent of writes under about a quarter of a second. The architecture is fixed: fix it with configuration.",
		Diagram:     replicas(promote, P{"replicationLagMs": 3000.0}),
		Solution:    replicas(promote, P{"replicationLagMs": 3000.0, "replication": "sync"}),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "db", AtSec: 20, DownSec: 300}},
		Goals:       engine.Goals{MinSuccessRate: 0.70, MaxP95Ms: f(260), MaxLostWrites: i(20), StarP95Ms: f(250)},
		DurationSec: 65, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"A write was acknowledged to the user, then the primary died. Where else did that write exist at that moment?",
			"Open the replica's settings and read the Replication help text: who waits for whom?",
			"Right now the primary acknowledges before the replica has the write. Which replication setting changes who waits for whom, and what does that do to write latency?",
		},
		Answer: "The answer: switch the replica to synchronous replication: the primary only acknowledges once the replica has the write, so nothing acknowledged is lost. Check what it does to write latency.",
		Predictions: []Prediction{{ID: "p1", Question: "What is the price of synchronous replication?", Options: []string{"Reads from the replica become slower and staler than before", "Every write waits for the replica, and stops if it is down", "Nothing: it is strictly better than asynchronous"}, Answer: 1,
			Explain: "Synchronous replication buys zero data loss with latency, and with a dependency: the primary cannot acknowledge what its replica has not confirmed."}},
		Debrief: "{{replication-lag|Replication lag}} is your data-loss window in a {{failover|failover}}. Synchronous replication closes it, at the cost of slower writes and a new way to be unavailable (a dead replica blocks writes). Shrinking the lag also helps; in this simulator lag is free to configure, but in a real system a tiny lag costs primary throughput or is simply unachievable across regions.",
	})

	// 14h ─ Hard, config only: the old primary comes back ────────────────────────────────
	register(&Scenario{
		ID: "fence-the-old-primary", Title: "Fence the old primary", Difficulty: "Hard", ConfigOnly: true,
		Summary:     "The failover worked. Then the dead primary woke up.",
		Story:       "The primary dies at 10 seconds and {{failover|a replica is promoted}} by 25 seconds. At 50 seconds the old primary restarts. Nobody told it that it was replaced. Keep the data: lose at most a hundred acknowledged writes, and keep most requests succeeding. The architecture is fixed: fix it with configuration.",
		Diagram:     replicas(P{"onFailure": "promote", "detectSec": 5.0, "promoteSec": 10.0, "fencing": false}, P{"replicationLagMs": 500.0}),
		Solution:    replicas(promote, P{"replicationLagMs": 500.0}),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 40}},
		Goals:       engine.Goals{MinSuccessRate: 0.68, MaxLostWrites: i(100)},
		DurationSec: 65, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The new primary has been taking writes for 25 seconds when the old one comes back. Who do clients talk to then?",
			"Look at the database's Failover settings: there is one that decides whether a recovered old primary is allowed back in.",
			"What stops a recovered node from taking back the role it lost? Look at the failover settings.",
		},
		Answer: "The answer: turn on 'Fence the old primary'. Without it the old primary rejoins as primary and everything written to the replacement since promotion is lost. Do not just disable failover: writes would fail for the whole outage.",
		Predictions: []Prediction{{ID: "p1", Question: "The old primary restarts and nothing stops it. What happens to writes made to the new primary in the meantime?", Options: []string{"Merged into the old primary once it rejoins the cluster", "Lost: they exist only on the new primary, which clients abandon", "Kept: both primaries replicate to each other, so nothing is lost"}, Answer: 1,
			Explain: "Two nodes that both believe they are the primary diverge. Without fencing, the survivor of the confusion silently drops the other's writes."}},
		Debrief: "{{failover|Failover}} is only half the job: you also have to make sure the old primary cannot come back as one. {{fencing|Fencing}} (a token, a lease, or simply keeping the node out until it is reconciled) is what prevents {{split-brain|split brain}}. Staying down avoids the problem but costs the whole outage.",
		Deeper:  "{{failover|Failover}} without {{fencing|fencing}} creates {{split-brain|two nodes that both believe they are the primary}}, and whichever the clients reach next wins while the other side's writes are quietly lost. Fencing has to be enforced where the data lives: a monotonically increasing term or token that the storage layer checks, a lease the old primary must renew, or simply keeping the node out until it has been reconciled, because a node cannot be trusted to know it was replaced. Staying down is the safe answer and costs the whole outage; fencing keeps both safety and availability but needs {{leader-election|a source of truth for who the current primary is}}, which leads to the coordination service in a later challenge. An interviewer will push on the harder case where the old primary never crashed and was only partitioned, so it keeps serving writes it believes are valid. That is the real split-brain failure, and it is why the check must come from the resource and not from the process.",
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
		Story:   "Every forty seconds traffic jumps from 40 to 180 requests a second for twenty seconds and drops again. {{autoscaling|Autoscaling}} is switched on, yet each wave starts with a burst of failures: a new server takes ten seconds to boot, and the scaler only looks at the last thirty seconds of load. Keep success high and the slowest requests fast without buying a big fleet for the whole run. The architecture is fixed: fix it with configuration.",
		Diagram: waveFleet(nil), Solution: waveFleet(P{"minInstances": 3.0}),
		Traffic:     map[string][]engine.TrafficPoint{"users": waves},
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(500), MaxCostPerHour: f(0.5)},
		DurationSec: 260, WarmupSec: 5, Speed: 4,
		Hints: [3]string{
			"Look at when the failures happen: at the start of each wave. How much capacity is running at that moment, and how long until more arrives?",
			"Open the app server's settings, 'Show advanced', Scaling group. Waiting longer before removing servers is not the whole story: think about the floor and about when the scaler decides to act.",
			"The scaler always reacts late. Which two settings let capacity already be there when the wave arrives, and which one costs less?",
		},
		Answer: "The answer: keep a floor of servers that covers the wave (minimum servers 3), or make the scaler act earlier with a lower target busy level. Compare the cost of each.",
		Predictions: []Prediction{{ID: "p1", Question: "A wave arrives every 40 seconds and a server takes 10 seconds to boot. Which setting helps most?", Options: []string{"Remove idle servers sooner so the monthly bill stays low", "Keep a floor of servers: reactive help comes too late", "Make the scaler wait longer before removing servers"}, Answer: 1,
			Explain: "Reactive scaling always lags demand by the measuring window plus the boot time. When waves are shorter than that lag, only capacity that is already running helps."}},
		Debrief: "{{autoscaling|Autoscaling}} is reactive, so it suits changes slower than its own reaction time. For predictable, fast waves, pre-provision a floor (or scale on a schedule); a lower target busy level buys earlier action at a higher cost. Waiting longer before scaling down avoids flapping but would not have fixed the wave start.",
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
			"Which crowd keeps sending however the server is doing? Size the server for that crowd, not for the one that waits politely.",
		},
		Answer: "The answer: size the server for the public crowd, which is open-loop (its load does not fall when the server struggles): raise the app server's {{thread-pool|threads}}.",
		Predictions: []Prediction{{ID: "p1", Question: "Why can a closed-loop load test pass at a load that breaks the system in production?", Options: []string{"Production has more data, so queries are slower", "Users who wait for answers slow down as the system does", "Load tests run from inside the data centre"}, Answer: 1,
			Explain: "A fixed crowd that waits is self-limiting: latency rises, arrivals fall. Real internet traffic keeps coming at the same rate however slow you are."}},
		Debrief: "Closed-loop tests flatter a system: the offered load shrinks as latency grows (and they hide latency by not measuring the requests that were never sent). Real traffic is mostly open-loop, so {{littles-law|size for the arrival rate}}, not for the number of testers.",
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
			"Average load and peak load are different numbers. What does the server do during a rush, and what happens to the requests that arrive while {{thread-pool|all threads are busy}}?",
			"A queue turns a spike into a delay, as long as it is big enough and the wait stays acceptable. The rate limiter rejects traffic: a rejected request is still a failed request.",
			"One setting on the queue and one on the threads: how long a queue can you afford before the slowest requests are too late, and how fast must the threads drain it?",
		},
		Answer: "The answer: make the queue big enough to hold a rush (about a hundred) and give the server a few more threads so the queue drains quickly. A huge queue alone pushes the slowest requests past 0.7 s; sixteen threads cost too much.",
		Predictions: []Prediction{{ID: "p1", Question: "Average load is 70% of capacity. Why do users still see errors?", Options: []string{"The average is miscalculated, and load is really higher", "Rushes well above capacity overflow a short queue", "The database slows down once the cache fills up"}, Answer: 1,
			Explain: "Utilisation is an average over time; failures happen in the moments when arrivals exceed capacity. Queues and headroom absorb those moments."}},
		Debrief: "Provision for the burst, not the mean. A queue absorbs a burst only if it is large enough and drains fast enough; it trades errors for waiting time. {{load-shedding|Rate limiting}} protects a system from overload but does not make rejected users succeed, so it is the wrong tool when the burst is short enough to wait out.",
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
		Story:   "You {{sharding|sharded}} the database four ways so no shard sees more than a quarter of the traffic. Then {{hot-key|one item went viral}}: 35% of all requests ask for it, and it lives on exactly one shard, which now takes half the load and falls over while the other three idle. Fix it. You may add and connect components.",
		Diagram: hotShard(4, false), Solution: hotShard(4, true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(500), MaxCostPerHour: f(1.7), StarCostPerHour: f(1.65)},
		DurationSec: 50, WarmupSec: 3, Speed: 1,
		Hints: [3]string{
			"Which shard is failing, and why does adding capacity to the other three not help?",
			"More shards will not spread one item: it still maps to a single shard. What could answer the popular reads without touching that shard at all?",
			"What could answer the hot item's reads before they reach the shard, and where in the path would it have to sit?",
		},
		Answer: "The answer: put a cache in front of the shard router (connect the app servers to it and the cache to the router). A tiny cache is enough: the {{hot-key|hot item}} stays in it.",
		Predictions: []Prediction{{ID: "p1", Question: "One item gets 35% of reads. What does going from 4 shards to 8 do?", Options: []string{"Halves the hot shard's load, as the keys split across eight", "Helps a little, but the hot item still lives on one shard", "Nothing at all, because the router ignores new shards"}, Answer: 1,
			Explain: "Resharding spreads the other keys thinner, but a single key cannot be split by hashing: its load stays on whichever shard owns it."}},
		Debrief: "{{sharding|Sharding}} balances many keys, not one. A {{hot-key|hot key}} needs a different tool: a cache in front, a replica set for that key, or splitting the key (for example, a counter sharded into N sub-counters). Check the load per shard, not the average.",
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
		Story:   "Users keep asking for the same 500 popular items, and the cache holds 600. A search-engine crawler is walking your whole catalogue of 100,000 items, once each, at 150 requests a second. Every item the crawler touches goes into the cache and {{eviction|pushes out something users want}}. The database can only do 200 reads a second. Keep users fast. The architecture is fixed: fix it with configuration.",
		Diagram: scanned(P{"capacityItems": 600.0, "eviction": "fifo"}), Solution: scanned(P{"capacityItems": 600.0, "eviction": "lfu"}),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(400)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Items the crawler reads once go into the cache and are never read again. What do they push out?",
			"Open the cache's eviction setting. Which policy decides by how recently an item was used, and which by how often?",
			"Which policy counts how often an item is used rather than when? Compare how each policy treats an item read exactly once.",
		},
		Answer: "The answer: switch eviction to LFU (least frequently used): an item read once has a count of one and is the first to go, so the popular items stay. LRU is better than FIFO here but still lets a long scan push things out; a much bigger cache also works but is the expensive answer.",
		Predictions: []Prediction{{ID: "p1", Question: "Which eviction policy best protects a small popular set from a one-pass scan?", Options: []string{"FIFO: the oldest item goes first", "LFU: least frequently used out first", "Random: no item is favoured by the scan"}, Answer: 1,
			Explain: "A scan produces many items with a count of one. LFU evicts those first and keeps the items that are read again and again."}},
		Debrief: "{{eviction|Eviction}} policy is a bet about the future. LRU assumes recent means useful, which a scan breaks; FIFO ignores use altogether; LFU assumes frequent means useful, which resists scans (but can hold stale favourites). Real systems often add scan-resistant variants or keep crawler traffic out of the cache.",
	})
}
