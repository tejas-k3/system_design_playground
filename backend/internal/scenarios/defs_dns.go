package scenarios

import "sysdesign/internal/engine"

func registerDNS() {
	design := func(ttl float64) engine.Diagram {
		return (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 60.0, "readPct": 100.0, "keySpace": 200.0, "timeoutMs": 1000.0, "retries": 0.0, "dnsTtlSec": ttl}).
			n("app1", engine.KAppServer, cols[2], rowTop, P{"threads": 64.0}).
			n("app2", engine.KAppServer, cols[2], rowBot, P{"threads": 64.0}).
			n("db", engine.KDatabase, cols[4], rowMid, P{"connections": 64.0}).
			e("users", "app1").e("users", "app2").e("app1", "db").e("app2", "db").done()
	}
	register(&Scenario{
		ID: "stale-dns", Title: "A stale DNS record", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The replacement is ready, but users keep calling the failed address.",
		Story:   "At ten seconds, one of two app servers fails. The other is healthy and reachable. Users keep sending some requests to the old address until their cached {{ttl|DNS record expires}}. Shorten the Users' DNS record TTL so traffic moves to the survivor sooner; name lookups add a small cost.",
		Diagram: design(30), Solution: design(2),
		Chaos:       []engine.ChaosEvent{{Type: "crash", NodeID: "app1", AtSec: 10, DownSec: 35}},
		Goals:       engine.Goals{MinSuccessRate: 0.9, MaxP95Ms: f(400), MaxCostPerHour: f(6.0), StarP95Ms: f(250), StarCostPerHour: f(5.8)},
		DurationSec: 55, WarmupSec: 5, Speed: 1,
		Hints: [3]string{
			"The second app server stays healthy. Why do requests still fail after the crash?",
			"The Users keep a DNS answer for a while. Find the advanced name-resolution setting on Users.",
			"What TTL balances faster failover against the extra cost of resolving names more often?",
		},
		Answer: "The answer: shorten the Users' DNS record TTL. A cached address can keep pointing at a failed server until it expires, while a shorter TTL means more DNS lookups.",
		Predictions: []Prediction{{ID: "p1", Question: "After a server fails, what does a shorter DNS TTL change?", Options: []string{"It makes the healthy server process each request faster", "It lets users refresh the address sooner, with more lookups", "It prevents the server from failing again"}, Answer: 1,
			Explain: "A shorter TTL makes clients resolve the name more often, so they can stop using a failed address sooner. It does not change the server's processing time or prevent failures."}},
		Debrief: "DNS turns a service name into an address. Caching that answer saves lookups, but a long TTL can leave callers using an old address after a server fails. Choose the TTL to balance recovery time and lookup cost. A virtual IP that moves to the survivor would avoid the wait altogether, because callers keep the same address (not modelled here).",
	})
}
