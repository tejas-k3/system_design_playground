package scenarios

// Levels are the interview bands a challenge is written for: sde1 (new grad to about two years: fundamentals), sde2
// (about two to five years: a whole service and its weak points) and sde3 (five years and up: judgement and failure
// analysis). The band is about the idea being tested, not about how hard the clicking is, so it does not follow the
// Difficulty label.
var Levels = []string{"sde1", "sde2", "sde3"}

func validLevel(l string) bool {
	for _, v := range Levels {
		if v == l {
			return true
		}
	}
	return false
}

func levelList(level string, ids ...string) map[string]string {
	m := map[string]string{}
	for _, id := range ids {
		m[id] = level
	}
	return m
}

// levelOf maps a challenge id to its band. A challenge missing from here fails TestEveryScenarioHasALevel.
var levelOf = func() map[string]string {
	m := map[string]string{}
	for _, part := range []map[string]string{
		levelList("sde1", "hello-world", "edge-cache", "slow-reads", "opening-night", "slow-query", "server-down", "write-flood",
			"one-slow-server", "upload-500kb", "video-upload", "first-design-links"),
		levelList("sde2", "open-link-shortener", "open-notifications", "open-news-feed", "open-nearby-search", "open-chat", "typeahead-index", "crawl-dedup",
			"connection-crunch", "retry-pileup", "stale-prices", "who-needs-telling", "double-booked", "charged-twice", "sync-or-async",
			"wave-rider", "flash-crowd", "passed-the-load-test", "cache-stampede", "hot-shard", "scan-pollution", "celebrity-post",
			"image-egress", "gc-storm", "disk-writes", "one-heavy-query", "pick-the-store", "poison-pill", "consumer-lag", "hot-cell",
			"count-the-clicks", "restart-the-counter", "ranked-feed", "chat-order", "front-door", "cold-start", "slow-news", "everyone-asks-auth", "where-do-sessions-live", "unique-visitors", "polite-crawler", "url-shortener", "rate-limiter-showdown", "noisy-tenant",
			"shed-the-cheap-traffic", "notification-fan-out", "add-a-shard", "reconnect-storm", "bad-deploy", "flaky-neighbour",
			"cut-off", "multi-region-reads", "slow-first-token", "rag-under-load", "live-scoreboard", "nearby-drivers",
			"ten-thousand-students", "restore-window", "stale-dns"),
		levelList("sde3", "primary-dies", "fence-the-old-primary", "who-is-the-leader", "lease-expired", "quorum-overlap", "effect-before-cause", "older-write-wins", "liar-in-the-group", "old-leaders-writes",
			"lost-cart", "payment-saga", "frozen-checkout", "nobody-to-wait-for", "disappearing-money", "restore-drill", "retry-storm", "heavy-tail",
			"nobody-on-call", "vendor-wait", "scoreboard-survives", "search-tail", "top-ten", "the-gpu-budget", "zone-outage",
			"go-global", "split-the-database", "morning-rush", "launch-day", "launch-day-capstone", "bot-flood",
			"checkout-during-a-sale"),
	} {
		for k, v := range part {
			m[k] = v
		}
	}
	return m
}()
