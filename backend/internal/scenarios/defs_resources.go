package scenarios

import (
	"fmt"

	"sysdesign/internal/engine"
)

// registerResourceLimits registers search tails, egress, bandwidth, GC, disks, heavy queries, write skew, indexes, store choice and queues.
func registerResourceLimits() {
	// Search over 16 shards: the slowest shard sets the latency. Config only: ask for fewer answers, or ask again.
	searchTail := func(fixed bool) engine.Diagram {
		app := P{"threads": 32.0, "gather": "all"}
		if fixed {
			app = P{"threads": 32.0, "gather": "firstK", "k": 12.0}
		}
		b := (&bld{}).n("users", engine.KClient, cols[0], 200, P{"rps": 60.0, "readPct": 100.0, "timeoutMs": 2000.0}).
			n("app", engine.KAppServer, cols[1], 200, app).e("users", "app")
		for k := 0; k < 16; k++ {
			id := fmt.Sprintf("shard%d", k+1)
			b.n(id, engine.KIndex, cols[2], float64(k)*80, P{"queryMs": 30.0, "variability": "heavy", "connections": 16.0}).label(id, fmt.Sprintf("Search Shard %d", k+1)).e("app", id)
		}
		return b.done()
	}
	register(&Scenario{
		ID: "search-tail", Title: "Search at 16 shards", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Every search waits for the slowest of 16 shards.",
		Story:   "Search is {{sharding|split over 16 shards}} and the app server asks all of them and waits for every answer. Each shard answers in about 30 ms, but some answers take far longer, and {{tail-latency-amplification|with 16 shards nearly every search meets one of them}}. The architecture is fixed; tune the fan-out.",
		Diagram: searchTail(false), Solution: searchTail(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(300), MaxCostPerHour: f(1.7), StarCostPerHour: f(1.65), StarP95Ms: f(200)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the latency distribution: the {{p50|median}} is fine for one shard, so why is the {{p95}} so far from it?",
			"A request that waits for all 16 answers is as slow as the slowest of 16. Do you really need all of them?",
			"Does a request really need every shard to answer? What are the two ways to stop waiting for the slowest one?",
		},
		Answer: "The answer: set the app server's fan-out to answer on the first K shards (try 12 of 16), or hedge: ask a slow shard again. More threads will not help.",
		Predictions: []Prediction{{ID: "p1", Question: "Adding more app-server threads will…", Options: []string{"Cut the p95 a lot, as requests get served sooner", "Barely change it: the wait is on the shards", "Make each shard answer faster than before"}, Answer: 1,
			Explain: "The threads are not the bottleneck: each request is waiting for the slowest of 16 shards. Fewer required answers or a second try attacks that wait."}},
		Debrief: "{{tail-latency-amplification|Waiting for all N answers makes latency the maximum of N draws}}, so tail latency grows with fan-out. Answering on the first K, hedging slow calls, or allowing partial results trade completeness or extra load for a much shorter tail.",
		Deeper:  "{{tail-latency-amplification|When a request waits for all N shards its latency is the maximum of N draws}}, and the chance that at least one shard is slow grows quickly with N, so the tail gets worse as you add shards even if every shard is healthy. Answering on the first K trades completeness for latency, so the response must say it is partial and you must know how much relevance you give up; hedging a slow shard to a replica trades extra load for a shorter tail but only works when the slowness is independent between copies. At larger scale you limit fan-out with routing or tiered indexes and keep replicas per shard so a hedge has somewhere to go. An interviewer will ask how you choose K and how you spot a shard that is slow every time, which calls for ejecting it rather than hedging around it. The failure pattern is hedging during overload: the extra requests make every shard slower and the tail gets worse.",
	})

	// A celebrity's post is pushed into millions of timelines. Config only: stop pushing for the few huge accounts.
	celebrity := func(fixed bool) engine.Diagram {
		app := P{"threads": 32.0, "queueCap": 500.0}
		if fixed {
			app["feedStrategy"] = "hybrid"
			app["celebrityThreshold"] = 2000.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 40.0, "readPct": 70.0, "workload": "feed", "keySpace": 5000.0, "followersMean": 50.0, "followersSkew": 1.2, "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, app).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "queueCap": 500.0}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "celebrity-post", Title: "The celebrity post", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Most posts are fine, but a few accounts make the whole feed stall.",
		Story:   "Every post is copied into each follower's timeline when it is written, so reads are fast. Most accounts have a few dozen followers, but {{hot-key|a handful have tens of thousands}}, and every time one of them posts the database is buried under timeline inserts. The architecture is fixed; change how the feed is built.",
		Diagram: celebrity(false), Solution: celebrity(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(300), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.05), StarP95Ms: f(250)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at the failures and the {{p95}}: they come in bursts. Who is posting when it happens?",
			"Pushing a post costs one insert per follower. For an account with 30,000 followers that is 30,000 inserts for a single post.",
			"Could ordinary accounts and celebrities be handled differently? Which side of the work should each kind pay for?",
		},
		Answer: "The answer: set the app server's feed strategy to hybrid: push for ordinary accounts, pull celebrities when a timeline is read. More connections or threads only cost money.",
		Predictions: []Prediction{{ID: "p1", Question: "Where does the work go if celebrities are pulled instead of pushed?", Options: []string{"Nowhere: the posts are simply never delivered to anyone", "To readers: each timeline read fetches a few celebrity posts", "To the author, who waits much longer for the post to publish"}, Answer: 1,
			Explain: "Fan-out on write spends effort once per follower; fan-out on read spends it once per reader. A hybrid pays the cheap side for each kind of account."}},
		Debrief: "Push makes reads cheap and writes proportional to the audience; pull makes writes cheap and reads proportional to who you follow. With heavy skew a hybrid wins: push for the many small accounts and pull {{hot-key|the few giants}}.",
	})

	// Image downloads billed per GB: every view pays egress unless a CDN answers it
	imageFeed := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 100.0, "respKB": 300.0, "keySpace": 2000.0, "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[2], rowMid, P{"threads": 32.0}).
			n("db", engine.KDatabase, cols[3], rowTop, nil).
			n("files", engine.KObjectStore, cols[3], rowBot, nil).label("files", "Image store").
			e("app", "db").e("app", "files")
		if fixed {
			b.n("cdn", engine.KCDN, cols[1], rowMid, P{"capacityItems": 5000.0, "ttlSec": 300.0}).e("users", "cdn").e("cdn", "app")
		} else {
			b.e("users", "app")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "image-egress", Title: "The image feed bill", Difficulty: "Medium",
		Summary: "Everything works, but the cloud bill is shocking.",
		Story:   "An image feed serves 100 views a second, each a 300 KB picture fetched from the image store, which charges for every GB that leaves it. The pages are fast and nothing fails, but finance wants the hourly bill under $2.50.",
		Diagram: imageFeed(false), Solution: imageFeed(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(2.5), StarCostPerHour: f(1.5)},
		DurationSec: 60, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at the cost, not the latency: 100 views a second x 300 KB is 30 MB every second.",
			"Which component can answer a popular picture without going back to the image store?",
			"What can answer a popular picture without going back to the store, and how many distinct pictures does it need room for?",
		},
		Answer: "The answer: put a CDN in front of the app server and give it room for the popular pictures (there are about 2,000 distinct ones).",
		Predictions: []Prediction{{ID: "p1", Question: "A CDN that can only hold a quarter of the pictures will…", Options: []string{"Cut the bill to almost nothing, as the CDN is free", "Cut it somewhat: popular pictures hit, the rest still pay", "Make no difference, since a quarter is too few"}, Answer: 1,
			Explain: "Views are skewed, so a small cache catches the popular ones, but every miss still downloads from the store and pays for it."}},
		Debrief: "Egress is billed per byte, so the cheapest download is the one that never reaches the origin. A CDN {{cache-hit-rate|sized to the popular set}} turns a dominant cost into a rounding error; one that is too small saves some and still pays for the rest.",
	})

	// Video uploads of 10 MB at 6 a second need 480 Mbit/s on every hop: the link to the store is sized for far less
	videoUpload := func(fixed bool) engine.Diagram {
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 6.0, "readPct": 0.0, "reqKB": 10000.0, "timeoutMs": 4000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("db", engine.KDatabase, cols[2], rowTop, nil).label("db", "Video catalogue").
			n("files", engine.KObjectStore, cols[2], rowBot, nil).label("files", "Video store").
			e("users", "app").e("app", "db").e("app", "files")
		for k := range b.d.Edges {
			if b.d.Edges[k].ID == "app>files" {
				b.d.Edges[k].BandwidthMbps = 200
				if fixed {
					b.d.Edges[k].BandwidthMbps = 1000
				}
			}
		}
		return b.done()
	}
	register(&Scenario{
		ID: "video-upload", Title: "Video uploads", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Short uploads work, but videos time out.",
		Story:   "Creators upload clips of about 10 MB, six a second at the evening peak. The app server streams each one to the video store over a 200 Mbit/s link. In testing with tiny files everything was fine; at the peak most uploads time out after 4 seconds.",
		Diagram: videoUpload(false), Solution: videoUpload(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(1500), MaxCostPerHour: f(1.3), StarCostPerHour: f(1.1), StarP95Ms: f(1000)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Do the arithmetic: 6 uploads a second x 10 MB is how many Mbit/s? Compare it with the link.",
			"6 x 10 MB x 8 bits = 480 Mbit/s, and the link to the store carries 200. Uploads queue behind each other on it until they time out.",
			"Total the bandwidth the peak uploads need, then find the connection that carries all of it. Is that link big enough?",
		},
		Answer: "The answer: select the connection from the app server to the video store and raise its bandwidth above what the peak needs (1,000 Mbit/s leaves room).",
		Predictions: []Prediction{{ID: "p1", Question: "More app-server threads will…", Options: []string{"Fix it, since more uploads then run in parallel", "Not help: they all share the same saturated link", "Lower the bandwidth each upload needs, as work is shared"}, Answer: 1,
			Explain: "Parallel uploads still squeeze through one link. When a link is the limit, adding workers only adds more bytes waiting for it."}},
		Debrief: "Bandwidth is a capacity like any other: {{throughput|bytes per second in must be less than the link can carry}}. Estimate peak QPS x object size x 8 before choosing the pipe, and keep big bodies off paths that are sized for small ones.",
	})

	// GC storm: long stop-the-world pauses on every app server. Config only: tune the pauses, do not paper over them.
	gcStorm := func(fixed bool) engine.Diagram {
		pause := 1500.0
		if fixed {
			pause = 200.0
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 120.0, "readPct": 90.0, "timeoutMs": 800.0, "retries": 0.0}).
			n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).
			n("db", engine.KDatabase, cols[3], rowMid, nil).
			e("users", "lb")
		for k := 0; k < 3; k++ {
			id := fmt.Sprintf("app%d", k+1)
			b.n(id, engine.KAppServer, cols[2], float64(k)*110+60, P{"threads": 16.0, "gcPauseMs": pause, "gcEverySec": 8.0}).e("lb", id).e(id, "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "gc-storm", Title: "The GC storm", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Averages look fine, yet one request in seven fails.",
		Story:   "Each app server freezes for a second and a half every eight seconds or so (a stop-the-world garbage collection after a heap change). During a pause nothing on that server answers, and the clients give up after 800 ms. The {{p50|median latency}} is healthy, which is why nobody noticed.",
		Diagram: gcStorm(false), Solution: gcStorm(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.1), StarP95Ms: f(200)},
		DurationSec: 60, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at the failures: they are timeouts, in waves, and the latency of the requests that do succeed is fine. What freezes a whole server for a second?",
			"The failure breakdown and the events on the chart show GC pauses. Retries or more servers hide some of the damage but cost latency or money.",
			"The pause comes from garbage collection. Which advanced server settings change how long a pause lasts, compared with how long a client waits?",
		},
		Answer: "The answer: open an app server's advanced settings: shorten the GC pause (a smaller heap or a different collector). A pause shorter than the client's patience costs nothing.",
		Predictions: []Prediction{{ID: "p1", Question: "Letting clients retry twice will…", Options: []string{"Fix it completely, since a retry never meets a pause", "Recover most requests, but make the slow ones very slow", "Make it worse, because retries double the load"}, Answer: 1,
			Explain: "A retry reaches a healthy server, so more requests succeed, but each rescued request has already waited out its timeout: the tail gets worse."}},
		Debrief: "A stop-the-world pause is invisible in averages and brutal at the tail. Retries and extra servers mask it at a cost; the real fix is a shorter pause. Watch {{p99}} and the timeouts, not the mean.",
	})

	// Disk full of writes: every secondary index is another disk write per row, against a disk with a fixed IOPS budget
	diskWrites := func(fixed bool) engine.Diagram {
		idx := 3.0
		if fixed {
			idx = 0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 80.0, "readPct": 20.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 32.0, "diskIops": 200.0, "indexes": idx}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "disk-writes", Title: "The disk full of writes", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The database is mostly idle on paper, yet writes queue and time out.",
		Story:   "A write-heavy service (80 requests a second, mostly writes) runs on a database whose disk does 200 operations a second. Years ago someone added three secondary indexes for a report nobody opens any more. CPU and connections look fine, but writes pile up and time out.",
		Diagram: diskWrites(false), Solution: diskWrites(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(1.5), StarCostPerHour: f(1.3), StarP95Ms: f(200)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Count the disk operations: each write needs one for the row and one more for every secondary index. How many writes a second is that, against 200?",
			"64 writes a second x (1 + 3 indexes) is about 256 operations a second against a disk that does 200, so the queue never drains.",
			"Count the disk operations one write costs. Which of the things maintained on every write does nobody read from?",
		},
		Answer: "The answer: nobody queries by those fields (the lookups share is zero), so drop the indexes. A faster disk would also work, but removing the waste is cheaper.",
		Predictions: []Prediction{{ID: "p1", Question: "Removing the three unused indexes will…", Options: []string{"Slow reads down a lot, since lookups lose their index", "Cut the disk operations per write from four to one", "Free memory, but leave the disk work as it was"}, Answer: 1,
			Explain: "Each write must update every index as well as the table. With no reads using them, they only cost disk writes."}},
		Debrief: "Every index is paid for on every write. Check that an index is used before keeping it, and compare the write rate times the operations per write with the disk's budget.",
	})

	// A few expensive requests hold every thread: head-of-line blocking. Config only: give the server more threads.
	oneHeavy := func(fixed bool) engine.Diagram {
		threads := 16.0
		if fixed {
			threads = 64
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 250.0, "readPct": 100.0, "heavyPct": 5.0, "heavyMult": 40.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": threads, "cpuMs": 20.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 64.0, "readMs": 2.0}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "one-heavy-query", Title: "One heavy query", Difficulty: "Medium", ConfigOnly: true,
		Summary: "One request in twenty is expensive, and everyone suffers.",
		Story:   "A search page answers in 20 ms, except that one request in twenty runs a report that takes 800 ms of server time. The server has {{thread-pool|16 threads}}. Most requests are light, yet the whole site goes slow in waves and some time out.",
		Diagram: oneHeavy(false), Solution: oneHeavy(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(4), StarCostPerHour: f(3.5), StarP95Ms: f(200)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Work out the {{thread-pool|threads}}' budget: the heavy requests are 5% of 250 a second, each holding a thread for 800 ms. How many threads is that on average?",
			"{{littles-law|12 requests a second x 0.8 s is about 10 threads busy}} with heavy work alone, out of 16, so the 238 light requests a second have almost nothing left and queue behind them.",
			"How many threads do the heavy requests tie up at once, and how many must stay free for the light ones?",
		},
		Answer: "The answer: give the server enough threads that the heavy requests cannot take them all. The inspector shows {{p95}} for light and heavy requests separately.",
		Predictions: []Prediction{{ID: "p1", Question: "The heavy requests are 5% of traffic. Their effect on the light ones is…", Options: []string{"About 5% slower, in line with their share of traffic", "Far worse: they hold threads the light ones need", "None at all, since they are separate requests"}, Answer: 1,
			Explain: "Every request shares the same threads. A few long requests occupy a large share of thread-time, so the quick ones wait behind them."}},
		Debrief: "{{throughput|Throughput}} is not the only budget: a small share of expensive requests can take most of the worker time and make the cheap majority slow. Look at latency by request type, and {{bulkhead|keep heavy work from sharing the pool the light work needs}}.",
	})

	// Write skew: each doctor leaves after checking the other is still on call. Config only: raise the isolation level.
	onCall := func(fixed bool) engine.Diagram {
		iso := "readCommitted"
		if fixed {
			iso = "serializable"
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 30.0, "workload": "oncall", "keySpace": 40.0, "keyDist": "uniform", "timeoutMs": 3000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "isolation": iso}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "nobody-on-call", Title: "Nobody on call", Difficulty: "Hard", ConfigOnly: true,
		Summary: "Every rule is checked, and the rule still breaks.",
		Story:   "A rota has pairs of doctors and the rule is that at least one of each pair stays on call. A doctor going off call first checks that the partner is still on. Nobody ever writes the partner's row, yet some nights both doctors in a pair are off at once.",
		Diagram: onCall(false), Solution: onCall(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxWriteSkew: i(0), MaxP95Ms: f(400), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.05), StarP95Ms: f(250)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Two requests for one pair overlap: each reads the pair before the other has written. What does the database promise about that?",
			"Moving to repeatable read (snapshot isolation) catches two writers of the SAME row, but these write different rows. Look at the numbers after you try it.",
			"Which isolation level makes two requests that read the same pair take turns, even when they write different rows, and what does it cost in aborts?",
		},
		Answer: "The answer: only serializable makes the two requests take turns on the pair. Set the database's isolation level to serializable and check the aborts it brings.",
		Predictions: []Prediction{{ID: "p1", Question: "Will repeatable read (snapshot isolation) stop both doctors leaving?", Options: []string{"Yes: snapshot isolation aborts anything that conflicts", "No: they write different rows, so nothing clashes", "Only when load is low and few requests overlap"}, Answer: 1,
			Explain: "Snapshot isolation only detects two writes to the same row. Write skew is two writes to different rows justified by the same stale read, which only serializable execution rules out."}},
		Debrief: "Write skew is the anomaly snapshot isolation misses: a rule that spans several rows, checked by reading and enforced by writing one of them. Either serialize those transactions, lock the rows the rule reads, or make the rule a constraint the database enforces.",
		Deeper:  "Write skew happens when two transactions read an overlapping set of rows, each decides it is safe, and each writes a different row, so snapshot isolation sees no conflict even though the rule across the rows is broken. You have three fixes: run the transactions serializably and accept aborts that the client must retry, lock the rows the rule reads, or turn the rule into something the database enforces, for example by making both writers touch one row that represents the pair. At larger scale the rule gets harder when the rows live on {{sharding|different shards}}, which is a reason to keep an invariant inside one partition. An interviewer will ask for other examples such as booking the same room or spending from two accounts, and what your database really means by repeatable read, because the name does not guarantee the same behaviour everywhere. The failure is silent: nothing errors and the broken invariant is noticed by a person, as in this story.",
	})

	// The slow query: most reads look an item up by a field other than its key, and there is no index for it
	slowQuery := func(fixed bool) engine.Diagram {
		idx := 0.0
		if fixed {
			idx = 1
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 150.0, "readPct": 90.0, "secondaryPct": 60.0, "timeoutMs": 2000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "indexes": idx}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "slow-query", Title: "The slow query", Difficulty: "Easy", ConfigOnly: true,
		Summary: "Most reads search by email, and the database reads every row to find it.",
		Story:   "Sixty percent of reads look an account up by its email address, not its id. The table has no index on email, so each of those reads scans the table and takes twenty times as long as a lookup by id. At the lunchtime peak {{backpressure|the database queue fills and requests are refused}}.",
		Diagram: slowQuery(false), Solution: slowQuery(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(300), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.05), StarP95Ms: f(200)},
		DurationSec: 30, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"{{utilisation|The database is busy}} but not because of how many requests there are: what is each lookup by email doing?",
			"Without an index on the field being searched, the database reads the whole table (see 'lookups by another field' on the users).",
			"The database can keep one extra structure that costs a little on every write. Is that price worth paying for this mix of reads and writes?",
		},
		Answer: "The answer: give the database a secondary index. It costs a little on every write, but this service is 90% reads.",
		Predictions: []Prediction{{ID: "p1", Question: "An index on email makes writes…", Options: []string{"Faster, because every write finds its row more quickly", "Slightly slower, as the index must be updated too", "Unchanged, as an index only helps queries, not writes"}, Answer: 1,
			Explain: "Every write also updates each index, so a read speed-up is paid for with a small write cost. Worth it when reads dominate."}},
		Debrief: "A lookup on an unindexed field is a full scan. An index trades a little write cost and storage for turning that scan into a seek. Check the access paths your queries really use.",
	})

	// Pick the store: a sensor firehose. The write path of the relational baseline is the bottleneck
	pickStore := func(fixed bool) engine.Diagram {
		model := "relational"
		if fixed {
			model = "timeSeries"
		}
		return (&bld{}).
			n("sensors", engine.KClient, cols[0], rowMid, P{"rps": 700.0, "readPct": 0.0, "timeoutMs": 1500.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 64.0, "queueCap": 500.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "model": model, "queueCap": 500.0}).
			e("sensors", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "pick-the-store", Title: "Pick the store", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Seven hundred sensor readings a second overwhelm a general-purpose database.",
		Story:   "Sensors send 700 readings a second and nothing ever reads them back in real time. The readings go into a relational database that spends the same effort on each insert as on an order, and {{backpressure|a quarter of them are refused when its queue fills}}.",
		Diagram: pickStore(false), Solution: pickStore(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(3.0), StarCostPerHour: f(2.6), StarP95Ms: f(200)},
		DurationSec: 30, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"The workload is almost all appends and almost no reads. Is a general-purpose relational layout the right shape for it?",
			"More database connections do not help here: {{connection-pool|the pool each app server may open to the database}} is the limit.",
			"What is it about the data (what is written, and how it is read) that points to a different kind of storage engine, and where is the data model chosen?",
		},
		Answer: "The answer: change the database's data model (advanced settings) to one built for append-heavy writes: time-series, or wide-column.",
		Predictions: []Prediction{{ID: "p1", Question: "What makes a time-series store cheaper per insert?", Options: []string{"It skips durability, so nothing is written to disk", "Append-optimised storage avoids updating in place", "It keeps everything in memory instead of on disk"}, Answer: 1,
			Explain: "Appending to the end of a log-structured layout avoids random in-place updates, so each write costs a fraction of a general-purpose engine's."}},
		Debrief: "Match the store to the access pattern. Append-heavy, rarely-read data fits an append-optimised layout; paying a general-purpose engine's cost per row for a firehose is how a database becomes the bottleneck.",
	})

	// Poison pill: a malformed job is retried ten times, burning worker time, and then vanishes
	poisonPill := func(fixed bool) engine.Diagram {
		attempts, dlq := 10.0, false
		if fixed {
			attempts, dlq = 3, true
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 50.0, "readPct": 0.0, "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("q", engine.KQueue, cols[2], rowMid, P{"capacity": 20000.0, "maxAttempts": attempts, "deadLetter": dlq}).
			n("w", engine.KWorker, cols[3], rowMid, P{"concurrency": 8.0, "processMs": 100.0, "poisonPct": 12.0}).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "app").e("app", "q").e("q", "w").e("w", "db").done()
	}
	register(&Scenario{
		ID: "poison-pill", Title: "The poison pill", Difficulty: "Medium", ConfigOnly: true,
		Summary: "One in eight messages can never be processed, and everything else queues behind them.",
		Story:   "A bad deploy upstream means about 12% of the messages are malformed. The queue tries each one ten times, every try occupying a worker for its full processing time, and then silently drops it. Healthy messages wait behind the retries and nobody can find out what was dropped.",
		Diagram: poisonPill(false), Solution: poisonPill(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MinJobsCompleted: f(0.85), MaxJobP95Ms: f(1500), MaxJobsLost: i(0), MaxCostPerHour: f(1.3), StarCostPerHour: f(1.2), StarP95Ms: f(500)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Open the job counters: how many retries are there, and how long do healthy jobs wait?",
			"Retrying something that can never succeed only burns worker time. How many attempts does a job deserve, and where should the failures go instead of vanishing?",
			"Two settings on the queue: how many attempts a job deserves, and where a job goes once it has used them up.",
		},
		Answer: "The answer: on the queue (advanced settings): lower the attempts per job and turn on the {{dead-letter-queue|dead-letter queue}} so the failed jobs are kept for inspection.",
		Predictions: []Prediction{{ID: "p1", Question: "Lowering the attempts alone (no dead-letter queue) will…", Options: []string{"Fix everything, since the bad jobs are then gone", "Free the workers but lose the bad messages silently", "Keep the bad messages, just with fewer attempts each"}, Answer: 1,
			Explain: "Fewer retries stop the waste, but a job that used up its attempts is gone. A dead-letter queue keeps it where someone can look at it."}},
		Debrief: "Retries help against transient failures and hurt against permanent ones. Bound the attempts, back off, and route what still fails to a {{dead-letter-queue|dead-letter queue}} so it is visible and recoverable instead of lost.",
	})

	// Consumer lag: eight consumers, four partitions
	consumerLag := func(fixed bool) engine.Diagram {
		parts := 4.0
		if fixed {
			parts = 16
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 60.0, "readPct": 0.0, "keySpace": 200.0, "keyDist": "uniform", "timeoutMs": 3000.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0}).
			n("log", engine.KLog, cols[2], rowMid, P{"partitions": parts, "capacity": 50000.0}).
			n("db", engine.KDatabase, cols[4], rowMid, nil).
			e("users", "app").e("app", "log")
		for k := 0; k < 8; k++ {
			id := fmt.Sprintf("c%d", k+1)
			b.n(id, engine.KWorker, cols[3], float64(k)*60, P{"concurrency": 2.0, "processMs": 100.0}).label(id, fmt.Sprintf("Consumer %d", k+1)).e("log", id).e(id, "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "consumer-lag", Title: "Consumer lag", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Eight consumers, and the backlog still grows.",
		Story:   "Events stream into a {{sharding|partitioned}} log read by eight consumers, each able to handle several messages at once. Yet the backlog grows all day and processing falls further and further behind. Adding yet more consumers was already tried.",
		Diagram: consumerLag(false), Solution: consumerLag(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MinJobsCompleted: f(0.97), MaxJobP95Ms: f(2000), MaxCostPerHour: f(1.5), StarCostPerHour: f(1.4), StarP95Ms: f(800)},
		DurationSec: 40, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Look at which consumers are busy. Do all eight have work?",
			"A partition is read by one member of a group at a time, and one message after another to keep order. How many partitions are there?",
			"What caps how many consumers can work at once, and where is that number set?",
		},
		Answer: "The answer: raise the number of partitions on the log (16 gives every consumer two). More consumers would not help: they would sit idle.",
		Predictions: []Prediction{{ID: "p1", Question: "With 4 partitions, what do consumers 5 to 8 do?", Options: []string{"Share the load equally with consumers 1 to 4", "Nothing: a partition has one reader per group", "Take over the slowest partitions from the others"}, Answer: 1,
			Explain: "Within a consumer group each partition is assigned to exactly one member, so parallelism is capped by the partition count."}},
		Debrief: "In a partitioned log the number of partitions is the ceiling on consumer parallelism. Choose it for the {{throughput|throughput}} you will need, since changing it later also changes how keys map to partitions.",
	})

	// Hot cell: location pings from one busy city all land on the shard that owns that map cell
	hotCell := func(fixed bool) engine.Diagram {
		by := "key"
		if fixed {
			by = "request"
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 500.0, "workload": "geo", "readPct": 0.0, "keySpace": 400.0, "keyDist": "uniform", "hotKeyPct": 70.0, "timeoutMs": 1000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "queueCap": 2000.0, "dbPool": 200.0}).
			n("shards", engine.KShardRouter, cols[2], rowMid, P{"shardBy": by}).
			e("users", "app").e("app", "shards")
		for k := 0; k < 4; k++ {
			id := fmt.Sprintf("r%d", k+1)
			b.n(id, engine.KRedis, cols[3], float64(k)*100+30, P{"scoreMs": 4.0}).e("shards", id)
		}
		return b.done()
	}
	register(&Scenario{
		ID: "hot-cell", Title: "The hot cell", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Four shards, and one of them is on fire.",
		Story:   "A ride-sharing app stores driver locations in {{sharding|four shards}}, split by map cell. Most pings come from the one busy city centre, so the shard that owns that cell takes most of the load while the other three idle. Adding shards was already tried: the {{hot-key|hot cell}} stays on a single one.",
		Diagram: hotCell(false), Solution: hotCell(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(300), MaxCostPerHour: f(1.5), StarCostPerHour: f(1.3), StarP95Ms: f(200)},
		DurationSec: 30, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Compare the four shards in the inspector: is the load even?",
			"Each shard can take about 250 updates a second (4 ms each). The shard that owns the busy cell is asked for roughly three quarters of 500 a second.",
			"The router decides which shard owns a request. What could it use instead of the cell so the busy cell is not tied to one shard, and what would reads then pay?",
		},
		Answer: "The answer: change how the shard router chooses a shard: by the request itself rather than the cell, so the busy cell is spread over all four. (A real system pays for that on reads, which then have to ask every shard.)",
		Predictions: []Prediction{{ID: "p1", Question: "Adding a fifth and sixth shard, still split by cell, will…", Options: []string{"Fix the hot shard, as the cell spreads out", "Leave the hot cell on one shard, so nothing improves", "Make it worse, since the data must move first"}, Answer: 1,
			Explain: "One cell maps to one shard however many shards there are. A hot key cannot be spread by adding more partitions of the same key."}},
		Debrief: "{{sharding|Partitioning}} by a key that is itself skewed concentrates the skew. Spread {{hot-key|hot keys}} (by sender, by a suffix, or by splitting the cell), and remember the price: reads of one key must then query every shard.",
	})
}
