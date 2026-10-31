package scenarios

import (
	"sysdesign/internal/engine"
)

// registerServicePatterns registers crawlers, LLM serving, load shedding, URL shortener and rate limiting.
func registerServicePatterns() {
	// Polite crawler: the sites it fetches from refuse a crawler that comes in rushes
	politeCrawler := func(fixed bool) engine.Diagram {
		app := P{"threads": 16.0, "queueCap": 500.0, "hostChoice": "byKey"}
		if fixed {
			app["perHostRps"] = 8.0
		}
		b := (&bld{}).
			n("crawl", engine.KClient, cols[0], rowMid, P{"rps": 12.0, "readPct": 100.0, "arrival": "bursty", "burstFactor": 4.0, "burstSec": 2.0, "keySpace": 3000.0, "keyDist": "uniform", "timeoutMs": 8000.0, "retries": 0.0}).
			n("crawler", engine.KAppServer, cols[1], rowMid, app).
			n("pages", engine.KDatabase, cols[3], rowBot, P{"connections": 8.0}).label("pages", "Page store").
			e("crawl", "crawler").e("crawler", "pages")
		for k, h := range []string{"s1", "s2", "s3"} {
			b.n(h, engine.KExternalAPI, cols[2], float64(k)*110+40, P{"latencyMs": 100.0, "errorPct": 0.0, "calledFor": "all", "politeRps": 8.0}).label(h, "Site "+h[1:]).e("crawler", h)
		}
		return b.done()
	}
	register(&Scenario{
		ID: "polite-crawler", Title: "The polite crawler", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The sites keep answering 429 to the crawler, and it is about to be blocked.",
		Story:   "A crawler fetches pages from three sites. Its work arrives in rushes, so for a couple of seconds at a time it asks one site for 16 pages a second, while each site says it will serve a crawler at most 8 a second and answers {{load-shedding|429 (too many requests)}} beyond that. Over a whole minute the crawler is nowhere near the limit, but the sites do not average: a crawler that keeps getting 429 gets blocked for good.",
		Diagram: politeCrawler(false), Solution: politeCrawler(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(7000), MaxCostPerHour: f(1.5), StarP95Ms: f(100)},
		DurationSec: 120, WarmupSec: 10, Speed: 1,
		Hints: [3]string{
			"Look at the failure reasons: who is refusing the crawler, and when does it happen?",
			"The sites count calls per second, not per minute. During a rush the crawler asks for twice what a site allows.",
			"Where could a ceiling on calls to one site be enforced, so that the excess waits instead of being refused? Look at the advanced settings of the crawling server.",
		},
		Answer: "The answer: give the crawler server a limit on calls per second to any one site (its 'Calls per second to one service' setting, under Show advanced), equal to what the sites allow. The excess waits its turn instead of being refused.",
		Predictions: []Prediction{{ID: "p1", Question: "Limiting the crawler to 8 calls a second per site (what the sites allow) will…", Options: []string{"Make it slower on average, since it cannot exceed 8", "Leave its speed unchanged, since the average is under 8", "Make each fetch faster, as sites then reply quicker"}, Answer: 0,
			Explain: "A limit is a ceiling: a rush that used to be refused now queues. Nothing is lost, but pages wait their turn, and a limit below the average load would make the queue grow without end."}},
		Debrief: "Politeness is a per-host rate limit on the crawler itself, usually with {{bulkhead|a queue per host}} so that waiting for one slow or strict site does not block the others. The average rate is what you plan capacity with, but the peak rate decides whether you get blocked.",
	})

	// Slow first token: a chat service on one GPU whose batch is far too small for the traffic
	slowFirstToken := func(fixed bool) engine.Diagram {
		llm := P{"gpus": 1.0, "batchSize": 4.0, "kvCacheTokens": 32000.0, "promptTokens": 500.0, "outputTokens": 150.0, "queueCap": 400.0}
		if fixed {
			llm["batchSize"] = 64.0
			llm["kvCacheTokens"] = 64000.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 3.0, "readPct": 0.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 30000.0, "retries": 0.0}).
			n("chat", engine.KAppServer, cols[1], rowMid, P{"threads": 64.0, "queueCap": 2000.0}).
			n("model", engine.KLLMServer, cols[2], rowMid, llm).label("model", "Chat model").
			e("users", "chat").e("chat", "model").done()
	}
	register(&Scenario{
		ID: "slow-first-token", Title: "Slow first token", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Users stare at a blank chat for seconds before the first word appears.",
		Story:   "A chat assistant runs on one GPU. Users do not mind the answer streaming in word by word, but they stare at a blank screen for many seconds before the first word appears: the time to first token is the whole problem. The GPU takes only four answers at a time, and three new requests arrive every second, each needing about one second of GPU time at that batch size. The budget is about six dollars an hour in all, which pays for one GPU, not two.",
		Diagram: slowFirstToken(false), Solution: slowFirstToken(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxTTFTMs: f(2500), MaxCostPerHour: f(6.0), StarCostPerHour: f(5.5), StarP95Ms: f(100)},
		DurationSec: 120, WarmupSec: 20, Speed: 1,
		Hints: [3]string{
			"Look at the model's queue and at how many answers it writes at once. Is the GPU keeping up with the requests?",
			"At four answers at a time one GPU finishes about one request a second; three arrive every second. A GPU writes a whole batch of answers in about the time it takes to write one, so a bigger batch means far more answers per second.",
			"Which setting lets one GPU work on several answers in the same pass, and what else limits how many fit at once?",
		},
		Answer: "The answer: raise the number of answers at once: 32 or 64 is enough. Every answer in progress holds its prompt and answer tokens in the KV cache (about 650 each), so the cache caps the batch at roughly 49 whatever you ask for; raising the cache lifts that cap. Buying a second GPU would work but breaks the budget.",
		Predictions: []Prediction{{ID: "p1", Question: "Writing 64 answers at once instead of 4 makes each answer…", Options: []string{"Sixteen times slower, since the GPU is shared sixteen ways", "Somewhat slower, but many more answers per second", "Just as fast, as GPUs run batches in parallel for free"}, Answer: 1,
			Explain: "Each decode step is a little longer with more answers sharing it, but a step produces a token for every answer in the batch. Batching trades a little speed per answer for a lot more answers per second."}},
		Debrief: "A GPU's {{throughput|throughput}} comes from batching: decoding is limited by reading the model, which is shared by every answer in the batch. The limits are the KV cache (memory for every answer in progress) and the per-answer slowdown. Time to first token is mostly queueing, so fix the throughput, not the prompt reading.",
	})

	// RAG under load: document search that finds everything but cannot keep up
	ragSearch := func(fixed bool) engine.Diagram {
		ef := 512.0
		if fixed {
			ef = 160
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 150.0, "readPct": 100.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 3000.0, "retries": 0.0}).
			n("api", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0, "queueCap": 500.0}).
			n("docs", engine.KVectorStore, cols[2], rowMid, P{"vectors": 20.0, "dims": 768.0, "efSearch": ef, "connections": 4.0, "queueCap": 300.0}).label("docs", "Document search").
			e("users", "api").e("api", "docs").done()
	}
	register(&Scenario{
		ID: "rag-under-load", Title: "Search under load", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The document search finds everything, and answers nobody in time.",
		Story:   "An assistant looks up documents in a vector index before it answers: twenty million passages, searched approximately. Whoever set it up asked for the best recall they could get, so every search examines hundreds of candidates and finds nearly all the true neighbours. At 150 searches a second the four search cores cannot keep up, the queue grows and users time out. Retrieval should still find at least nine of every ten true neighbours.",
		Diagram: ragSearch(false), Solution: ragSearch(true),
		Goals:       engine.Goals{MinSuccessRate: 0.99, MaxP95Ms: f(250), MinRecall: f(0.9), MaxCostPerHour: f(2.0), StarP95Ms: f(100)},
		DurationSec: 60, WarmupSec: 10, Speed: 1,
		Hints: [3]string{
			"Look at the search component: how long does one search take, and how many can it run at once?",
			"{{littles-law|Four searches at once, each taking about 30 ms, is about 135 a second}}: less than 150. A search takes a fixed time plus time for every candidate it examines.",
			"Search time has a fixed part and a part for each candidate examined. Which setting moves the second part, and how far can it go before recall drops under the line?",
		},
		Answer: "The answer: examine fewer candidates. Recall falls as you do (it is about 94% at 128 and 75% at 64), so stay above the 90% line. More search cores would also work, but they cost money and the budget is tight.",
		Predictions: []Prediction{{ID: "p1", Question: "Halving the candidates examined will…", Options: []string{"Halve the recall, as it halves the candidates", "Cost a little recall and save a lot of time", "Make no difference, since search time is fixed"}, Answer: 1,
			Explain: "Recall climbs steeply at first and then flattens: the last candidates add little. Time grows steadily with every candidate, so the extra recall at the top is the most expensive part."}},
		Debrief: "Approximate nearest-neighbour search is a dial between recall and latency, and the top of the dial is steep: 99% recall costs far more than 94%. Pick the recall the application needs, not the maximum, and spend the saved time on capacity.",
	})

	// The GPU budget: a few huge prompts stall the one GPU the budget pays for
	gpuBudget := func(fixed bool) engine.Diagram {
		lim := P{"limitRps": 1000.0, "burst": 1000.0}
		if fixed {
			lim = P{"unit": "tokens", "limitTokensPerMin": 56000.0}
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 2.0, "readPct": 0.0, "keySpace": 10000.0, "keyDist": "uniform", "timeoutMs": 60000.0, "retries": 0.0, "requestTokens": 500.0, "bigRequestPct": 10.0}).
			n("gate", engine.KRateLimiter, cols[1], rowMid, lim).label("gate", "Token limiter").
			n("chat", engine.KAppServer, cols[2], rowMid, P{"threads": 64.0, "queueCap": 2000.0}).
			n("model", engine.KLLMServer, cols[3], rowMid, P{"gpus": 1.0, "batchSize": 32.0, "kvCacheTokens": 200000.0, "outputTokens": 60.0, "queueCap": 400.0}).label("model", "Chat model").
			e("users", "gate").e("gate", "chat").e("chat", "model").done()
	}
	register(&Scenario{
		ID: "the-gpu-budget", Title: "The GPU budget", Difficulty: "Hard", ConfigOnly: true,
		Summary: "One user in ten pastes a book, and everyone else's first word takes seconds.",
		Story:   "A chat assistant runs on the one GPU the budget pays for. Most prompts are short, but one in ten carries twenty times as many tokens (someone pasted a whole document), and reading ten thousand tokens takes the GPU two and a half seconds during which it writes nothing for anyone. The request limiter in front counts requests, so it sees nothing unusual: two a second, as always. Everyone waits for the first word, and a second GPU is not in the budget.",
		Diagram: gpuBudget(false), Solution: gpuBudget(true),
		Goals:       engine.Goals{MinSuccessRate: 0.78, MaxTTFTMs: f(3000), MaxCostPerHour: f(6.0), StarCostPerHour: f(5.5), StarP95Ms: f(100)},
		DurationSec: 120, WarmupSec: 20, Speed: 1,
		Hints: [3]string{
			"Look at the time to first token and at what the model is doing while users wait. What kind of request keeps the GPU busy?",
			"The limiter counts requests, and all requests look alike to it. The cost of a request to the GPU is its prompt: a limiter that counts tokens would charge a pasted document for what it takes.",
			"What does a request cost the GPU, and what is the limiter counting instead? Which unit would match the cost?",
		},
		Answer: "The answer: switch the limiter's count to tokens and give it an allowance of about 56,000 tokens a minute: the short prompts fit easily, and the document-sized ones run the allowance dry and are {{load-shedding|turned away}} (roughly one request in ten is refused by design).",
		Predictions: []Prediction{{ID: "p1", Question: "A limiter that counts tokens instead of requests will mostly refuse…", Options: []string{"Everyone a little, to stay under the cap", "The few huge requests that use most of the tokens", "The newest users, who have no history yet"}, Answer: 1,
			Explain: "A request is let through only if its whole weight fits in what is left. A small one almost always does; a huge one needs far more than the bucket usually holds, so the heavy few are refused and the many light ones carry on."}},
		Debrief: "Capacity in an LLM service is measured in tokens, not requests: a request's cost is its prompt and its answer. {{load-shedding|Limiting by tokens}} (and, in practice, capping the prompt length and billing by token) keeps a few heavy requests from consuming what everyone shares.",
		Deeper:  "In an LLM service the cost of a request is its tokens, not its count: reading a long prompt keeps the GPU busy while it writes nothing for anyone, so one pasted document delays everyone's first word. A limiter that counts tokens charges the prompt up front, since the answer length is not yet known, and can settle the difference when the answer is finished; capping the prompt length and billing per token pushes the same pressure back onto users. In practice you also {{bulkhead|separate long prompts onto their own pool}} or priority, and split a long read into chunks so the decode work for other users is not blocked behind it. An interviewer will ask how to be fair per tenant rather than globally, and what the memory for the attention cache does to your batch size. The failure pattern is head-of-line blocking: a few expensive requests that look identical to cheap ones at the door.",
	})

	// Shed the cheap traffic: more arrives than the server can do, and a deep queue makes every request wait out its timeout
	shedCheap := func(fixed bool) engine.Diagram {
		app := P{"threads": 8.0, "queueCap": 2000.0}
		if fixed {
			app["shedAboveQueueMs"] = 100.0
		}
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 400.0, "readPct": 100.0, "keySpace": 5000.0, "priorityPct": 20.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, app).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 32.0}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "shed-the-cheap-traffic", Title: "Shed the cheap traffic", Difficulty: "Medium", ConfigOnly: true,
		Summary: "More traffic than the server can do, and a long queue makes everyone give up.",
		Story:   "Midday traffic is 400 requests a second and the app server can finish about 250. Excess requests wait in a queue that holds 2,000 of them, and a request that waited two seconds is abandoned by its user, so most of the work is spent on answers nobody is waiting for. One request in five is marked urgent (checkouts); the rest can be asked again later.",
		Diagram: shedCheap(false), Solution: shedCheap(true),
		Goals:       engine.Goals{MinSuccessRate: 0.4, MaxP95Ms: f(500), MaxCostPerHour: f(0.65), StarCostPerHour: f(0.6), StarP95Ms: f(200)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at the queue and at how long a request waits in it. Is the server busy with requests whose users are still there?",
			"Past 250 a second a longer queue only makes every request older. {{load-shedding|Turning some requests away early}} keeps the queue short, so the ones that are admitted are answered in time.",
			"What could the server look at, such as how long the oldest request has waited, to decide to turn work away before it joins a queue that is already too slow?",
		},
		Answer: "The answer: set 'Shed above' on the app server to a few hundred milliseconds: when the oldest waiting request is older than that, ordinary requests are {{load-shedding|turned away at the door}} and the urgent ones are still admitted.",
		Predictions: []Prediction{{ID: "p1", Question: "Turning away some requests at the door will make the share of requests answered…", Options: []string{"Lower, as every request turned away at the door is a failure by itself", "Higher, as work is no longer wasted on requests users abandoned", "Unchanged, since the server still does the same total work"}, Answer: 1,
			Explain: "An overloaded server with a deep queue serves requests so late that their users have left: very little of its work counts. Shedding spends the same capacity on requests that are still wanted."}},
		Debrief: "Overload is not fixed by queueing more. Bound the wait, {{load-shedding|shed the requests that can be retried}}, and keep the ones that matter (urgent, paid, already half done) flowing; the cap on the queue's age matters more than its length.",
	})

	// URL shortener: millions of reads of short links, and every new link takes its number from one counter row
	urlShortener := func(fixed bool) engine.Diagram {
		app := P{"threads": 64.0, "queueCap": 1000.0, "dbPool": 60.0}
		b := (&bld{}).
			n("visitors", engine.KClient, cols[0], rowTop, P{"rps": 1200.0, "readPct": 100.0, "keySpace": 50000.0, "keyDist": "zipf", "zipfS": 1.1, "timeoutMs": 1500.0, "retries": 0.0}).label("visitors", "Link visitors").
			n("creators", engine.KClient, cols[0], rowBot, P{"rps": 70.0, "readPct": 0.0, "keySpace": 50000.0, "keyDist": "uniform", "hotKeyPct": 100.0, "timeoutMs": 1500.0, "retries": 0.0}).label("creators", "Link creators")
		if fixed {
			app["hotKeyMode"] = "split"
			app["splitShards"] = 8.0
			b.n("app", engine.KAppServer, cols[1], rowMid, app).
				n("cache", engine.KCache, cols[2], rowMid, P{"capacityItems": 4000.0, "ttlSec": 300.0, "startWarm": true}).
				n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 12.0, "readMs": 20.0, "writeMs": 20.0, "hotRowLock": true}).label("db", "Link database").
				e("visitors", "app").e("creators", "app").e("app", "cache").e("cache", "db").e("app", "db")
		} else {
			b.n("app", engine.KAppServer, cols[1], rowMid, app).
				n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 12.0, "readMs": 20.0, "writeMs": 20.0, "hotRowLock": true}).label("db", "Link database").
				e("visitors", "app").e("creators", "app").e("app", "db")
		}
		return b.done()
	}
	register(&Scenario{
		ID: "url-shortener", Title: "The URL shortener", Difficulty: "Medium",
		Summary: "Short links are read hundreds of times for every one created, and every new link takes its number from one counter row.",
		Story:   "Visitors follow short links at 1,200 a second, {{hot-key|a few viral ones far more than the rest}}, and new links are created at 70 a second. Every read goes to the database, which can answer about 600 a second at best. Each new link also takes the next number from a single counter row, and the row lock lets one such write through every 20 ms: 50 a second, while 70 are asked for. The waiting writes hold {{connection-pool|database connections}} that the readers need.",
		Diagram: urlShortener(false), Solution: urlShortener(true),
		Goals:       engine.Goals{MinSuccessRate: 0.97, MaxP95Ms: f(250), MaxCostPerHour: f(3.0), StarCostPerHour: f(2.7), StarP95Ms: f(100)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"There are two different problems here, one on the read path and one on the write path. Which one do the failures come from first?",
			"Reads: a few links are followed again and again, so a cache in front of the database answers most of them. Writes: one counter row is a ceiling of one write per 20 ms however big the database.",
			"Reads and writes have different problems. What helps repeated reads, and what stops every new link from queuing on one counter row?",
		},
		Answer: "The answer: add a cache between the app server and the database, and in the app server's hot-key setting split the counter over several rows (or hand each server its own range of numbers).",
		Predictions: []Prediction{{ID: "p1", Question: "A cache in front of the database would fix…", Options: []string{"The slow reads, but not the counter", "Both the reads and the counter", "Only the counter, which is hit most"}, Answer: 0,
			Explain: "A cache absorbs repeated reads. Creating a link is a write, and every write to the counter row still has to take its turn at the database."}},
		Debrief: "A URL shortener is read-heavy with a skewed popularity: cache the popular links and {{cache-hit-rate|the database is left with the misses}}. The write path has its own trap, a single sequential ID: hand out blocks of numbers to each server, or split the counter, so creating links does not queue on one row.",
	})

	// Rate limiter showdown: a partner's job fires a burst on the second, across the edge of the limiter's window
	var edgeSpikes []engine.TrafficPoint
	edgeSpikes = append(edgeSpikes, engine.TrafficPoint{0, 100})
	for k := 2; k <= 60; k += 2 {
		at := float64(k)
		edgeSpikes = append(edgeSpikes, engine.TrafficPoint{at - 0.1, 100}, engine.TrafficPoint{at - 0.09, 8000}, engine.TrafficPoint{at + 0.09, 8000}, engine.TrafficPoint{at + 0.1, 100})
	}
	limiterShowdown := func(algo string) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 100.0, "keySpace": 2000.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("rl", engine.KRateLimiter, cols[1], rowMid, P{"limitRps": 300.0, "burst": 300.0, "algorithm": algo, "windowSec": 1.0}).
			n("app", engine.KAppServer, cols[2], rowMid, P{"threads": 32.0, "queueCap": 2000.0}).
			n("db", engine.KDatabase, cols[3], rowMid, P{"connections": 7.0, "readMs": 20.0}).
			e("users", "rl").e("rl", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "rate-limiter-showdown", Title: "Rate limiter showdown", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The limiter promises 300 requests a second, and the database sees twice that for a moment.",
		Story:   "A partner's batch job fires a burst of requests every other second, centred on the instant the clock ticks over, on top of a steady trickle of other users. The rate limiter promises no more than 300 requests a second to a database that does about 350, yet the database is drowning for a second after every burst. The limiter counts requests in fixed one-second windows that reset on the clock: half of each burst lands at the end of one window and half at the start of the next, and each half is given a full allowance. Most of the burst has to be {{load-shedding|turned away}} either way.",
		Diagram: limiterShowdown("fixedWindow"), Solution: limiterShowdown("slidingWindowLog"),
		Traffic:     map[string][]engine.TrafficPoint{"users": edgeSpikes},
		Goals:       engine.Goals{MinSuccessRate: 0.2, MaxP95Ms: f(800), MaxCostPerHour: f(1.1), StarCostPerHour: f(1.0)},
		DurationSec: 40, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"Look at what the database receives in the second around a burst, not at what the limiter allows per window.",
			"A fixed window forgets everything at its edge. A burst that straddles the edge is admitted twice, once in each window, so up to twice the limit gets through in a fraction of a second.",
			"Which limiter algorithm never lets more than the limit through in any window-length span, and what does each one cost in memory?",
		},
		Answer: "The answer: change the limiter's algorithm to one that never lets more than the limit through in any window-length span: the sliding window log is exact, the sliding window counter is a cheap approximation.",
		Predictions: []Prediction{{ID: "p1", Question: "A burst straddling the edge of a fixed one-second window with a limit of 300 lets through up to…", Options: []string{"At most 300 requests, exactly the limit", "Up to 600 requests, double the limit", "Between 300 and 450, depending on timing"}, Answer: 1,
			Explain: "Each window has its own count: the last moments of one window and the first moments of the next can each admit the whole allowance."}},
		Debrief: "Every rate-limiting algorithm makes a different promise at the boundary. A fixed window is cheap but allows up to twice the limit across its edge; a sliding log is exact and costs memory per request; a sliding counter is cheap and close; a token bucket allows a burst up to its size and then a steady rate. Pick the one whose worst case your backend can survive.",
	})

	typeahead := func(indexes float64) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 100.0, "readPct": 70.0, "secondaryPct": 80.0, "timeoutMs": 1500.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 24.0, "queueCap": 300.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0, "indexes": indexes, "diskIops": 130.0}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "typeahead-index", Title: "Typeahead under load", Difficulty: "Medium", ConfigOnly: true,
		Summary: "Keep prefix lookups fast while the index is maintained on writes.",
		Story:   "A catalogue serves suggestions as people type. Most reads search by a name prefix, while new and changed names must still be written. Keep p95 below 300 ms and the hourly cost under $1.20. This exercise models a prefix lookup as a lookup by another indexed field; it does not simulate a trie or matching each prefix length.",
		Diagram: typeahead(0), Solution: typeahead(1),
		Traffic: map[string][]engine.TrafficPoint{"users": {{0, 100}, {10, 100}, {20, 180}, {40, 180}, {50, 100}, {60, 100}}},
		Goals:   engine.Goals{MinSuccessRate: 0.98, MaxP95Ms: f(300), MaxCostPerHour: f(1.2), StarP95Ms: f(300), StarCostPerHour: f(1.05)}, DurationSec: 60, WarmupSec: 5,
		Hints: [3]string{
			"Which field does each suggestion query search, and how much work does an unindexed lookup ask the database to do?",
			"An index makes alternate-field reads cheaper, but the database must update each index when a row changes.",
			"How many of these reads need the alternate field, and is the reduction in read work worth the added write work?",
		},
		Answer: "The answer: keep one secondary index for the prefix field. It avoids a table scan on most reads while adding one index update to each write; more indexes add write work without helping this query.",
		Predictions: []Prediction{{ID: "p1", Question: "Adding one index for the suggestion field makes writes…", Options: []string{"Faster, because the database can find the row sooner", "Slower, because each write also updates the index", "Unchanged, because indexes only affect reads"}, Answer: 1,
			Explain: "An index saves read work for lookups by that field, and adds work whenever the field is written."}},
		Debrief: "A {{secondary-index|secondary index}} trades extra work on writes for faster lookups by another field. Keep only indexes that serve a real access path. This model treats a prefix query as one alternate-field lookup; it does not measure autocomplete ranking or results for each prefix length.",
	})

	crawler := func(dedup string, bits float64) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 25.0, "readPct": 100.0, "workload": "crawl", "crawlHosts": 40.0, "crawlHostSkew": 1.2, "crawlLinks": 6.0, "crawlDupPct": 80.0, "crawlDedup": dedup, "crawlUrls": 30000.0, "crawlBitsPerUrl": bits, "crawlHashes": 7.0, "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 32.0, "queueCap": 500.0}).
			n("db", engine.KDatabase, cols[2], rowMid, P{"connections": 16.0}).
			e("users", "app").e("app", "db").done()
	}
	register(&Scenario{
		ID: "crawl-dedup", Title: "The repeating crawler", Difficulty: "Medium",
		Summary: "Stop the frontier from fetching pages it has already found.",
		Story:   "A crawler visits many sites and each page links to pages already seen as well as new ones. At least 98% of requests must succeed, p95 must stay below 300 ms, cost must stay under $1.20 an hour, at least 98% of new pages must be found, and no page may be fetched twice. Choose a seen-URL check and keep the frontier moving.",
		Diagram: crawler("none", 10), Solution: crawler("exact", 10),
		Goals:       engine.Goals{MinSuccessRate: 0.98, MaxP95Ms: f(300), MaxCostPerHour: f(1.2), StarCostPerHour: f(1.0), MinRecall: f(0.98), MaxDuplicates: i(0)},
		DurationSec: 60, WarmupSec: 5,
		Hints: [3]string{
			"What happens when a page links back to a URL already in the frontier or fetched earlier?",
			"An exact set grows with each distinct URL. A Bloom filter uses fixed memory but can sometimes say a new URL was already seen.",
			"Which seen-URL check meets both the recall target and the rule against fetching a page twice?",
		},
		Answer: "The answer: use an exact seen-URL set. It avoids refetching known pages without false positives, at the cost of memory that grows with the number of distinct URLs.",
		Predictions: []Prediction{{ID: "p1", Question: "A Bloom filter reports that a new URL was already seen. The crawler will…", Options: []string{"Fetch it anyway, because Bloom filters keep an exact set", "Skip it, so a false positive can hide a new page", "Add another URL to the frontier to compensate"}, Answer: 1,
			Explain: "Bloom filters can report a false positive. The crawler saves memory but may skip a new URL, reducing recall."}},
		Debrief: "A {{crawl-frontier|frontier}} holds discovered pages waiting to be fetched. A seen-URL check prevents repeats; an exact set preserves recall but grows with distinct URLs, while a Bloom filter trades some recall for bounded memory. This run does not charge for the exact set's memory.",
	})
}
