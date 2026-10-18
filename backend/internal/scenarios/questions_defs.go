package scenarios

import "fmt"

const (
	secPerDay   = 86400.0
	secPerMonth = 30 * secPerDay
)

func init() {
	// URL shortener: 100 M new URLs a month, 10 reads per write, 3x peak, 500 B a record, kept 5 years.
	writeQps := 100e6 / secPerMonth
	registerQuestion(&Question{
		ID: "url-shortener-numbers", Title: "A URL shortener: how big?", Mode: ModeEstimate, Difficulty: "Easy",
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
	registerQuestion(&Question{
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
	registerQuestion(&Question{
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

	registerDiagnose("cut-off-threads", diagSource{"cut-off", 1, 60}, &Question{
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

	registerDiagnose("retry-storm-recovery", diagSource{"retry-pileup", 1, 60}, &Question{
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

	registerDiagnose("stampede-spikes", diagSource{"cache-stampede", 1, 60}, &Question{
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

func registerDiagnose(id string, src diagSource, q *Question) {
	q.ID, q.Mode, q.source = id, ModeDiagnose, src
	q.Symptoms = loadSymptoms(id)
	registerQuestion(q)
}
