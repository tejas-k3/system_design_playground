package scenarios

import "sysdesign/internal/engine"

// An Alternative is a different single idea a learner might have tried on a challenge's starting design, with what
// the engine measured for it (seed 42). It is shown once the challenge is solved, next to the reference solution.
type Alternative struct {
	Label       string   `json:"label"`
	Changes     []Change `json:"changes"`
	Pass        bool     `json:"pass"`
	SuccessPct  float64  `json:"successPct"`
	P95Ms       float64  `json:"p95Ms"`
	CostPerHour float64  `json:"costPerHour"`
	Why         string   `json:"why"`
}

// Change sets one setting of one node.
type Change struct {
	Node  string `json:"node"`
	Param string `json:"param"`
	Value any    `json:"value"`
}

// Alternatives returns the stored alternatives for a challenge.
func Alternatives(id string) []Alternative { return alternatives[id] }

func applyChanges(d engine.Diagram, cs []Change) engine.Diagram {
	out := engine.Diagram{Nodes: append([]engine.Node(nil), d.Nodes...), Edges: append([]engine.Edge(nil), d.Edges...), Notes: d.Notes}
	for _, c := range cs {
		for i := range out.Nodes {
			if out.Nodes[i].ID != c.Node {
				continue
			}
			p := engine.Params{}
			for k, x := range out.Nodes[i].Params {
				p[k] = x
			}
			p[c.Param] = c.Value
			out.Nodes[i].Params = p
		}
	}
	return out
}

func alt(label string, pass bool, success, p95, cost float64, why string, cs ...Change) Alternative {
	return Alternative{Label: label, Changes: cs, Pass: pass, SuccessPct: success, P95Ms: p95, CostPerHour: cost, Why: why}
}

