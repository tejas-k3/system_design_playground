package scenarios

import (
	"fmt"

	"sysdesign/internal/engine"
)

// registerConsistencyAndStreams registers restores, replication and quorum consistency, leases, hot rows, and stream counting and ranking.
func registerConsistencyAndStreams() {
	// Restore drill: a bad migration corrupts the database; how much is lost and how long until it is back
	restoreDrill := func(fixed bool) engine.Diagram {
		every, mbps := 0.0, 200.0
		if fixed {
			every, mbps = 5, 600
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 60.0, "readPct": 50.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 32.0, "dataGB": 10.0, "backupEverySec": every, "restoreMBps": mbps}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "restore-drill", Title: "The restore drill", Difficulty: "Medium", ConfigOnly: true,
		Summary: "A bad migration corrupts the database. How much is lost, and for how long is it gone?",
		Story:   "At 25 seconds a bad migration starts silently corrupting rows. Someone notices three seconds later and restores the last backup from before the damage. The database holds 10 GB. There are no backups at all, and restoring streams 200 MB a second. The business can lose at most five seconds of writes and can be without the database for at most twenty seconds.",
		Diagram: restoreDrill(false), Solution: restoreDrill(true),
		Chaos:       []engine.ChaosEvent{{Type: "corrupt", NodeID: "db", AtSec: 25}, {Type: "restore", NodeID: "db", AtSec: 28}},
		Goals:       engine.Goals{MinSuccessRate: 0.7, MaxRpoSec: f(5), MaxRtoSec: f(20), MaxCostPerHour: f(1.1), StarCostPerHour: f(1.0)},
		DurationSec: 80, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Two numbers decide this: how much is lost (the recovery point) and how long you wait (the recovery time). Which setting moves each?",
			"A restore goes back to the last backup before the damage, so the data lost is the time since that backup: with no backups it is everything. And the time to restore is the data size divided by the restore speed: 10 GB at 200 MB/s is over 50 seconds.",
			"Two numbers decide the drill: how much has changed since the last backup, and how fast the copy streams back. Which setting moves each?",
		},
		Answer: "The answer: turn on backups every few seconds on the database, and raise the restore speed. Each costs a little: a restore bandwidth that is higher than you need is wasted money.",
		Predictions: []Prediction{{ID: "p1", Question: "Backing up every five seconds (and nothing else) will…", Options: []string{"Fix both the data lost and the time taken to restore", "Cut the data lost but leave the restore as slow", "Cut the time to restore but keep the same data loss"}, Answer: 1,
			Explain: "How often you back up sets how much you can lose; how fast a backup streams back sets how long you wait. They are separate levers."}},
		Debrief: "RPO and RTO are different dials: backup frequency sets the recovery point, restore bandwidth and data size set the recovery time. Pick them from what the business can bear, and then practise the restore.",
	})

	// Lost cart: two leaders accept writes for the same cart; last-writer-wins silently drops one of them
	lostCart := func(fixed bool) engine.Diagram {
		policy := "lastWriterWins"
		if fixed {
			policy = "merge"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 150.0, "readPct": 0.0, "keySpace": 100.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 32.0, "topology": "multiLeader", "leaders": 2.0, "syncLagMs": 300.0, "conflictPolicy": policy}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "lost-cart", Title: "The lost cart item", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Customers add items to a cart and some of them silently vanish.",
		Story:   "To keep checkout fast in two regions, the cart database has two leaders that each accept writes and {{replication-lag|pass them to the other 300 ms later}}. Two people adding to the same cart in that gap each succeed, and then one of the additions disappears: the database keeps whichever write is newer. Nothing errors, so nobody notices until customers complain.",
		Diagram: lostCart(false), Solution: lostCart(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxLostUpdates: i(0), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.0)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Count the lost updates on the goals panel: every request succeeds, so what is being lost, and when?",
			"A write to a cart at one leader and another write to the same cart at the other leader within 300 ms are concurrent: neither leader has seen the other's. With last-writer-wins one of them is thrown away.",
			"What should happen when two writes to the same cart meet? Which data types can combine both instead of choosing one?",
		},
		Answer: "The answer: choose what happens on a conflict: a merging data type (a counter or a set that adds) keeps both. Shortening the delay only makes conflicts rarer, and one leader would avoid them altogether at the cost of a slow region.",
		Predictions: []Prediction{{ID: "p1", Question: "If the leaders passed writes on in 50 ms instead of 300, lost updates would…", Options: []string{"Disappear altogether once writes cross quickly", "Become rarer but still happen", "Double, as more writes cross each other"}, Answer: 1,
			Explain: "The window in which two writes can be concurrent shrinks, so fewer collide, but any two writes inside it still conflict."}},
		Debrief: "With more than one writer, concurrent writes to the same item are normal, and the conflict rule decides who loses. Last-writer-wins is easy and loses data silently; data types that merge keep every update.",
		Deeper:  "With several writers, concurrent writes to one item are normal, and the conflict rule decides what survives; last-writer-wins is simple and drops data without any error. Merging types such as a set that adds, or a counter, keep every update, but they only work when the data model can be merged, and removals need extra care or a deleted item comes back. Shortening the {{replication-lag|replication delay}} only makes conflicts rarer, and routing all writes for a cart to one home region removes them at the cost of a slow write for far users. At larger scale you need version vectors or similar to tell concurrent writes from ordered ones, because relying on clocks lets skew pick the wrong winner. An interviewer will ask what you do with a conflict that cannot be merged, such as a stock count, and how a user would be shown it.",
	})

	// Quorum overlap: a key-value store that answers quickly but sometimes shows a client its own old data
	quorumOverlap := func(fixed bool) engine.Diagram {
		r, w := 1.0, 1.0
		if fixed {
			r, w = 2, 2
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 50.0, "keySpace": 20.0, "keyDist": "uniform", "consistency": "causal", "timeoutMs": 3000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "dbPool": 200.0}).
			n("kv", engine.KKVStore, cols[2], rowMid, P{"nodes": 5.0, "replicas": 3.0, "readQuorum": r, "writeQuorum": w, "readMs": 4.0, "writeMs": 60.0, "variability": "exponential", "concurrency": 32.0}).
			e("users", "app").e("app", "kv").done()
	}
	register(&Scenario{
		ID: "quorum-overlap", Title: "Read your own writes", Difficulty: "Hard", ConfigOnly: true,
		Summary: "The store is fast and never errors, but users sometimes lose sight of what they just saved.",
		Story:   "A five-node key-value store keeps three copies of every key. To be quick, a write returns as soon as one copy has it and {{consistency-levels|a read returns the first copy that answers}}. Users save a setting, reload, and sometimes see the old value. Support has noticed that it also flips back and forth between old and new.",
		Diagram: quorumOverlap(false), Solution: quorumOverlap(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxSessionViolations: i(0), MaxP95Ms: f(600), MaxCostPerHour: f(1.6), StarCostPerHour: f(1.4)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The goals panel counts reads that returned something older than what the same client already knew. Where could an older copy come from?",
			"With three copies, a write acknowledged by one copy and a read answered by a different one need not meet: the read can reach copies the write has not got to yet.",
			"How many copies must a read and a write touch, together, so they always share at least one? Which two numbers add up to more than the number of copies?",
		},
		Answer: "The answer: make every read overlap every acknowledged write: the read {{quorum|quorum}} plus the write quorum must be more than the number of copies (3). Two and two does it; the price is waiting for the second-fastest copy.",
		Predictions: []Prediction{{ID: "p1", Question: "With three copies, a read quorum of 1 and a write quorum of 3 would…", Options: []string{"Still show old data, because a single read can miss the new write", "Never show old data, but make writes slower and fragile", "Be exactly as fast as 2 and 2, just reversed"}, Answer: 1,
			Explain: "1 + 3 is more than 3, so the read always meets a copy with the write, but a write now waits for the slowest copy and needs all of them to be up."}},
		Debrief: "{{quorum|R + W > N}} makes every read share a copy with the latest completed write, so the read sees that write. Also, W > N/2 makes any two write quorums share a copy, so two writes cannot succeed without meeting. These rules do not stop successive reads going backwards while a write is only partly applied. You buy them with latency (the kth fastest copy instead of the fastest) and availability (fewer failures tolerated). Choose R and W for the workload: read-heavy systems often use a cheap read quorum and a larger write quorum.",
		Deeper:  "When R + W > N, every read shares a copy with the latest completed write, so it sees that write; W > N/2 also makes any two write quorums share a copy, so two writes cannot succeed without meeting. These rules do not stop successive reads going backwards while a write is only partly applied. You buy them with latency (the kth fastest copy instead of the fastest) and availability (fewer failures tolerated); for a read-heavy workload you can make reads cheap and writes expensive, but then one dead copy blocks every write. The guarantee is weaker than it looks: a write that reached only some copies before failing can still be seen by some reads, concurrent writes need a version or clock to order them, and {{quorum|sloppy quorums}} that accept stand-in nodes during an outage give up the overlap exactly when you need it. At larger scale you add read repair and background anti-entropy so copies converge. An interviewer will ask what changes across data centres, where a local quorum is cheap and a global one is slow.",
	})

	// Lease expired: a frozen lock holder wakes up and books a seat someone else already took
	leaseExpired := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 600.0, "readPct": 0.0, "workload": "booking", "keySpace": 3000.0, "keyDist": "uniform", "timeoutMs": 4000.0, "retries": 0.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 64.0, "writeMs": 40.0, "conditionalWrite": fixed}).
			n("redis", engine.KRedis, cols[2], 320, nil).
			e("users", "lb")
		for k := 0; k < 3; k++ {
			id := fmt.Sprintf("app%d", k+1)
			b.n(id, engine.KAppServer, cols[2], float64(k)*100+30, P{"threads": 24.0, "queueCap": 2000.0, "lockTtlMs": 150.0, "lockWaitMs": 300.0, "dbPool": 100.0, "gcPauseMs": 600.0, "gcEverySec": 0.5}).e("lb", id).e(id, "db").e(id, "redis")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "lease-expired", Title: "The lease that ran out", Difficulty: "Hard", ConfigOnly: true,
		Summary: "The lock was held. The seat was booked twice anyway.",
		Story:   "Seats are booked under a Redis lock that expires after 150 ms, so a crashed server cannot hold one forever. The app servers freeze for 600 ms every so often (a garbage-collection pause). A server that froze while holding a lock wakes up long after it expired, another server has booked the seat in the meantime, and the first one books it anyway: two people have the same seat.",
		Diagram: leaseExpired(false), Solution: leaseExpired(true),
		Goals:       engine.Goals{MinSuccessRate: 0.93, MaxConflicts: i(0), MaxP95Ms: f(4000), StarP95Ms: f(500), MaxCostPerHour: f(3.2), StarCostPerHour: f(3.0)},
		DurationSec: 90, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at when the double bookings happen: they follow the garbage-collection pauses. What does a server that was frozen believe about its lock when it wakes up?",
			"It still thinks it holds the lock, but the lease ran out while it was frozen. Nothing the lock service does can reach into a process that is not running: the check has to happen where the write lands.",
			"The stale writer cannot be stopped from outside. Where could the write itself be refused, using something the database already knows about the seat?",
		},
		Answer: "The answer: make the database book a seat only if it is still free (a conditional write, like a unique constraint). A {{fencing|fencing token}} from a lock service helps only when the stale write arrives after the newer one; the conditional write does not depend on order.",
		Predictions: []Prediction{{ID: "p1", Question: "Why is a lock with an expiry not enough to guarantee that only one process acts at a time?", Options: []string{"The expiry is always set too short to be safe", "A paused process can act after its lease has been reissued", "Redis loses locks whenever it restarts"}, Answer: 1,
			Explain: "A lease bounds how long a dead holder blocks others, but a live holder that is frozen does not know its lease ended. Only the resource being written can refuse the stale write."}},
		Debrief: "Treat a lock as an optimisation, not a guarantee. Make the write itself safe: a conditional write (compare-and-set, unique constraint) refuses whatever arrives late, and a {{fencing|fencing token}} does the same when the resource can compare tokens.",
		Deeper:  "A lock with an expiry only bounds how long a dead holder blocks others; it cannot stop a live holder that was paused by garbage collection, a swapped-out process or a slow network from acting after the lease has passed to someone else. The write itself must refuse stale work: a conditional write such as compare-and-set or a unique constraint does not depend on timing, while a {{fencing|fencing token}} only helps when the resource can compare tokens and sees them in order. The lock is still useful as an optimisation because it reduces contention, so treat it as that and not as the safety mechanism. An interviewer will ask what you do when the final resource, such as an external API, cannot do a conditional write; the usual answer is to make the operation {{idempotency|idempotent}} or to give each key a single consumer. The failure pattern is any pause longer than the lease, and clock jumps produce the same result.",
	})

	// Who is the leader: failover is decided by a coordination service that has itself lost its majority
	whoLeads := func(fixed bool) engine.Diagram {
		nodes := 3.0
		if fixed {
			nodes = 5
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 40.0, "keyDist": "uniform", "keySpace": 20000.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[4], rowTop, P{"onFailure": "promote", "electionBy": "coordinator", "detectSec": 3.0, "promoteSec": 5.0}).
			n("replica", engine.KReplica, cols[4], rowMid, P{"replicationLagMs": 500.0}).
			n("zk", engine.KCoordinator, cols[4], rowBot, P{"nodes": nodes, "electionTimeoutMs": 1000.0}).label("zk", "Coordination service").
			e("users", "lb").e("db", "zk").servers(2, cols[2], rowTop, 100, P{"threads": 16.0}, "replica").done()
	}
	register(&Scenario{
		ID: "who-is-the-leader", Title: "Who is the leader?", Difficulty: "Hard", ConfigOnly: true,
		Summary: "The primary is dead, a replica is ready, and nobody is allowed to promote it.",
		Story:   "{{failover|Failover}} here is not left to a timer: promotion waits until the coordination service {{leader-election|agrees on who the leader is}}, because promoting without agreement is how {{split-brain|two primaries}} appear. Tonight two of the coordination service's three machines fail at the same time as the database primary. The service has {{quorum|no majority}}, so it elects nobody, so the replica waits. Writes fail the whole time.",
		Diagram: whoLeads(false), Solution: whoLeads(true),
		Chaos:       []engine.ChaosEvent{{Type: "clusterNodeDown", NodeID: "zk", AtSec: 15, DurationSec: 40, Factor: 2}, {Type: "crash", NodeID: "db", AtSec: 20, DownSec: 60}},
		Goals:       engine.Goals{MinSuccessRate: 0.85, MaxP95Ms: f(800), MaxLostWrites: i(100), MaxCostPerHour: f(2.5)},
		DurationSec: 70, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The replica is ready but the database is not promoting it. Who has to agree first, and can they?",
			"The coordination service needs a majority of its machines to elect anyone. Count how many it has and how many it lost.",
			"How many machines must be alive for a majority, and how many machines does the cluster need so that it can lose two and still have that many?",
		},
		Answer: "The answer: with 3 machines, losing 2 leaves no majority. Run 5 (a majority is 3): the same two failures leave a working {{quorum|quorum}} and the promotion goes ahead on schedule.",
		Predictions: []Prediction{{ID: "p1", Question: "A 3-machine cluster loses two machines. Can the survivor keep serving as leader?", Options: []string{"Yes: one machine is enough to lead alone", "No: a majority (2 of 3) is needed for a leader", "Yes, but only for reads, not writes"}, Answer: 1,
			Explain: "A majority is what makes two leaders impossible: two groups cannot both hold more than half. The price is that the cluster stops when it loses its majority."}},
		Debrief: "{{quorum|Quorums}} trade availability for safety. N machines tolerate (N-1)/2 failures: 3 tolerate one, 5 tolerate two. Size the cluster for the failures you expect together, and spread it across failure domains so they do not happen together.",
		Deeper:  "A coordination service stays safe by refusing to act without {{quorum|a majority}}, so {{cap-trade-off|it trades availability for the guarantee that two leaders can never exist}}; with N machines it tolerates (N-1)/2 failures, which is why three survive one loss and five survive two. An even number adds cost without adding tolerance. Machines that share a zone, rack or power feed fail together, so the count only protects you if the members sit in different failure domains. Keep the data path out of the coordinator and use it only for {{leader-election|election}} and small configuration, since every request through it needs a majority round trip. An interviewer will ask what the database does while there is no leader, which here is failing writes rather than risking {{split-brain|two primaries}}, and how clients find the new leader. The classic failure is a cluster that lost its majority during a rolling upgrade because one member was already down.",
	})

	// Launch day: nearly every write is to one row, and row locks make those writes queue one behind the other
	launchDay := func(fixed bool) engine.Diagram {
		app := P{"threads": 64.0, "queueCap": 2000.0, "dbPool": 100.0}
		if fixed {
			app["hotKeyMode"] = "split"
			app["splitShards"] = 8.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 200.0, "readPct": 0.0, "keySpace": 5000.0, "keyDist": "uniform", "hotKeyPct": 90.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, app).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 64.0, "writeMs": 20.0, "hotRowLock": true}).label("db", "Stock database").
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "launch-day", Title: "Launch day", Difficulty: "Hard", ConfigOnly: true,
		Summary: "The new phone goes on sale and every buyer decrements the same stock row.",
		Story:   "At noon the new phone goes on sale. {{hot-key|Nine in ten orders change the same row}}, the stock counter, and the database locks a row while it writes it, so those writes go one after another: 20 ms each is 50 a second, and 180 a second are arriving. The queue behind that row holds {{connection-pool|database connections}}, and the buyers time out before their turn comes. The rest of the shop has nothing wrong with it.",
		Diagram: launchDay(false), Solution: launchDay(true),
		Goals:       engine.Goals{MinSuccessRate: 0.95, MaxP95Ms: f(500), MaxCostPerHour: f(4.0), StarCostPerHour: f(3.6), StarP95Ms: f(100)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Which row is slow, and what limits how fast one row can change?",
			"A row lock lets one write at a time through. With 20 ms per write one row takes 50 writes a second whatever you do to the database around it; more connections only give the waiting writes somewhere to wait.",
			"One row is a ceiling however big the database is. How could the counter be spread over several rows, and what does that cost when reading the total?",
		},
		Answer: "The answer: stop writing to a single row: in the app server's hot-key setting, split the counter over several rows, so the writes go to different rows in parallel. Reading the total then costs adding the rows up.",
		Predictions: []Prediction{{ID: "p1", Question: "Doubling the database connections will…", Options: []string{"Double the writes the hot row can take", "Not change what one row can take: the lock is the limit", "Halve the latency of every write to the row"}, Answer: 1,
			Explain: "A lock serialises the writes to its row. More connections let more of them wait at once, which does not make the row faster."}},
		Debrief: "A {{hot-key|hot row}} is a {{throughput|throughput}} ceiling of one over the write time, however big the database. Split the counter (several sub-rows added up on read), batch the updates, or queue them so they are applied in order without making buyers wait.",
		Deeper:  "A row lock makes writes to one row sequential, so the ceiling is one over the write time however large the database is; here that is 50 a second against 180 arriving. Splitting the counter over several rows lets the writes proceed in parallel but turns every read of the total into a sum, and it makes the question whether stock is zero inexact, so overselling needs care: hand each sub-row a share of the stock and rebalance when one runs dry. Batching or queueing the updates also works and keeps them ordered, at the cost of buyers waiting or learning of a failure later. The same shape appears in like counters and view counts. An interviewer will ask how you guarantee you never sell more than you have and whether a virtual waiting room would be fairer. The failure pattern is timeouts on the hot row {{retry-storm|triggering retries}}, which lengthens the very queue that caused them.",
	})

	// Count the clicks: a stream job counts clicks per window, but some arrive long after they happened
	countClicks := func(fixed bool) engine.Diagram {
		st := P{"windowSec": 10.0, "watermarkDelaySec": 2.0, "lateEventPct": 15.0, "lateBySec": 20.0}
		if fixed {
			st["allowedLatenessSec"] = 30.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 200.0, "readPct": 0.0, "keySpace": 10000.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0, "queueCap": 500.0}).
			n("clicks", engine.KStream, cols[2], rowMid, st).label("clicks", "Clicks per 10 seconds").
			e("users", "app").e("app", "clicks").done()
	}
	register(&Scenario{
		ID: "count-the-clicks", Title: "Count the clicks", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The click counts are wrong because some clicks arrive late.",
		Story:   "A dashboard shows clicks per ten seconds, counted by a stream job by the time each click happened. About one click in seven comes from a phone that was offline and reaches the server twenty seconds after it happened. The job has already published the window by then and throws the click away, so the totals are always a little low. The dashboard must still show each window within three seconds of it ending.",
		Diagram: countClicks(false), Solution: countClicks(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxLateDropped: i(20), MaxResultDelaySec: f(3), MaxCostPerHour: f(1.0)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the stream job's stats: how many clicks does it drop, and why?",
			"A window is published when the watermark (the newest click time minus a delay) passes its end. A click that arrives after that is dropped, unless the job is told to accept late ones for a while.",
			"The dashboard cannot wait longer. Is there a way to publish on time and still correct a window when a late click turns up?",
		},
		Answer: "The answer: waiting longer before publishing (the watermark delay) would catch them, but the dashboard must not wait more than three seconds. Instead, allow lateness: publish on time, then update the window when a late click arrives.",
		Predictions: []Prediction{{ID: "p1", Question: "To count clicks that arrive 20 seconds late, the dashboard must…", Options: []string{"Wait 20 seconds before showing anything at all, to be safe", "Show the window on time and correct it when late clicks arrive", "Ignore late clicks, since they are rare and make little difference"}, Answer: 1,
			Explain: "A longer watermark delay trades freshness for completeness. Allowed lateness gives both, at the price of results that change after they were shown."}},
		Debrief: "Event time is when something happened, not when you heard about it. The watermark is a guess about how late data can be: too tight and you drop data, too loose and you answer slowly. Allowed lateness, with updates to published windows, lets you answer early and still end up correct.",
	})

	// Unique visitors: counting different people exactly needs memory that grows with the audience
	uniqueVisitors := func(fixed bool) engine.Diagram {
		st := P{"counting": "exact"}
		if fixed {
			st = P{"counting": "hyperLogLog", "hllPrecision": 12.0}
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 200.0, "readPct": 0.0, "keySpace": 5000.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0, "queueCap": 500.0}).
			n("visitors", engine.KStream, cols[2], rowMid, st).label("visitors", "Unique visitors").
			e("users", "app").e("app", "visitors").done()
	}
	register(&Scenario{
		ID: "unique-visitors", Title: "Unique visitors", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Counting each visitor exactly needs a table that grows with the audience.",
		Story:   "The marketing dashboard shows how many different people visited. The stream job keeps every visitor id it has seen, so the table grows with the audience, and the job has a memory budget of 16 KB for it (in this small run; a real audience is thousands of times larger and so is the table). Marketing is happy with a number that is right to within a few percent.",
		Diagram: uniqueVisitors(false), Solution: uniqueVisitors(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxStateKB: f(16), MaxCountErrorPct: f(6), MaxCostPerHour: f(1.0)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the stream job's memory for counting. What makes it grow, and how fast?",
			"An exact count needs to remember every id to know whether it was seen before. If a number that is slightly off is acceptable, there are structures that count distinct items in a fixed, tiny amount of memory.",
			"Which structure counts distinct items in a fixed amount of memory? Its precision trades error against memory: find a setting that meets both limits.",
		},
		Answer: "The answer: switch the job's visitor counting to HyperLogLog. Its precision sets the number of registers: more registers, less error, more memory. Pick one that fits both the 16 KB budget and the 6% error.",
		Predictions: []Prediction{{ID: "p1", Question: "A HyperLogLog with twice the registers has…", Options: []string{"Half the error for twice the memory", "About 30% less error for twice the memory", "The same error, but it handles more visitors"}, Answer: 1,
			Explain: "The error falls with the square root of the registers: doubling them divides it by about 1.4, while memory doubles. Accuracy gets expensive quickly."}},
		Debrief: "Approximate structures trade a bounded, known error for memory that does not grow with the data: HyperLogLog counts distinct items, a Bloom filter answers 'seen before?', a count-min sketch counts how often. They are the answer whenever the exact version will not fit.",
	})

	// Top ten: the ten most popular items out of a huge catalogue, in a small amount of memory
	topTen := func(fixed bool) engine.Diagram {
		st := P{"topK": "exact"}
		if fixed {
			st = P{"topK": "countMin", "sketchWidth": 1000.0, "sketchDepth": 3.0}
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 300.0, "readPct": 0.0, "keySpace": 200000.0, "keyDist": "zipf", "zipfS": 1.1, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0, "queueCap": 500.0}).
			n("trending", engine.KStream, cols[2], rowMid, st).label("trending", "Trending items").
			e("users", "app").e("app", "trending").done()
	}
	register(&Scenario{
		ID: "top-ten", Title: "The top ten", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Find the ten most popular items out of 200,000 without a counter for each.",
		Story:   "The shop shows a trending list: the ten most viewed items out of a catalogue of 200,000. The stream job keeps an exact counter for every item it has seen, and the table keeps growing: it already needs far more memory than the 16 KB the job is allowed (in this small run; the real catalogue and traffic are far bigger). The list does not have to be perfect, but at least eight of the ten must be right.",
		Diagram: topTen(false), Solution: topTen(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MinTopKRecall: f(0.8), MaxStateKB: f(16), MaxCostPerHour: f(1.0)},
		DurationSec: 60, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the memory the stream job needs and at what it is counting. Does every item need its own counter?",
			"Most of the 200,000 items are seen once or never. Only the few very popular ones matter for a top ten, and a structure can estimate counts for all items in a fixed table, at the cost of items sharing counters.",
			"Which probabilistic structure counts every item in a fixed table? Its width and depth set both accuracy and memory: what do the limits say about them?",
		},
		Answer: "The answer: switch the job to the count-min sketch. Its width and depth set the table (width x depth counters of 4 bytes each): a table too narrow mixes everything together and misses the top ten, one too big breaks the memory budget.",
		Predictions: []Prediction{{ID: "p1", Question: "A count-min sketch estimates how often an item occurred. It can be wrong by…", Options: []string{"Counting too few: it can miss occurrences", "Counting too many: other items share its counters", "Either way, equally, depending on the table"}, Answer: 1,
			Explain: "Every occurrence of an item adds to all of its counters, so an estimate is never too low. Other items sharing a counter can only inflate it, and taking the smallest counter keeps that small."}},
		Debrief: "{{hot-key|Heavy hitters}} are few, and a structure that only needs to be right about them can be tiny. A count-min sketch plus a short candidate list finds the top items in a fixed table; the width bounds the overcount and the depth bounds the chance of it. Exactness costs memory in proportion to the number of different items.",
		Deeper:  "Exact counting needs memory in proportion to the number of different items, while a count-min sketch uses a fixed table: each occurrence increments one counter per row, so an estimate can be too high because of collisions but never too low, and taking the smallest counter keeps the overcount small. The table here holds width times depth counters of 4 bytes each, so the budget fixes how wide it can be; the sketch gives counts but not names, so you also keep a short candidate list of the current leaders. Sketches from different workers merge by adding the tables, which is what makes them practical when the stream is spread over many machines. An interviewer will ask how you would get the top ten for the last hour, which needs a sketch per time slice, and when an exact structure with a bounded number of counters is a better fit. The failure is a table too narrow for a long tail: the noise floor rises until items near the bottom of the top ten swap places.",
	})
	// Effect before cause: replies are seen before the post they answer, because a read can land on a copy that has not caught up
	effectBeforeCause := func(fixed bool) engine.Diagram {
		r, w := 1.0, 1.0
		if fixed {
			r, w = 2, 2
		}
		return (&bld{}).
			n("authors", engine.KClient, cols[0], rowTop, P{"rps": 100.0, "readPct": 50.0, "keySpace": 20.0, "keyDist": "uniform", "consistency": "causalOrder", "timeoutMs": 3000.0, "retries": 0.0}).label("authors", "Authors").
			n("readers", engine.KClient, cols[0], rowBot, P{"rps": 150.0, "readPct": 100.0, "keySpace": 20.0, "keyDist": "uniform", "consistency": "causalOrder", "timeoutMs": 3000.0, "retries": 0.0}).label("readers", "Readers").
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "dbPool": 200.0}).
			n("kv", engine.KKVStore, cols[2], rowMid, P{"nodes": 5.0, "replicas": 3.0, "readQuorum": r, "writeQuorum": w, "readMs": 4.0, "writeMs": 60.0, "variability": "exponential", "concurrency": 32.0}).
			e("authors", "app").e("readers", "app").e("app", "kv").done()
	}
	register(&Scenario{
		ID: "effect-before-cause", Title: "The reply before the question", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Readers see a reply, then open the post it answers and find an older version.",
		Story:   "A discussion app keeps posts and replies in a five-node key-value store with three copies of each item. An author writes a reply straight after reading the post it answers. Readers then sometimes see the reply and, when they open the post it answers, an older version of it: the answer arrives before the question. Nothing fails, so nobody is told.",
		Diagram: effectBeforeCause(false), Solution: effectBeforeCause(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxSessionViolations: i(0), MaxP95Ms: f(220), MaxCostPerHour: f(1.6), StarP95Ms: f(160), StarCostPerHour: f(1.45)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The goals count reads that showed an older version of something after the reader had already seen a newer thing built on it. Where could an older copy come from?",
			"With three copies, a write that one copy has acknowledged and a read answered by a different single copy need not meet: the read can reach a copy the write has not got to yet.",
			"Which settings decide how many copies a read and a write must touch? Does changing only one of them make every read meet every write?",
		},
		Answer: "The answer: make reads and writes overlap, with a read quorum of 2 and a write quorum of 2 over three copies, so a read always meets the newest acknowledged write. Read repair helps (it brings a stale copy up to date when it is read, and removes most violations without a second copy's wait) but it does not reach zero, because the first read after a write can still hit a stale copy.",
		Predictions: []Prediction{{ID: "p1", Question: "Raising only the read quorum to 2, and leaving writes at 1, will…", Options: []string{"Remove every violation, because a read now sees two copies", "Cut them sharply but not to zero, because a write may still sit on one copy", "Change nothing, because only the write quorum matters"}, Answer: 1,
			Explain: "A read of two copies and a write on one copy can still miss each other when the write is on the third copy: 2 + 1 is not more than 3. Here it left 11 to 25 violations a run, down from about 90."}},
		Debrief: "A cause must never be shown older than an effect built on it. When a read and a write each touch only some of the copies, they can miss each other, so a reader can see a reply and then an older post. {{quorum|Overlapping quorums}} (R + W > N) make every read meet the latest acknowledged write. Read repair helps without a second copy's wait, but only after a read has found the copy behind.",
		Deeper:  "Causal consistency says that if one write was built on what its author had read, nobody should see the later write without that earlier state. A key-value store gets there by making sure a read meets the newest acknowledged write, which R + W > N does; raising only one of the two narrows the gap but does not close it, and writing to every copy pays for it on every write and stops working when one copy is down. {{read-your-writes|Read-your-writes}} and monotonic reads are weaker, per-client guarantees; causal order also covers what one client's write was built on, for every reader. Read repair fixes a copy only after a read has found it behind, so it removes most violations without a quorum's latency, but it does not guarantee that the first read after a write is right. An interviewer will ask how you would keep cause and effect together without a quorum on every read; the usual answer is to carry a version or a dependency with each write and have the reader wait or retry until it holds.",
	})
}