// The numbers are what the engine measured when they were written; TestAlternativesMatchTheEngine runs every one
// again, so they cannot drift from the simulator.
var alternatives = map[string][]Alternative{
	"gc-storm": {
		alt("Let clients retry twice", false, 99.1, 1013, 1.07, "Retries reach a healthy server, so nearly everything succeeds, but each rescued request has already waited out its timeout: the tail is a second.", Change{"users", "retries", 2.0}),
		alt("Give each server 32 threads", false, 91.6, 167, 2.62, "More threads do nothing for a server that is frozen, and they cost more.", Change{"app1", "threads", 32.0}, Change{"app2", "threads", 32.0}, Change{"app3", "threads", 32.0}),
	},
	"one-heavy-query": {
		alt("Raise the threads from 16 to 20", false, 97.9, 477, 1.45, "A little more room, but the heavy requests still take most of the thread-time; the light ones still queue behind them.", Change{"app", "threads", 20.0}),
		alt("Let clients retry twice", false, 84.0, 955, 1.34, "Retries add load to a server that has no spare threads.", Change{"users", "retries", 2.0}),
	},
	"reconnect-storm": {
		alt("Let clients retry three times", false, 68.3, 3196, 0.74, "Everyone retries together, so the herd arrives again and again.", Change{"users", "retries", 3.0}),
		alt("Spread reconnects over 2 seconds", false, 79.6, 831, 0.74, "Two seconds is still a spike for a server that sets up about 500 connections a second; thirty seconds is a trickle.", Change{"users", "reconnectJitterSec", 2.0}),
	},
	"hot-cell": {
		alt("Give the app server 64 threads", false, 0.0, 0, 2.60, "The wait is on the hot shard, not on the app server; the extra threads just wait there too, and cost more.", Change{"app", "threads", 64.0}),
		alt("Let clients retry twice", false, 0.0, 0, 1.14, "Retries go to the same shard.", Change{"users", "retries", 2.0}),
	},
	"bot-flood": {
		alt("Raise the shared limit to 400 requests a second", false, 58.1, 177, 2.53, "The bots use up the larger allowance too, so real customers are still turned away.", Change{"rl", "limitRps", 400.0}),
	},
	"retry-storm": {
		alt("Allow one immediate retry", false, 16.5, 204, 2.46, "One retry doubles the load on a system that is already struggling, so it never recovers.", Change{"users", "retries", 1.0}),
		alt("Allow no retries", true, 85.5, 200, 2.46, "Without the extra load the system recovers by itself; it passes, but does not protect the customers who got an error.", Change{"users", "retries", 0.0}),
	},
	"zone-outage": {
		alt("Spread only the servers over three zones", false, 56.2, 154, 1.92, "The balancer and the database are still in one zone, so losing it still takes the service down.", Change{"app2", "zone", "az-b"}, Change{"app3", "zone", "az-c"}),
	},
	"restore-drill": {
		alt("Back up every 5 seconds, nothing else", false, 35.7, 164, 0.80, "The data lost drops to 3 seconds, but the restore still takes 51 seconds: backups set the recovery point, not the recovery time.", Change{"db", "backupEverySec", 5.0}),
		alt("Restore at 600 MB/s, no backups", false, 77.9, 167, 0.74, "The database is back in 17 seconds, but with no backups the restore goes back to the very start: 28 seconds of writes lost.", Change{"db", "restoreMBps", 600.0}),
	},
	"lost-cart": {
		alt("Pass writes on in 50 ms instead of 300", false, 100.0, 177, 0.74, "A shorter window means fewer collisions (234 lost instead of 1,038 over the run), but any two writes to one cart inside it still lose one.", Change{"db", "syncLagMs", 50.0}),
		alt("Use a single leader", true, 100.0, 177, 0.74, "No conflicts at all, because every write goes through one place. It passes here; the price (not modelled in this run) is that the far region pays a cross-region round trip for every write.", Change{"db", "topology", "singleLeader"}),
	},
	"quorum-overlap": {
		alt("Wait for two copies on writes only (R=1, W=2)", false, 100.0, 216, 1.30, "1 + 2 is not more than 3 copies, so a read can still land on the one copy the write has not reached: 9 own-write misses.", Change{"kv", "writeQuorum", 2.0}),
		alt("Wait for two copies on reads only (R=2, W=1)", false, 100.0, 161, 1.30, "Closer, but 2 + 1 is still not more than 3: one read in the run still missed a write.", Change{"kv", "readQuorum", 2.0}),
		alt("Write to all three copies (R=1, W=3)", true, 100.0, 321, 1.30, "1 + 3 is more than 3, so it works, but every write waits for the slowest copy (p95 321 ms against 216) and would fail if any copy were down.", Change{"kv", "writeQuorum", 3.0}),
	},
	"lease-expired": {
		alt("Make the lock lease 1 second", true, 97.5, 2238, 2.71, "Passes this run, because every pause (600 ms) is shorter than the lease. It is a bet on how long a pause can be: with 3-second pauses it double books again (6 on this seed), where the conditional write still has none.", Change{"app1", "lockTtlMs", 1000.0}, Change{"app2", "lockTtlMs", 1000.0}, Change{"app3", "lockTtlMs", 1000.0}),
	},
	"add-a-shard": {
		alt("Spread keys over a ring with one point per shard", true, 100.0, 148, 3.79, "Still a ring, so only about a fifth of the keys move; with a single point per shard the split is less even, but the move is still small enough here to pass.", Change{"shards", "hashing", "ring"}, Change{"shards", "vnodes", 1.0}),
	},
	"heavy-tail": {
		alt("Give the app server 96 threads", false, 100.0, 264, 2.91, "The slow reads are slow by themselves, not queued: more threads leave the p95 where it was (264 ms) and cost more.", Change{"app", "threads", 96.0}),
		alt("Hedge after only 5 ms", true, 100.0, 164, 2.12, "It passes and has the best p95, but almost every read gets a second request (6,082 hedges for about 6,000 reads): the replicas carry twice the load for a gain that waiting longer gets most of.", Change{"app", "hedgeReadsMs", 5.0}),
	},
	"noisy-tenant": {
		alt("Double the shared limit to 200 a second", false, 67.2, 151, 1.27, "The quiet tenants do better (64% instead of about 32%) but are still turned away, because the noisy tenant takes the larger share of whatever limit there is.", Change{"rl", "limitRps", 200.0}),
	},
	"notification-fan-out": {
		alt("Give each sender 16 slots instead of 4", false, 100.0, 129, 1.64, "More capacity makes the shared work faster, not wider: each order still reaches only one of the three workers (1.00 deliveries per message) and the bill goes up.", Change{"ch1", "concurrency", 16.0}, Change{"ch2", "concurrency", 16.0}, Change{"ch3", "concurrency", 16.0}),
	},
	"go-global": {
		alt("Give the US app server 96 threads", false, 100.0, 291, 2.48, "The delay is distance, not load: the p95 stays at 291 ms and the bill goes up.", Change{"app-us", "threads", 96.0}),
	},
	"payment-saga": {
		alt("Let clients retry a failed order twice", false, 99.5, 606, 1.00, "Success rate goes up (a retry usually gets a different answer from the card processor) but every failed attempt still leaves its stock reserved: 302 orders half done on this seed, and the retries make the tail twice as slow.", Change{"users", "retries", 2.0}),
	},
	"frozen-checkout": {
		alt("Let clients retry a failed order twice", false, 79.3, 848, 1.00, "More requests get through once the server is back, but the transactions that were mid-flight when it died are still locked: 13 in doubt on this seed, and the retries push the p95 to 848 ms.", Change{"users", "retries", 2.0}),
		alt("Give the app server 96 threads", false, 70.4, 517, 2.46, "Capacity does not change what happens when the coordinator dies: 12 transactions are still in doubt, and the bill goes up.", Change{"app", "threads", 96.0}),
	},
	"who-is-the-leader": {
		alt("Run 4 coordination machines instead of 3", false, 67.7, 174, 1.26, "Four machines need a majority of three, so losing two still leaves no quorum: the same outage, for more money. Even sizes add cost, not tolerance.", Change{"zk", "nodes", 4.0}),
		alt("Let a timer promote the replica instead", true, 92.9, 177, 1.18, "It passes here because the primary really is dead. A timer promotes whatever the cause, so a primary that was only cut off would stay up beside its replacement: two primaries. This run cannot show that risk.", Change{"db", "electionBy", "timer"}),
	},
	"launch-day": {
		alt("Give the database 256 connections", false, 0.0, 0, 7.83, "More connections let more writes wait at once; the row still lets one through every 20 ms. Nothing succeeds and the bill more than doubles.", Change{"db", "connections", 256.0}),
		alt("Split the counter over only 2 rows", false, 0.0, 0, 3.32, "Two rows take 100 writes a second between them; 180 are asked for, so the queue still builds and everything times out.", Change{"app", "hotKeyMode", "split"}, Change{"app", "splitShards", 2.0}),
		alt("Split the counter over 4 rows", false, 100.0, 507, 3.32, "Four rows can just take the load (200 a second), so nearly every order succeeds, but the rows run almost full and the p95 sits at the 500 ms goal: the margin is too thin.", Change{"app", "hotKeyMode", "split"}, Change{"app", "splitShards", 4.0}),
	},
	"count-the-clicks": {
		alt("Wait 25 seconds before publishing each window", false, 100.0, 132, 0.38, "No click is dropped, but every window is published 25 seconds after it ends: far beyond the three seconds the dashboard allows. Completeness bought with freshness.", Change{"clicks", "watermarkDelaySec", 25.0}),
		alt("Allow only 15 seconds of lateness", false, 100.0, 132, 0.38, "Clicks that are 20 seconds late still miss the window for part of the time (550 dropped on this seed instead of 1,788): a partial fix, because lateness has to cover the latest click you want to count.", Change{"clicks", "allowedLatenessSec", 15.0}),
		alt("Make the windows 60 seconds long", false, 100.0, 132, 0.38, "Longer windows catch more late clicks (536 dropped), but the totals are now per minute, not per ten seconds, and the dashboard waits longer for each.", Change{"clicks", "windowSec", 60.0}),
	},
	"unique-visitors": {
		alt("HyperLogLog with 16 bits of precision", false, 100.0, 132, 0.38, "Almost exact (0.04% off), but its 65,536 registers take 64 KB: more than the 16 KB budget, and nearly as much as the exact table. Precision you do not need is memory you pay for.", Change{"visitors", "counting", "hyperLogLog"}, Change{"visitors", "hllPrecision", 16.0}),
		alt("HyperLogLog with 10 bits of precision", true, 100.0, 132, 0.38, "1 KB and about 3% off on this seed: inside both the memory and the 6% limits. A legitimate cheaper answer; a smaller sketch passes on some seeds and not others, so it is not a safe choice.", Change{"visitors", "counting", "hyperLogLog"}, Change{"visitors", "hllPrecision", 10.0}),
	},
	"top-ten": {
		alt("A sketch only 40 counters wide", false, 100.0, 132, 0.38, "Under a kilobyte, but with 200,000 items sharing 80 counters every counter is inflated by thousands of other items and the top ten is mostly wrong (2 of 10 found).", Change{"trending", "topK", "countMin"}, Change{"trending", "sketchWidth", 40.0}, Change{"trending", "sketchDepth", 2.0}),
		alt("A sketch 4,000 counters wide and 4 deep", false, 100.0, 132, 0.38, "Finds the whole top ten, but its 16,000 counters take 63 KB: nearly as much as the exact table and far over the 16 KB budget.", Change{"trending", "topK", "countMin"}, Change{"trending", "sketchWidth", 4000.0}, Change{"trending", "sketchDepth", 4.0}),
		alt("A sketch 300 counters wide and 3 deep", true, 100.0, 132, 0.38, "4 KB, and it finds 9 or 10 of the top ten on every seed tried: a cheaper answer that still passes. The head of a Zipf crowd is so far ahead of the rest that it survives a lot of sharing.", Change{"trending", "topK", "countMin"}, Change{"trending", "sketchWidth", 300.0}, Change{"trending", "sketchDepth", 3.0}),
	},
	"polite-crawler": {
		alt("Let the crawler retry refused fetches twice", false, 80.1, 831, 0.37, "Each refusal is retried straight away, into the same rush: 243 fetches are still refused (429) and the crawler keeps hammering the sites it is meant to be gentle with.", Change{"crawl", "retries", 2.0}),
		alt("Limit the crawler to 3 fetches a second per site", false, 30.4, 7791, 0.37, "Politer than needed: 3 a second is below the 4 a second the crawler averages, so the queue grows for the whole run and most fetches time out.", Change{"crawler", "perHostRps", 3.0}),
		alt("Limit the crawler to 5 fetches a second per site", false, 57.1, 7638, 0.37, "Above the average but only just: the waiting never fully clears between rushes, and on this seed over a third of the fetches time out. A limit is only safe with room over the average load.", Change{"crawler", "perHostRps", 5.0}),
	},
	"slow-first-token": {
		alt("Write 16 answers at once", false, 100.0, 22698, 4.76, "Four times the batch, but one GPU still finishes only about 2.6 requests a second against the 3 that arrive, so the queue keeps growing and the first token waits 17 seconds.", Change{"model", "batchSize", 16.0}),
		alt("Buy a second GPU and keep the batch at 4", false, 55.5, 29362, 7.26, "Two GPUs at a batch of 4 finish about 2.1 requests a second: still below the 3 that arrive, and over the budget.", Change{"model", "gpus", 2.0}),
		alt("Two GPUs with the big batch", false, 100.0, 5348, 7.26, "The first token is fast (285 ms) and the answers stream quickly, but a second GPU is $2.50 an hour the budget does not have.", Change{"model", "gpus", 2.0}, Change{"model", "batchSize", 64.0}, Change{"model", "kvCacheTokens", 64000.0}),
		alt("Write 32 answers at once", true, 100.0, 10695, 4.76, "Enough for the load: the first token takes 1.5 s on this seed, inside the 2.5 s goal, though with less room than a batch of 64.", Change{"model", "batchSize", 32.0}, Change{"model", "kvCacheTokens", 64000.0}),
	},
	"rag-under-load": {
		alt("Examine 448 candidates instead of 512", false, 100.0, 768, 1.40, "A search still takes about 26 ms, so four cores manage about 150 a second against 150 asked for: no room at all, the queue builds and the p95 is three times the goal, for recall you do not need.", Change{"docs", "efSearch", 448.0}),
		alt("Examine only 64 candidates", false, 100.0, 137, 1.40, "Fast, but it finds only 75% of the true neighbours, under the 90% the application needs.", Change{"docs", "efSearch", 64.0}),
		alt("Examine 104 candidates", false, 100.0, 140, 1.40, "89% recall: a fraction under the line. Recall rises about 5 points for every 20 candidates here, so this is one step short.", Change{"docs", "efSearch", 104.0}),
		alt("Run 32 search cores instead of 4", false, 100.0, 174, 2.80, "Capacity is no longer the problem and all the recall is kept, but the cores cost $1.40 an hour more than the whole budget allows.", Change{"docs", "connections", 32.0}),
		alt("Run 8 search cores instead of 4", true, 100.0, 177, 1.60, "Twice the cores finish the 30 ms searches fast enough and keep full recall, for $0.20 an hour more: a legitimate alternative to examining fewer candidates.", Change{"docs", "connections", 8.0}),
	},
	"the-gpu-budget": {
		alt("Buy a second GPU", false, 100.0, 6143, 7.27, "Two GPUs spread the pasted documents and the first word comes after 2.5 s, but the second GPU is $2.50 an hour the budget does not have, and the first token is still slower than with a token limit.", Change{"model", "gpus", 2.0}),
		alt("Count tokens but allow 90,000 a minute", false, 96.7, 15581, 4.77, "Generous enough that most of the huge prompts get through: the GPU still spends seconds reading them and the first token takes 5 s.", Change{"gate", "unit", "tokens"}, Change{"gate", "limitTokensPerMin", 90000.0}),
		alt("Count tokens but allow only 45,000 a minute", false, 74.2, 2374, 4.77, "Protects the first token (0.25 s) but turns away a quarter of all requests, not just the huge ones: below the 78% that must be served.", Change{"gate", "unit", "tokens"}, Change{"gate", "limitTokensPerMin", 45000.0}),
	},
	"shed-the-cheap-traffic": {
		alt("Shed above 500 ms instead of 100", false, 49.6, 1097, 0.56, "The server still works through a queue half a second deep, so answers that are admitted arrive late: the p95 is over a second. The age at which you start shedding is the latency you accept.", Change{"app", "shedAboveQueueMs", 500.0}),
		alt("Cut the queue to 100 requests", false, 49.9, 655, 0.56, "A short queue also stops the pile-up, and most requests are answered, but 100 waiting requests still mean about half a second of waiting: the p95 is 655 ms, over the goal. It limits how many wait, not for how long.", Change{"app", "queueCap", 100.0}),
		alt("Double the threads to 16", false, 100.0, 582, 0.74, "Enough capacity for all 400 a second, so nothing is turned away, but it costs $0.74 an hour against a $0.65 budget and the p95 is still 582 ms.", Change{"app", "threads", 16.0}),
		alt("Shed above 50 ms", true, 50.3, 239, 0.56, "Passes with the lowest p95 of the lot (239 ms): a tighter bound keeps the queue shorter still. The price is that bursts get turned away more eagerly.", Change{"app", "shedAboveQueueMs", 50.0}),
	},
	"url-shortener": {
		alt("Split the counter over 8 rows, nothing else", false, 0.0, 0, 2.40, "The new links no longer queue on one row, but 1,200 reads a second still go straight to a database that answers about 600: nothing succeeds. Fixing the write path leaves the read path.", Change{"app", "hotKeyMode", "split"}, Change{"app", "splitShards", 8.0}),
		alt("Give the database 64 connections", false, 67.6, 1447, 3.32, "More connections let more reads and counter writes wait at once. Still a third of requests fail, the counter row still lets one write through every 20 ms, and the bill is over budget.", Change{"db", "connections", 64.0}),
		alt("Make a write take 10 ms instead of 20", false, 0.0, 0, 2.40, "The counter row now takes 100 writes a second instead of 50, enough for the 70 asked for, but the reads still swamp the database: a faster write does not make the reads cheaper.", Change{"db", "writeMs", 10.0}),
		alt("Split the counter and run the database with 32 connections, no cache", true, 100.0, 225, 2.72, "Enough connections to answer every read at the database. It passes on this seed (and on 7 of the 9 seeds tried; the p95 sits close to the 250 ms goal), but it costs $2.72 against $2.53 for the cache and loses the cache's star.", Change{"app", "hotKeyMode", "split"}, Change{"app", "splitShards", 8.0}, Change{"db", "connections", 32.0}),
	},
	"rate-limiter-showdown": {
		alt("Switch to a token bucket (burst 300)", false, 32.8, 974, 0.88, "A bucket that holds 300 tokens lets a whole second's allowance through in the first moments of a burst, and refills while it is spent: no better than the fixed window at protecting the database (p95 974 ms).", Change{"rl", "algorithm", "tokenBucket"}),
		alt("Switch to a leaky bucket", false, 33.1, 1141, 0.88, "It smooths what it lets out, but it queues the burst at the limiter instead of turning it away, so requests still wait over a second.", Change{"rl", "algorithm", "leakyBucket"}),
		alt("Keep the fixed window but halve the limit to 150", false, 18.8, 606, 0.88, "Twice 150 is 300, so the database is safe, but every ordinary second now turns away half of what it used to admit: 18.8% succeed against the 20% the partner needs.", Change{"rl", "limitRps", 150.0}),
		alt("Use the sliding window counter", true, 26.4, 655, 0.88, "A cheap approximation of the log: it weighs the previous window by how much of it still overlaps, so a burst across the edge is counted against both. Close to the log's numbers, with one counter per window instead of a record per request.", Change{"rl", "algorithm", "slidingWindowCounter"}),
		alt("Token bucket with a burst of only 100", true, 21.0, 408, 0.88, "Passes with the best p95 (408 ms) because it never lets more than 100 through in a rush; the price is the least successful traffic of all, only just over the 20% floor.", Change{"rl", "algorithm", "tokenBucket"}, Change{"rl", "burst", 100.0}),
	},
	"multi-region-reads": {
		alt("Give every app server 96 threads", false, 100.0, 280, 7.25, "The servers were never busy; they wait on the ocean. The p95 stays at 280 ms and the bill more than triples.", Change{"app-us", "threads", 96.0}, Change{"app-eu", "threads", 96.0}, Change{"app-ap", "threads", 96.0}),
		alt("Move the primary database to Europe", false, 100.0, 334, 2.02, "Europe's reads get fast, but now the US and Asia cross an ocean to reach it (Asia to Europe is the longest hop of all): the p95 is worse, 334 ms, not better.", Change{"db", "region", "eu"}),
		alt("Move the Asian app server to the US", false, 100.0, 280, 2.02, "The app server is next to the database now, but the users are an ocean from the app server: the distance is paid on the way in instead of on the way to the data.", Change{"app-ap", "region", "us"}),
	},
	"opening-night": {
		alt("Give the one server 16 threads", false, 67.8, 280, 0.48, "Four times the threads is about 400 requests a second at best, against the 600 the launch brings: better, but a third of the requests are still turned away.", Change{"app1", "threads", 16.0}),
		alt("Give the one server 32 threads", false, 100.0, 151, 1.00, "Enough room for 600 a second, but one big machine costs more than proportionally: $1.00 an hour against the $0.90 budget, where four small servers cost $0.62, and it is still a single point of failure.", Change{"app1", "threads", 32.0}),
		alt("Give the database 64 connections", false, 21.7, 710, 1.09, "The database was not the bottleneck: the single app server's four threads were. Nothing improves and the bill goes up.", Change{"db", "connections", 64.0}),
		alt("Let clients retry twice", false, 21.9, 1187, 0.24, "Retries add load to a server that is already full, so the tail gets longer and nothing more succeeds.", Change{"users", "retries", 2.0}),
	},
	"slow-reads": {
		alt("Give the database 64 connections", false, 69.2, 643, 1.38, "The database could already take the reads in parallel: the app servers are the limit, because each request holds a thread for the whole 40 ms read. More connections change nothing and cost more.", Change{"db", "connections", 64.0}),
		alt("Give each app server 32 threads", false, 80.6, 738, 2.62, "More threads hold more slow reads at once, which helps (81% succeed), but it nearly doubles the bill past the budget and the reads are still 40 ms each.", Change{"app1", "threads", 32.0}, Change{"app2", "threads", 32.0}, Change{"app3", "threads", 32.0}),
		alt("Halve the database read time to 20 ms", false, 95.7, 468, 0.52, "A faster database frees threads sooner and nearly fixes the failures, but every read still pays 20 ms and the p95 (468 ms) is far from the goal; a cache answers the popular items without asking the database at all.", Change{"db", "readMs", 20.0}),
	},
	"slow-query": {
		alt("Give the database 64 connections", false, 100.0, 507, 1.86, "More scans can run side by side, so nothing times out, but each one still reads the whole table: the p95 is 507 ms and the bill is over budget.", Change{"db", "connections", 64.0}),
		alt("Give the app server 64 threads", false, 96.8, 1013, 2.46, "The app server was not the limit; the threads just wait on the database scans. The p95 is over a second and the cost doubles.", Change{"app", "threads", 64.0}),
		alt("Add three indexes instead of one", true, 100.0, 164, 1.00, "Passes with the same result as one index here, because only one field is looked up. The extra two are wasted: every index makes each write 30% slower, which this mostly-read traffic hides.", Change{"db", "indexes", 3.0}),
	},
	"edge-cache": {
		alt("Give each app server 32 threads", false, 100.0, 154, 2.62, "The origin copes if every server is big enough, but it pays for it: $2.62 an hour against the $1.20 budget. A CDN answers the repeated requests for $0.10.", Change{"app1", "threads", 32.0}, Change{"app2", "threads", 32.0}, Change{"app3", "threads", 32.0}),
		alt("Give the database 64 connections", false, 66.1, 408, 1.38, "The servers, not the database, were saturated, so nothing improves and the bill goes up.", Change{"db", "connections", 64.0}),
		alt("Make a database read take 5 ms", false, 75.3, 369, 0.52, "A faster store helps (75% succeed) but every request still travels to the origin, and the servers are still the limit.", Change{"db", "readMs", 5.0}),
	},
	"connection-crunch": {
		alt("Shrink each server's pool to 20 connections", false, 88.2, 244, 5.56, "Six servers at 20 are still 120 connections asked of a database that accepts 64, so the surplus is refused exactly as before. The sum of the pools, not each pool, has to fit under the limit.", Change{"app1", "dbPool", 20.0}, Change{"app2", "dbPool", 20.0}, Change{"app3", "dbPool", 20.0}, Change{"app4", "dbPool", 20.0}, Change{"app5", "dbPool", 20.0}, Change{"app6", "dbPool", 20.0}),
		alt("Raise the database's connection limit to 1000", true, 100.0, 244, 5.56, "Passes: nothing is refused any more. The model does not charge for the memory a thousand connections take on a real database, so this is a bet that the database copes with them.", Change{"db", "maxConnections", 1000.0}),
		alt("Let clients retry twice", false, 97.5, 643, 5.56, "A retry usually finds a free connection, so nearly everything succeeds, but each rescued request has waited and the tail is over the goal (643 ms against 600).", Change{"users", "retries", 2.0}),
	},
	"retry-pileup": {
		alt("Serve the newest requests first", true, 88.3, 369, 0.52, "Passes with the best p95 of any fix: when the queue is long it answers the people still waiting and leaves the oldest, whose callers have already given up, to expire. One setting does most of the work of the reference solution.", Change{"app1", "queueOrder", "lifo"}, Change{"app2", "queueOrder", "lifo"}, Change{"app3", "queueOrder", "lifo"}),
		alt("Stop retrying", false, 75.2, 229, 0.52, "The storm is gone and the system recovers, but every request that hits the slow patch is a failure for good: 75% succeed against the 85% needed.", Change{"users", "retries", 0.0}),
		alt("Back off exponentially, nothing else", false, 18.8, 181, 0.52, "The retries come later, but with four of them each and no limit on the queue, the servers are still busy with requests nobody is waiting for.", Change{"users", "backoff", "exponential"}),
		alt("Give each app server 32 threads", false, 82.6, 181, 2.62, "More threads soak up part of the storm, but a slow database makes each one wait longer: still under 85% and the bill is more than double.", Change{"app1", "threads", 32.0}, Change{"app2", "threads", 32.0}, Change{"app3", "threads", 32.0}),
	},
	"stale-prices": {
		alt("Read by freshness, nothing else", true, 100.0, 158, 0.69, "Passes: reads that need fresh data go to the primary, which also skips the stale cache. The cache and the replica still serve anything that does not, and it costs nothing extra.", Change{"app", "readRouting", "byConsistency"}),
		alt("Send every read to the primary", true, 100.0, 158, 0.69, "Always fresh, and the primary has the room here. It would stop being fine as the reads grew: it gives up everything the cache and the replica were added for.", Change{"app", "readRouting", "primary"}),
		alt("Clear cache entries on writes, nothing else", false, 100.0, 158, 0.69, "The cache is now right, but the replica is two seconds behind and 'nearest' still sends reads to it: 83% of reads are too stale.", Change{"cache", "writePolicy", "invalidate"}),
		alt("Shorten the cache lifetime to 1 second", false, 100.0, 158, 0.69, "Fewer stale answers (31% instead of 89%), but a second is longer than the half a second users tolerate, and the replica is still in the way.", Change{"cache", "ttlSec", 1.0}),
		alt("Cut the replica's lag to 100 ms", false, 100.0, 148, 0.69, "The replica is nearly current, but the cache can still hand out a 30-second-old price: 84% of reads are too stale.", Change{"replica", "replicationLagMs", 100.0}),
	},
	"double-booked": {
		alt("Raise the isolation level to serializable", false, 100.0, 146, 0.98, "The isolation level only governs the transfer workload; two bookings of the same seat are not transactions that conflict, so the same 5 double bookings happen.", Change{"db", "isolation", "serializable"}),
		alt("Give the database only 8 connections", false, 100.0, 151, 0.61, "Fewer concurrent writes make collisions rarer (8 double bookings are still found) but not impossible, and it only works by throttling everything.", Change{"db", "connections", 8.0}),
		alt("Run the app server with a single thread", false, 0.3, 1910, 0.46, "No two bookings can overlap in time, but nothing finishes either: 250 a second through one thread is hopeless.", Change{"app", "threads", 1.0}),
	},
	"charged-twice": {
		alt("Wait 2 seconds before a client retries", true, 100.0, 497, 0.72, "Passes here because the slow reply (about 0.8 seconds) now arrives before the client gives up, so no retry is sent. It works only while the server stays that quick: one slow day brings the duplicates back, where an idempotency key does not.", Change{"users", "timeoutMs", 2000.0}),
		alt("Turn retries off", false, 90.9, 424, 0.72, "No request is repeated, so no charge is, but 9% of the customers whose request was slow now fail and never get their order.", Change{"users", "retries", 0.0}),
		alt("Make the database write 30 ms instead of 130 ms", false, 100.0, 334, 0.72, "A faster write finishes inside the client's timeout almost always, so only 3 duplicates remain, but any one is a customer charged twice.", Change{"db", "writeMs", 30.0}),
		alt("Back off exponentially between retries", false, 100.0, 900, 0.72, "The retries are spaced out, but each one still repeats the charge: 129 duplicates.", Change{"users", "backoff", "exponential"}),
	},
	"disappearing-money": {
		alt("Use repeatable read", false, 79.0, 248, 0.48, "No more lost updates: the second of two conflicting transfers is aborted instead. But about one transfer in five is aborted, below the 97% needed.", Change{"db", "isolation", "repeatableRead"}),
		alt("Repeatable read, and let clients retry twice", false, 98.4, 783, 0.48, "Retrying the aborted transfers brings the success rate up, but each retry waits and the p95 (783 ms) misses the 600 ms goal. Serializable takes a lock and waits instead of aborting.", Change{"db", "isolation", "repeatableRead"}, Change{"users", "retries", 2.0}),
		alt("Turn on row locks", false, 100.0, 258, 0.48, "Row locks serialise single-row writes like a booking; a transfer reads two rows and writes them back, so the lost updates (470) are untouched.", Change{"db", "rowLocks", true}),
	},
	"video-upload": {
		alt("Give the app server 64 threads", false, 4.9, 3744, 2.50, "The uploads share one saturated link; more threads only add more uploads to the same link.", Change{"app", "threads", 64.0}),
	},
}
