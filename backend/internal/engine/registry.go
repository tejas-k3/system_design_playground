package engine

import "math"

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type ShowIf struct {
	Key string   `json:"key"`
	In  []string `json:"in"`
}

// ParamSpec describes one editable setting; the UI renders the inspector from these.
type ParamSpec struct {
	Key              string   `json:"key"`
	Label            string   `json:"label"`
	Group            string   `json:"group"`
	Type             string   `json:"type"` // number | select | boolean
	Default          any      `json:"default"`
	Unit             string   `json:"unit,omitempty"`
	Min              float64  `json:"min,omitempty"`
	Max              float64  `json:"max,omitempty"`
	Step             float64  `json:"step,omitempty"`
	Help             string   `json:"help,omitempty"`
	Options          []Option `json:"options,omitempty"`
	ShowIf           *ShowIf  `json:"showIf,omitempty"`
	NeedsEdgeTo      []string `json:"needsEdgeTo,omitempty"`
	FixedInChallenge bool     `json:"fixedInChallenge,omitempty"`
	Advanced         bool     `json:"advanced,omitempty"`
}

// NodeSpec is a node kind: how it looks, what it may connect to, and what it costs.
type NodeSpec struct {
	Kind        string      `json:"kind"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Lesson      string      `json:"lesson"`
	Accent      string      `json:"accent"`
	Width       float64     `json:"width"`
	Height      float64     `json:"height"`
	CanConnect  []string    `json:"canConnectTo"`
	Params      []ParamSpec `json:"params"`
	cost        func(Params) float64
}

func num(key, label, group string, def, min, max, step float64, unit, help string) ParamSpec {
	return ParamSpec{Key: key, Label: label, Group: group, Type: "number", Default: def, Min: min, Max: max, Step: step, Unit: unit, Help: help}
}

func sel(key, label, group, def, help string, opts ...Option) ParamSpec {
	return ParamSpec{Key: key, Label: label, Group: group, Type: "select", Default: def, Options: opts, Help: help}
}

func flag(key, label, group string, def bool, help string) ParamSpec {
	return ParamSpec{Key: key, Label: label, Group: group, Type: "boolean", Default: def, Help: help}
}

// variability is the shared "service-time spread" setting of nodes that do work (see RNG.ServiceDist).
func variability(group string) ParamSpec {
	return sel("variability", "Service-time spread", group, "steady",
		"How much individual requests differ from the average. Steady is tidy; exponential is the textbook case; heavy-tailed has rare very slow requests, like real services.",
		opt("steady", "Steady"), opt("exponential", "Exponential"), opt("heavy", "Heavy-tailed")).adv()
}

// clockSkew: expiry compares a time stamped by the origin with this node's own clock, so a clock that is off
// shortens or lengthens every lifetime by the same amount.
func clockSkew() ParamSpec {
	return num("clockSkewMs", "Clock offset", "Freshness", 0, -60000, 60000, 500, "ms", "How far this node's clock is ahead of the origin's (negative = behind). Entries are stamped with the origin's time, so a clock 8 s ahead makes a 10 s lifetime last 2 s, and one behind makes it last longer.").adv()
}

// ttlJitter spreads entry lifetimes so a whole cache does not expire in the same instant.
func ttlJitter() ParamSpec {
	return num("ttlJitterPct", "TTL jitter", "Freshness", 0, 0, 90, 5, "%", "Each entry lives its TTL plus or minus this much, so entries do not all expire together.").adv()
}

func opt(v, l string) Option { return Option{Value: v, Label: l} }

func (p ParamSpec) showIf(key string, in ...string) ParamSpec {
	p.ShowIf = &ShowIf{Key: key, In: in}
	return p
}
func (p ParamSpec) needs(kinds ...string) ParamSpec { p.NeedsEdgeTo = kinds; return p }
func (p ParamSpec) fixed() ParamSpec                { p.FixedInChallenge = true; return p }
func (p ParamSpec) adv() ParamSpec                  { p.Advanced = true; return p }

func pb(p Params, k string) bool { v, _ := p[k].(bool); return v }

func pf(p Params, k string, def float64) float64 {
	switch v := p[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return def
}

var specs []NodeSpec
var specByKind map[string]*NodeSpec

func init() {
	specs = []NodeSpec{
		{
			Kind: KClient, Name: "Users", Description: "A crowd of users sending requests.",
			Lesson: "Traffic is bursty: design for the peak, not the average.",
			Accent: "#1971c2", Width: 160, Height: 72,
			CanConnect: []string{KLoadBalancer, KAppServer, KCDN, KRateLimiter},
			Params: []ParamSpec{
				flag("geoRouting", "Send users to the nearest region", "Traffic", false, "With a stack in more than one region connected to these users, send each request to the one in the region closest to them (geo-DNS routing), and to the next nearest if that one is down. Off: take turns among all of them.").adv(),
				num("dnsTtlSec", "DNS record TTL", "Traffic", 0, 0, 3600, 1, "s", "How long after a target fails Users keeps sending to it, until its cached answer expires and it resolves the name again. This is the worst case, with every cached answer as old as it can be. A request sent to the dead address is a real attempt: it takes the time a call to a dead server takes, and a retry can reach another target. 0 keeps today's routing behaviour. Each lookup costs $0.00001; a shorter TTL can restore traffic sooner but increases lookups.").adv(),
				sel("arrival", "User behaviour", "Traffic", "open", "Independent users keep arriving whatever happens (a crowd of strangers). Bursty users arrive in short rushes with the same average. A fixed crowd waits for each answer, thinks, then asks again, so it slows down when the system does.",
					opt("open", "Independent users (steady arrivals)"), opt("bursty", "Bursty: short rushes"), opt("closed", "A fixed crowd that waits and thinks"), opt("sessions", "Connected users (long-lived connections)")),
				num("users", "Users", "Traffic", 50, 1, 5000, 10, "", "How many users there are; each has at most one request in flight.").showIf("arrival", "closed"),
				num("thinkMs", "Think time", "Traffic", 1000, 0, 60000, 100, "ms", "Average pause between getting an answer and asking again.").showIf("arrival", "closed"),
				num("missingPct", "Asks for what is not there", "Request", 0, 0, 100, 1, "%", "Share of requests for an id that does not exist (a lookup of something deleted or never made). The database has to be asked to find out, unless something in front of it already knows.").adv(),
				num("requestTokens", "Tokens per request", "Request", 0, 0, 200000, 50, "tokens", "How many tokens a request carries (an LLM prompt), for limiters that count tokens. 0 = one unit each.").adv(),
				num("bigRequestPct", "Huge requests", "Request", 0, 0, 100, 1, "%", "Share of requests that carry 20 times as many tokens.").adv().showIf("arrival", "open", "bursty", "closed"),
				num("burstFactor", "Rush intensity", "Traffic", 4, 1.5, 5, 0.5, "x", "During a rush the rate is this many times the average (a rush covers a fifth of the time; the rest is quieter so the average stays put).").showIf("arrival", "bursty"),
				num("burstSec", "Rush length", "Traffic", 2, 0.5, 30, 0.5, "s", "Average length of a rush.").showIf("arrival", "bursty"),
				num("connections", "Connected users", "Traffic", 1000, 1, 100000, 100, "", "How many users keep a connection open. Each holds a slot on an app server while connected; when it ends, or the server dies, they connect again.").showIf("arrival", "sessions"),
				num("sessionSec", "Connection length", "Traffic", 300, 1, 86400, 10, "s", "Average time a connection stays open before the user reconnects.").showIf("arrival", "sessions"),
				num("msgsPerMin", "Messages per user", "Traffic", 6, 0, 600, 1, "/min", "How often a connected user sends a message (an ordinary request).").showIf("arrival", "sessions"),
				num("reconnectJitterSec", "Reconnect spread", "Traffic", 5, 0, 120, 1, "s", "After a refusal or a lost connection a user waits 1 s plus a random time up to this. 0 = everyone returns at the same moment, a reconnect storm.").showIf("arrival", "sessions"),
				num("rps", "Requests / sec", "Traffic", 50, 1, 5000, 10, "rps", "Base request rate.").showIf("arrival", "open", "bursty"),
				sel("pattern", "Traffic pattern", "Traffic", "constant", "Shape of traffic over time; ramp, spike and wave move between base and peak.",
					opt("constant", "Constant"), opt("ramp", "Ramp up"), opt("spike", "Spikes"), opt("wave", "Daily wave")),
				num("peakRps", "Peak requests / sec", "Traffic", 200, 1, 5000, 10, "rps", "Rate reached at the top of a ramp, spike or wave.").showIf("pattern", "ramp", "spike", "wave"),
				num("periodSec", "Period", "Traffic", 20, 2, 300, 1, "s", "Ramp duration, or spike/wave period.").showIf("pattern", "ramp", "spike", "wave"),
				num("readPct", "Reads", "Traffic", 80, 0, 100, 5, "%", "Share of requests that are reads; the rest are writes."),
				num("timeoutMs", "Timeout", "Patience & retries", 2000, 100, 30000, 100, "ms", "Users give up after this long. Abandoned work keeps running downstream."),
				num("retries", "Retries", "Patience & retries", 0, 0, 5, 1, "", "Extra attempts after a failure or timeout. Retries add load exactly when the system is struggling (a {{retry-storm|retry storm}})."),
				sel("backoff", "Retry backoff", "Patience & retries", "exponential", "Immediate retries can turn a blip into a retry storm.",
					opt("exponential", "Exponential + jitter"), opt("none", "Immediate")),
				num("reqKB", "Request size", "Payload", 0, 0, 102400, 10, "KB", "How big each request is (what an upload sends). 0 = not modelled. A message queue refuses anything above its own size limit.").adv(),
				num("respKB", "Response size", "Payload", 0, 0, 102400, 10, "KB", "How big each response is. 0 = not modelled. Recorded for the bandwidth model that builds on this.").adv(),
				sel("sizeDist", "Size distribution", "Payload", "fixed", "Every request and response the size above (fixed), or sizes spread lognormally around that mean: most are smaller, a few are several times larger. The average load is the same, but the big ones queue behind each other, so the tail of the latency is worse. Needs a size above.", opt("fixed", "Fixed"), opt("lognormal", "Lognormal: a heavy tail")).adv(),
				num("sizeSigma", "Size spread", "Payload", 0.5, 0.1, 2, 0.1, "", "How wide the lognormal spread is (the standard deviation of the logarithm of the size): 0.5 puts the biggest 1 in 100 at about 3 times the mean, 1 at about 10 times.").adv().showIf("sizeDist", "lognormal"),
				num("retryBudgetPct", "Retry budget", "Patience & retries", 0, 0, 100, 5, "%", "Retries may add at most this share to the number of first attempts (over the last ten seconds). Past it, a failure is simply reported instead of retried, so a struggling system is not buried under its own retries. 0 = no limit.").adv(),
				num("identities", "Distinct users", "Identity", 0, 0, 1000000, 10, "", "How many different accounts the requests come from (0 = not modelled). A rate limiter set to limit per user counts each account separately.").adv(),
				num("tenants", "Tenants", "Identity", 0, 0, 50, 1, "", "Group the accounts into this many tenants (customers of a shared service), in blocks: the first tenant has the lowest-numbered accounts, which with account skew are the busiest. A rate limiter set to limit per tenant then gives each tenant its own allowance. 0 = not modelled.").adv(),
				num("identitySkew", "Account skew", "Identity", 0, 0, 2, 0.1, "", "0 = every account sends equally often; higher means a few accounts send most of the traffic.").adv(),
				flag("abuse", "Bot traffic", "What they do", false, "These users are a scraper or attacker: they ask for a different item each time from a huge space (so caches rarely help), never retry, and ignore what they are told. Add them alongside real users to see what they do to the system and what a rate limiter protects.").adv(),
				num("heavyPct", "Heavy requests", "What they do", 0, 0, 50, 1, "%", "Share of requests that are expensive (a big report, a huge query): they cost heavyMult times the CPU and database time. A few of them can block the many light ones queued behind.").adv(),
				num("heavyMult", "Heavy cost", "What they do", 20, 2, 200, 1, "x", "How many times more CPU and database time a heavy request costs.").adv(),
				num("priorityPct", "Urgent requests", "What they do", 0, 0, 100, 5, "%", "Share of requests marked urgent. Only a queue set to serve urgent work first treats them differently.").adv(),
				num("secondaryPct", "Lookups by another field", "What they do", 0, 0, 100, 5, "%", "Share of reads that look an item up by something other than its key (email, status, date). Without a secondary index on the database each one scans.").adv(),
				sel("workload", "Workload", "What they do", "kv", "What kind of requests users make.",
					opt("kv", "General: read & write items"), opt("geo", "Location: nearby search & pings"),
					opt("leaderboard", "Leaderboard: rank reads & score updates"), opt("booking", "Booking: reserve scarce seats"),
					opt("transfer", "Transfers: move money between two accounts"), opt("feed", "Feed: posts reach followers, reads fetch a timeline"), opt("oncall", "On-call pairs: read two rows, write one (write skew)"), opt("crawl", "Web crawl: fetch URLs from a frontier of pages and sites")),
				num("followersMean", "Followers per author", "What they do", 100, 1, 100000, 10, "", "Average followers of an author. Counts are Zipf-skewed, so a few authors have far more.").showIf("workload", "feed"),
				num("followersSkew", "Follower skew", "What they do", 1, 0, 2, 0.1, "", "0 = everyone has the same number of followers; higher means a few celebrities hold most of them.").showIf("workload", "feed"),
				sel("keyDist", "Key popularity", "What they do", "zipf", "How requests spread over items.",
					opt("zipf", "Skewed (Zipf)"), opt("uniform", "Uniform"), opt("scan", "Sequential scan")),
				num("keySpace", "Distinct items", "What they do", 10000, 1, 1000000, 100, "", "Size of the item universe."),
				num("zipfS", "Skew", "What they do", 1, 0, 2, 0.1, "", "Zipf exponent: higher means a few items dominate.").showIf("keyDist", "zipf"),
				sel("consistency", "Freshness needed", "Consistency", "eventual", "How fresh a read must be. Strong compares with the latest write by anyone; bounded allows a set age; bounded by missed writes allows a set number of missed writes; read-your-writes means a client must see its own writes, and monotonic reads must not go backwards. The combined option asks for both. Monotonic writes keep one client's writes in issue order; writes follow reads counts a write that reaches a copy older than the value the client last read. Causal order counts a read that shows an older cause after the client has seen an effect built on a newer one. Quorum settings do not change monotonic writes, and writes follow reads is only judged on a key-value store's copies. All but the first four are checked per client.",
					opt("eventual", "Eventual (stale is fine)"), opt("bounded", "Bounded staleness"), opt("strong", "Strong (always fresh)"),
					opt("readYourWrites", "Read your writes"), opt("monotonic", "Monotonic reads"), opt("causal", "Read your writes + monotonic reads"),
					opt("monotonicWrites", "Monotonic writes"), opt("writesFollowReads", "Writes follow reads"), opt("boundedWrites", "Bounded by missed writes"), opt("causalOrder", "Causal order: no effect before its cause")),
				num("maxStaleMs", "Max staleness", "Consistency", 1000, 0, 60000, 100, "ms", "Reads older than this count as too stale.").showIf("consistency", "bounded"),
				num("maxMissedWrites", "Max missed writes", "Consistency", 1, 0, 100, 1, "", "A read that misses more than this many committed writes of its item counts as too stale: a numerical bound instead of an age bound.").showIf("consistency", "boundedWrites"),
				num("hotKeyPct", "Hot item share", "What they do", 0, 0, 100, 1, "%", "Share of requests hitting one single item.").adv(),
				num("crawlHosts", "Sites", "Crawl", 20, 1, 1000, 1, "", "How many sites the pages belong to. Page counts are Zipf-skewed, so a few sites are huge.").showIf("workload", "crawl"),
				num("crawlHostSkew", "Site skew", "Crawl", 1, 0, 2, 0.1, "", "0 = every site is equally big; higher means a few sites hold most pages.").showIf("workload", "crawl"),
				num("crawlLinks", "Links per page", "Crawl", 4, 1, 50, 1, "", "How many links each fetched page contains.").showIf("workload", "crawl"),
				num("crawlDupPct", "Links to known pages", "Crawl", 30, 0, 95, 5, "%", "Share of found links that point at a page already found (the web links back to itself).").showIf("workload", "crawl"),
				sel("crawlDedup", "Seen-URL check", "Crawl", "exact", "How a link is checked against the pages already found. None: known pages are fetched again. Exact set: nothing is repeated, but memory grows with every distinct URL. Bloom filter: memory is fixed, but its false positives make the crawler skip new pages it never saw (lost recall).",
					opt("none", "None: fetch repeats"), opt("exact", "Exact set (memory grows)"), opt("bloom", "Bloom filter (fixed memory)")).showIf("workload", "crawl"),
				num("crawlUrls", "Expected distinct URLs", "Crawl", 100000, 1000, 1000000, 1000, "", "The number of URLs the Bloom filter is sized for.").adv().showIf("workload", "crawl"),
				num("crawlBitsPerUrl", "Filter bits per URL", "Crawl", 10, 1, 64, 1, "bits", "Filter size is this many bits for each expected URL; the miss rate is (1 - e^(-hashes x URLs / bits))^hashes.").adv().showIf("workload", "crawl"),
				num("crawlHashes", "Filter hash functions", "Crawl", 7, 1, 16, 1, "", "How many bits each URL sets and each check reads.").adv().showIf("workload", "crawl"),
				flag("crawlTrap", "A crawler trap", "Crawl", false, "One site generates endless new links (a calendar, a session id in every URL): every page of it links to new pages, forever.").showIf("workload", "crawl"),
				num("crawlTrapLinks", "Trap links per page", "Crawl", 2, 1, 20, 1, "", "New links on each page of the trap site.").adv().showIf("workload", "crawl"),
				num("maxDepth", "Maximum depth", "Crawl", 0, 0, 100, 1, "", "Links are not followed from pages this many clicks from a starting page. 0 = no limit, so a trap never ends.").showIf("workload", "crawl"),
			},
			cost: func(Params) float64 { return 0 },
		},
		{
			Kind: KLoadBalancer, Name: "Load Balancer", Description: "Spreads requests across app servers.",
			Lesson: "A balancer is only as good as its idea of which servers are healthy.",
			Accent: "#2f9e44", Width: 160, Height: 72, CanConnect: []string{KAppServer},
			Params: []ParamSpec{
				sel("algorithm", "Algorithm", "Routing", "roundRobin", "How the next server is chosen.",
					opt("roundRobin", "Round robin"), opt("leastConn", "Least connections"), opt("random", "Random")),
				sel("zoneRouting", "Zone preference", "Routing", "any", "Any zone: spread across every server. Own zone first: use only servers in the balancer's own availability zone, which saves the hop between zones (1.5 ms each way).",
					opt("any", "Any zone"), opt("local", "Own zone first")).adv(),
				flag("crossZoneFailover", "Fail over to other zones", "Routing", true, "With 'own zone first': when no server in the balancer's zone is available, send requests to the other zones. Off, they have nowhere to go until a server in the zone is back.").adv(),
				flag("sticky", "Keep a user on one server", "Routing", false, "Sticky routing: requests from the same user go to the server they went to before, while it is healthy. It keeps local sessions found, but a busy user pins load to one server and a server that dies takes its sessions with it.").adv(),
				flag("outlierEjection", "Eject outliers", "Health", false, "Temporarily remove servers that error a lot or are much slower than their peers."),
				num("canaryPct", "Canary share", "Release", 0, 0, 100, 1, "%", "Send this share of requests to the servers running the new version (v2) and the rest to the old one (v1). 0 = every server gets its fair share.").adv(),
				num("healthCheckSec", "Health check interval", "Health", 2, 1, 30, 1, "s", "How long a dead server keeps receiving traffic before the balancer notices."),
				sel("layer", "Works at", "Capacity", "layer7", "Layer 7 reads each request, so it can route on what it contains (a canary share, a sticky user) and costs more per request. Layer 4 only forwards connections: cheaper, quicker and about four times the throughput, but it cannot route by content, so canary shares and sticky users are ignored.",
					opt("layer7", "Layer 7 (reads the request)"), opt("layer4", "Layer 4 (forwards connections)")).adv(),
				num("maxRps", "Most requests a second", "Capacity", 0, 0, 100000, 100, "/s", "The balancer is one machine: past this many requests a second (four times as many at layer 4) requests wait, and when the wait is longer than the queue allows they are refused. A bigger one costs more. 0 = never the bottleneck.").adv(),
				num("lbQueueMs", "Longest wait", "Capacity", 200, 1, 5000, 10, "ms", "How long a request may wait for the balancer before it is refused (503).").adv().showIf("layer", "layer7", "layer4"),
			},
			cost: func(p Params) float64 {
				c := 0.02
				if ps(p, "layer") == "layer4" {
					c = 0.01
				}
				return c + pf(p, "maxRps", 0)*0.00001
			},
		},
		{
			Kind: KAppServer, Name: "App Server", Description: "Runs your code. Each request holds a thread until it finishes.",
			Lesson: "A thread waiting on a slow database cannot serve anyone else.",
			Accent: "#868e96", Width: 160, Height: 72,
			CanConnect: []string{KCache, KDatabase, KReplica, KQueue, KShardRouter, KExternalAPI, KRedis, KObjectStore, KIndex, KAuth, KTopic, KLog, KKVStore, KCoordinator, KStream, KLLMServer, KVectorStore},
			Params: []ParamSpec{
				num("threads", "Threads", "Capacity", 8, 1, 64, 1, "", "Concurrent requests it can work on. Bigger machines cost more than proportionally."),
				sel("version", "Release version", "Release", "v1", "Which release this server runs. A load balancer with a canary share sends that share of traffic to v2; a bad-release event can be aimed at one version.", opt("v1", "v1 (current)"), opt("v2", "v2 (new)")).adv(),
				num("cpuMs", "CPU time", "Capacity", 20, 1, 500, 1, "ms", "Compute per request, not counting time waiting on dependencies.").fixed(),
				variability("Capacity"),
				flag("autoscale", "Autoscale", "Scaling", false, "{{autoscaling|Autoscale}}: add and remove whole servers (each with these threads) as load changes. New servers take time to boot, and every running server costs money.").adv(),
				num("minInstances", "Min servers", "Scaling", 1, 1, 32, 1, "", "Never fewer than this many.").adv().showIf("autoscale", "true"),
				num("maxInstances", "Max servers", "Scaling", 8, 1, 32, 1, "", "Never more than this many.").adv().showIf("autoscale", "true"),
				num("targetUtilPct", "Target busy level", "Scaling", 60, 20, 95, 5, "%", "Add servers when threads are busier than this, over the last 30 s.").adv().showIf("autoscale", "true"),
				sel("runtime", "Runs as", "Scaling", "vm", "A virtual machine takes the boot time above to start; a container shares its host, starts in seconds and costs a fifth less for the same threads.",
					opt("vm", "Virtual machine"), opt("container", "Container")).adv().showIf("autoscale", "true"),
				num("containerBootSec", "Container start time", "Scaling", 3, 0.5, 60, 0.5, "s", "How long a new container takes before it can take requests.").adv().showIf("runtime", "container"),
				num("coldStartMs", "Cold start", "Scaling", 0, 0, 30000, 50, "ms", "A function that scales to zero: the first request after it has been idle waits this long for it to start, and so do the ones that arrive meanwhile. 0 = always running.").adv(),
				num("idleSec", "Idle before it stops", "Scaling", 30, 0.1, 900, 0.1, "s", "How long without a request before the function is stopped to save money. Only with a cold start.").adv(),
				flag("keepWarm", "Keep one warm", "Scaling", false, "Pay for one instance that never stops, so there is no cold start. Only with a cold start.").adv(),
				num("scaleUpDelaySec", "Boot time", "Scaling", 30, 1, 300, 1, "s", "How long a new server takes before it can take requests.").adv().fixed().showIf("autoscale", "true"),
				num("scaleDownDelaySec", "Scale-down wait", "Scaling", 120, 0, 900, 5, "s", "How long load must stay low before one server is removed (stops it flapping).").adv().showIf("autoscale", "true"),
				num("queueCap", "Queue size", "Capacity", 50, 0, 2000, 10, "", "Requests that may wait for a thread. When full, new ones get a 503."),
				sel("queueOrder", "Queue order", "Capacity", "fifo", "Who gets the next free thread. Under overload newest-first serves people still waiting.",
					opt("fifo", "Oldest first"), opt("lifo", "Newest first")),
				num("queueTimeoutMs", "Max queue wait", "Capacity", 0, 0, 30000, 50, "ms", "Drop queued requests that waited longer than this (0 = never).").adv(),
				sel("feedStrategy", "Feed strategy", "Feed", "pushOnWrite", "For the feed workload: copy each post into every follower's timeline when it is written (fast reads, a post by a celebrity is a flood of writes), build the timeline when it is read (cheap posts, expensive reads), or push for ordinary authors and pull celebrities.",
					opt("pushOnWrite", "Push on write"), opt("pullOnRead", "Pull on read"), opt("hybrid", "Hybrid: pull celebrities")).adv(),
				num("celebrityThreshold", "Celebrity above", "Feed", 1000, 10, 1000000, 10, "followers", "Hybrid only: authors with more followers than this are pulled, not pushed.").showIf("feedStrategy", "hybrid").adv(),
				sel("gather", "Fan-out", "Fan-out", "none", "With two or more search shards behind this server, a read can go to every shard at once. Wait for all gives the slowest shard's latency; answering on the first K skips the slowest ones, but they still do the work.",
					opt("none", "Off"), opt("all", "Ask every shard, wait for all"), opt("firstK", "Ask every shard, answer on the first K")).needs(KIndex),
				num("k", "Answer after", "Fan-out", 3, 1, 64, 1, "shards", "How many shards must answer. More than there are shards means all of them.").showIf("gather", "firstK"),
				num("hedgeAfterMs", "Hedge after", "Fan-out", 0, 0, 5000, 5, "ms", "If a shard has not answered after this long, ask it again and take whichever answer comes first. The slow first query keeps running, so hedging adds load. 0 = off.").showIf("gather", "all", "firstK").adv(),
				flag("partialResults", "Partial results", "Fan-out", false, "If too few shards answered, still reply with what there is (counted as partial) instead of failing, as long as at least one answered.").showIf("gather", "all", "firstK"),
				num("gcPauseMs", "GC pause", "Failure modes", 0, 0, 5000, 50, "ms", "Stop-the-world garbage-collection pauses: for this long nothing on the server starts or finishes. 0 = off.").adv(),
				num("gcEverySec", "GC pause every", "Failure modes", 30, 1, 600, 1, "s", "Average time between pauses; the gaps are random (exponential), so pauses are not evenly spaced.").adv(),
				num("burstBaselinePct", "CPU baseline", "Failure modes", 100, 5, 100, 5, "%", "A burstable instance: the share of its CPU it may use all the time. Above it, it spends credits; with none left it is throttled to this share. 100 = not burstable.").adv(),
				num("burstCreditSec", "CPU credits", "Failure modes", 60, 1, 3600, 1, "s", "How many seconds of full-speed CPU the server can bank. It starts full and earns credits whenever it uses less than its baseline.").adv(),
				num("memLimitMB", "Memory limit", "Failure modes", 0, 0, 65536, 64, "MB", "Memory of one server. If the process needs more, it is killed (out of memory), drops everything in flight and restarts. 0 = not modelled.").adv(),
				num("maxConns", "Max connections", "Capacity", 0, 0, 1000000, 100, "", "Long-lived connections one server holds; beyond it new ones are refused. 0 = no limit besides memory.").adv(),
				num("memPerConnKB", "Memory per connection", "Failure modes", 0, 0, 10000, 10, "KB", "Memory each open connection takes. With a memory limit, this sets how many connections fit. 0 = not modelled.").adv(),
				sel("sessionState", "Where login sessions live", "Sessions", "none", "Needs the users to have identities (Distinct users). None: the server keeps nothing between requests. Local: the server remembers the users who signed in through it, and forgets them all when it crashes; a request that reaches another server finds the user signed out. Shared: sessions are kept in a connected Redis, found from any server, at the cost of a call per request and the store's own failures.",
					opt("none", "None: stateless"), opt("local", "In this server's memory"), opt("shared", "In a shared store (Redis)")).adv(),
				num("sessionTtlSec", "Session lifetime when idle", "Sessions", 0, 0, 3600, 1, "s", "Soft state: a session not used for this long disappears. 0 = kept until it is lost in a crash.").adv().showIf("sessionState", "local", "shared"),
				num("loginMs", "Sign-in time", "Sessions", 30, 1, 2000, 1, "ms", "Time to sign a user in the first time. A user who finds their session gone is refused and must sign in again.").adv().showIf("sessionState", "local", "shared"),
				sel("authMode", "Check who is calling", "Security", "none", "Every request carries a token. Auth service: ask the connected auth service each time (or once per user while the answer is remembered). Local signature: verify the token's signature on this server, with no call, but a revoked token is not noticed until it expires.",
					opt("none", "No check"), opt("service", "Ask the auth service"), opt("localSignature", "Verify the signature here")).adv(),
				num("verifyMs", "Signature check", "Security", 0.3, 0.05, 20, 0.05, "ms", "CPU time to verify one token's signature.").adv().showIf("authMode", "localSignature"),
				num("tokenUsers", "Signed-in users", "Security", 1000, 1, 1000000, 100, "", "How many different users hold a token. A remembered answer is kept per user.").adv().showIf("authMode", "service"),
				num("tokenCacheSec", "Remember a valid token for", "Security", 0, 0, 3600, 1, "s", "After the auth service says a token is valid, skip asking again for this long. A token revoked in the meantime is still accepted. 0 = ask every time.").adv().showIf("authMode", "service"),
				sel("authFailMode", "If the auth service fails", "Security", "closed", "Closed: refuse the request. Open: let it through unchecked and count it, which keeps the site up and accepts anyone for as long as the service is down.",
					opt("closed", "Fail closed"), opt("open", "Fail open")).adv().showIf("authMode", "service"),
				num("tlsMs", "TLS handshake", "Security", 0, 0, 500, 1, "ms", "Time to set up an encrypted connection (key exchange). 0 = not modelled.").adv(),
				num("newConnPct", "Requests on a new connection", "Security", 100, 0, 100, 1, "%", "Share of requests that open a new connection and pay the handshake; the rest reuse an open one.").adv(),
				num("tlsResumePct", "New connections that resume a session", "Security", 0, 0, 100, 1, "%", "A returning client skips most of the handshake with a saved session: it costs a third.").adv(),
				num("handshakeMs", "Connection setup", "Capacity", 0, 0, 1000, 5, "ms", "Time to set up a new long-lived connection (TLS, authentication).").adv(),
				num("pushPerSec", "Pushes per second", "Capacity", 0, 0, 100, 0.5, "/s", "How many times a second the server pushes something (a score, a price) to every user connected to it. Each push to each connection costs the CPU below, whether or not anyone asked for anything.").adv(),
				num("pushCpuMs", "CPU per push", "Capacity", 1, 0.1, 50, 0.5, "ms", "CPU time to send one push to one connection.").adv(),
				num("memBaseMB", "Base memory", "Failure modes", 200, 0, 65536, 50, "MB", "What an idle server already uses.").adv(),
				num("memPerReqMB", "Memory per request", "Failure modes", 1, 0.1, 500, 0.1, "MB", "Memory held by each request that is running or waiting in the queue.").adv(),
				num("memRestartSec", "Restart time", "Failure modes", 5, 1, 120, 1, "s", "How long the killed server takes to come back.").adv(),
				num("shedAboveQueueMs", "Shed above", "Capacity", 0, 0, 30000, 50, "ms", "When the oldest request in the queue has waited longer than this, ordinary requests are turned away at the door (the users' urgent ones are still admitted), so the queue stays short and urgent work stays fast. 0 = never.").adv(),
				sel("concurrencyLimit", "Concurrency limit", "Capacity", "fixed", "Fixed: every thread may run at once. Adaptive: the server lets only a limit of requests run at once (never more than its threads), and finds the limit itself. It watches how long each request holds a thread; while that stays near the best time it has seen it raises the limit (doubling at first, then by 1 every quarter second), and when it grows past 1.3 times that time it lowers the limit by 10%. Requests over the limit wait in the queue below. Helps when a dependency slows down as more work reaches it at once (see the database's contention). The current limit is shown in the node's stats.",
					opt("fixed", "Fixed (threads)"), opt("adaptive", "Adaptive (AIMD)")).adv(),
				flag("propagateDeadline", "Skip expired work", "Calls to dependencies", false, "Requests carry the time their client will give up. A request still waiting in a queue after that is skipped instead of served, so the server's capacity goes to requests somebody is still waiting for.").adv(),
				num("bulkheadThreads", "Bulkhead", "Calls to dependencies", 0, 0, 64, 1, "", "At most this many of the server's threads may be waiting on any one dependency at once; further calls to it are refused immediately, so one slow dependency cannot hold every thread. 0 = no limit.").adv(),
				num("callTimeoutMs", "Call timeout", "Calls to dependencies", 0, 0, 10000, 50, "ms", "Give up on a dependency call after this long (0 = wait forever)."),
				num("connectMs", "Connection setup", "Calls to dependencies", 0, 0, 2000, 10, "ms", "Time to open a new database connection (DNS, TCP, TLS, login). Only a request that has to open one pays it; reused connections do not, and a restarted server starts with none open. 0 = not modelled.").adv().needs(KDatabase, KReplica),
				num("dbPool", "DB pool size", "Calls to dependencies", 20, 1, 200, 1, "", "Max connections this server opens to each database: its {{connection-pool|connection pool}}.").needs(KDatabase, KReplica, KShardRouter),
				flag("circuitBreaker", "Circuit breaker", "Calls to dependencies", false, "A {{circuit-breaker|circuit breaker}}: after 5 failures in a row to a dependency, fail fast for 5 s, then try once."),
				sel("readSpread", "Reads over several replicas", "Data access", "first", "With more than one read replica, every read goes to the first by default. Round robin gives each replica a turn.",
					opt("first", "The first replica"), opt("roundRobin", "Take turns")).adv().needs(KReplica),
				num("hedgeReadsMs", "Hedge reads after", "Data access", 0, 0, 5000, 5, "ms", "If a read to a replica has not been answered after this long, ask another replica too and take whichever answers first. It avoids a replica's slow tail at the cost of extra load. 0 = never.").adv().needs(KReplica),
				sel("readRouting", "Read routing", "Data access", "nearest", "Where reads go: nearest copy (cache, replica, primary), by freshness, or primary only.",
					opt("nearest", "Nearest copy"), opt("byConsistency", "By freshness"), opt("primary", "Primary only")).needs(KCache, KReplica),
				sel("hotKeyMode", "Hot keys", "Data access", "none", "What the server does about one item that takes a large share of reads. A local cache keeps the hottest items in the server's own memory, so those reads never reach the cache node (at the cost of serving a copy up to the local lifetime old).",
					opt("none", "Nothing"), opt("localCache", "Local cache (reads)"), opt("split", "Split the item over several rows (counters)")).adv(),
				num("splitShards", "Rows the item is split over", "Data access", 8, 2, 64, 1, "", "A write goes to one of these rows, so they take the writes in parallel; a read has to add them all up, so it costs this many times as much.").adv().showIf("hotKeyMode", "split"),
				num("localCacheItems", "Local cache size", "Data access", 8, 1, 1000, 1, "items", "How many items each server keeps locally.").adv().showIf("hotKeyMode", "localCache"),
				num("localCacheTtlSec", "Local cache lifetime", "Data access", 1, 0.1, 60, 0.1, "s", "How long a local copy is served before it is fetched again. A write through this server drops its copy at once; other servers' copies stay until they expire.").adv().showIf("hotKeyMode", "localCache"),
				flag("idempotencyKeys", "Idempotency keys", "Data access", false, "Makes writes {{idempotency|idempotent}}: tag each write with its request id so a retried write is applied once (costs ~1 ms)."),
				flag("bloomGuard", "Bloom filter in front of reads", "Data access", false, "Keep a Bloom filter of every id that exists. A request for an id the filter has never seen is answered 'not found' at once; the filter's false positives (and every real id) go on as usual. Smaller filters take less memory and let more absent ids through.").adv(),
				num("bloomBitsPerKey", "Filter bits per id", "Data access", 10, 1, 64, 1, "bits", "Memory per stored id; the false-positive rate is (1 - e^(-hashes / bits per id))^hashes.").adv().showIf("bloomGuard", "true"),
				num("bloomHashes", "Filter hash functions", "Data access", 7, 1, 16, 1, "", "How many bits each id sets and each lookup checks.").adv().showIf("bloomGuard", "true"),
				sel("hostChoice", "Which external service", "Data access", "all", "With several external services connected: the first one only (as before), or one chosen by the request's key, as a crawler picks the site a page lives on.",
					opt("all", "The first one"), opt("byKey", "One per request, by key")).adv().needs(KExternalAPI),
				num("perHostRps", "Calls per second to one service", "Data access", 0, 0, 1000, 1, "/s", "Politeness: the most calls a second this server sends to any one external service; the rest wait their turn. 0 = no limit.").adv().needs(KExternalAPI),
				sel("workflow", "Writes that span services", "Data access", "none", "With several databases or external services connected, a write is a sequence of steps, one in each, in the order they were connected. Naive: if a later step fails, the earlier ones stay done and the order is left half finished. Saga: the earlier steps are undone by compensating calls, newest first; a compensation that fails leaves the order stuck. Two-phase commit: every service first votes and holds its row, then all commit, so nothing is ever half done, at the price of a second round and of a stall when the server dies between the votes and the decision. Three-phase commit: a pre-commit round is added so the services never wait for a dead server, at the price of a third round.",
					opt("none", "Not applicable"), opt("naive", "Naive: no undo"), opt("saga", "Saga: compensate on failure"), opt("twoPhase", "Two-phase commit"), opt("threePhase", "Three-phase commit")).adv(),
				num("termTimeoutMs", "Wait before deciding alone", "Data access", 1000, 100, 30000, 100, "ms", "Three-phase commit: how long the services wait for a dead coordinator before they decide among themselves. This assumes delays are bounded and the coordinator really is dead: a slow or partitioned coordinator would make them decide wrongly.").adv().showIf("workflow", "threePhase"),
				sel("eventPublish", "Tell others about a write", "Data access", "none", "When a write must also publish an event. Dual write: save to the database, then publish; if the publish fails the event is lost for good. Outbox: the event is saved with the data, atomically, and a relay publishes it until it is accepted.",
					opt("none", "Not applicable"), opt("dualWrite", "Dual write: save, then publish"), opt("outbox", "Outbox: save both together, relay later")).adv().needs(KQueue, KTopic, KLog),
				num("outboxRetryMs", "Outbox retry", "Data access", 500, 50, 10000, 50, "ms", "How long the relay waits before trying again to publish an event the queue refused.").adv().showIf("eventPublish", "outbox"),
				sel("lockService", "Locks come from", "Locks (Redis)", "redis", "Where this server takes locks for bookings. A coordinator (Raft/ZooKeeper style) also issues a fencing token with every lock, larger than any before; a Redis lock does not.",
					opt("redis", "Redis"), opt("coordinator", "Coordinator (with fencing tokens)")).adv(),
				num("lockWaitMs", "Lock wait", "Locks (Redis)", 0, 0, 5000, 25, "ms", "How long to keep retrying a busy lock before giving up (0 = fail fast).").needs(KRedis),
				num("lockTtlMs", "Lock lease (TTL)", "Locks (Redis)", 2000, 100, 30000, 100, "ms", "Locks expire after this long even if never released.").needs(KRedis).adv(),
			},
			cost: func(p Params) float64 {
				c := 0.1 * math.Pow(pf(p, "threads", 8)/8, 1.5)
				if ps(p, "runtime") == "container" {
					c *= 0.8 // containers share a host
				}
				if pb(p, "keepWarm") && pf(p, "coldStartMs", 0) > 0 {
					c += 0.05 // one instance that never stops
				}
				return c
			},
		},
		{
			Kind: KCache, Name: "Cache", Description: "In-memory cache. Misses go on to the data store.",
			Lesson: "Hit rate depends on how much of the popular set fits and what you choose to keep.",
			Accent: "#c2255c", Width: 160, Height: 72, CanConnect: []string{KDatabase, KReplica, KShardRouter},
			Params: []ParamSpec{
				num("capacityItems", "Capacity", "Memory", 2000, 0, 100000, 100, "items", "How many items fit."),
				num("capacityMB", "Capacity in bytes", "Memory", 0, 0, 1000000, 10, "MB", "Memory for cached bodies. Items are as large as the users' response size, so big objects fit fewer; the smaller of this and the item count applies. 0 = only count items.").adv(),
				num("rangeKB", "Range size", "Memory", 0, 0, 102400, 10, "KB", "Cache big objects in pieces of this size, each kept or evicted on its own. A request needs all the pieces of its object; only the missing ones go to the origin (and with sharing of concurrent misses, once each). 0 = whole objects. Needs the users' response size; an object no bigger than one piece is not cut up.").adv(),
				sel("eviction", "Eviction", "Memory", "lru", "Which item to drop when full.",
					opt("lru", "LRU: least recently used"), opt("lfu", "LFU: least frequently used"), opt("fifo", "FIFO: oldest inserted"), opt("random", "Random")),
				sel("writePolicy", "On writes", "Freshness", "none", "What happens to cached copies when data is written. Do nothing is pull: a copy stays until it expires (the TTL). Invalidate and write through are push: every cache is told about every write. A lease tells only the caches that hold the item, since an entry is a lease for as long as its TTL, so it sends fewer messages; the origin would have to remember who holds what, which is not modelled.",
					opt("none", "Do nothing"), opt("invalidate", "Invalidate"), opt("writeThrough", "Write through"), opt("lease", "Lease: tell only caches that hold it")),
				num("ttlSec", "Time to live", "Freshness", 0, 0, 3600, 1, "s", "Entries expire after this long, their {{ttl|time to live}} (0 = never)."),
				clockSkew(),
				ttlJitter(),
				num("shortTtlPct", "Items with a shorter TTL", "Freshness", 0, 0, 100, 5, "%", "Needs a time to live above 0. A share of the items (chosen by key) expire after the shorter time below instead of the time to live: for data that changes at different rates. Server-side invalidation (the write policy) works alongside it.").adv(),
				num("shortTtlSec", "Shorter time to live", "Freshness", 5, 1, 3600, 1, "s", "The time to live of the items in the share above.").adv(),
				flag("revalidate", "Revalidate on expiry", "Freshness", false, "An expired entry asks the origin whether it is still current, which costs about a tenth of a full read, and is renewed if it is; only an item that changed is fetched again.").adv(),
				num("revalidateMs", "Revalidation time", "Freshness", 1, 0.1, 100, 0.1, "ms", "How long that version check takes.").adv(),
				flag("coalesce", "Share concurrent misses", "Freshness", false, "When several requests miss on the same key at once (a {{stampede}}), only one goes to the data store and the rest wait for its answer.").adv(),
				flag("startWarm", "Start warm", "Memory", true, "Begin the run with popular items already cached."),
				num("latencyMs", "Latency", "Speed", 1, 0.1, 50, 0.1, "ms", "Time per lookup.").adv(),
			},
			cost: func(p Params) float64 {
				memory := 0.05 * pf(p, "capacityItems", 2000) / 2000
				if ps(p, "writePolicy") == "lease" {
					memory *= 1.5 // the origin has to remember who holds what: half as much again as the memory itself
				}
				return 0.03 + memory
			},
		},
		{
			// TODO(#17): database scenarios still to handle later: joins (a join penalty for document stores) and cheap
			// range queries by time with downsampling for time-series. The engine has no join or range-query concept yet.
			Kind: KDatabase, Name: "Database", Description: "The primary data store. Handles reads and writes.",
			Lesson: "Connections and query time cap throughput long before disks do.",
			Accent: "#e8590c", Width: 160, Height: 72, CanConnect: []string{KCoordinator, KObjectStore},
			Params: []ParamSpec{
				num("connections", "Concurrent queries", "Capacity", 16, 1, 256, 1, "", "Queries it can work on at once."),
				num("readMs", "Read time", "Speed", 10, 1, 500, 1, "ms", "Time for one read."),
				num("writeMs", "Write time", "Speed", 30, 1, 1000, 1, "ms", "Time for one write."),
				sel("topology", "Leaders", "Replication", "singleLeader", "One leader takes every write. With several leaders each accepts writes for any key and passes them to the others after a delay, so two writes to one key at different leaders inside that delay conflict.",
					opt("singleLeader", "One leader"), opt("multiLeader", "Several leaders")).adv(),
				num("leaders", "Number of leaders", "Replication", 2, 2, 8, 1, "", "How many leaders accept writes.").adv().showIf("topology", "multiLeader"),
				num("syncLagMs", "Time to reach the other leaders", "Replication", 200, 1, 10000, 10, "ms", "How long a write takes to arrive at the other leaders.").adv().showIf("topology", "multiLeader"),
				sel("conflictPolicy", "On a conflict", "Replication", "lastWriterWins", "Last-writer-wins keeps the write with the later clock stamp and silently drops the other (a lost update). Logical versions order writes by what each saw, not by the clock, so only writes that really were concurrent conflict. A merging data type (a counter that adds) keeps both.",
					opt("lastWriterWins", "Last writer wins"), opt("logicalVersion", "Logical versions"), opt("merge", "Merge (counter)")).adv().showIf("topology", "multiLeader"),
				num("clockSpreadMs", "Clock offset between leaders", "Replication", 0, 0, 60000, 100, "ms", "Each leader stamps its writes with its own clock. Leader 2 is this far ahead of leader 1, leader 3 twice as far, and so on. Last-writer-wins keeps the later stamp, so a write at a leader that is behind can lose to an older write at one that is ahead. Only last-writer-wins looks at the clock.").adv().showIf("topology", "multiLeader"),
				num("clockDriftMsPerMin", "Clock drift", "Replication", 0, 0, 10000, 100, "ms/min", "How much further each leader's clock drifts from the first one every minute, until it is synchronised.").adv().showIf("topology", "multiLeader"),
				num("clockSyncSec", "Synchronise clocks every", "Replication", 0, 0, 3600, 1, "s", "Set the drift back to zero this often, as a time protocol does. A fixed offset between the leaders is not removed. 0 = never.").adv().showIf("topology", "multiLeader"),
				num("backupEverySec", "Backup every", "Backups", 0, 0, 3600, 5, "s", "Take a backup this often (0 = never). When something goes wrong a restore goes back to the last backup before the damage, so this is how much data can be lost: the RPO.").adv(),
				num("restoreMBps", "Restore speed", "Backups", 200, 10, 10000, 10, "MB/s", "How fast a backup streams back. With the data size this sets how long the database is unavailable during a restore: the RTO.").adv(),
				num("dataGB", "Data size", "Storage", 0, 0, 100000, 10, "GB", "How much data the database holds. With more data than the buffer pool, a share of reads must go to disk and cost 10x. 0 = not modelled.").adv(),
				num("growthGBPerDay", "Data growth", "Storage", 0, 0, 100000, 1, "GB/day", "How much the data grows each day. Shown as a projected storage cost for the next year and as the days until the data no longer fits in memory (reads then start going to disk).").adv(),
				num("storagePerGBMonth", "Storage price", "Storage", 0.115, 0, 1, 0.005, "$/GB-mo", "Price of keeping one GB for a month.").adv(),
				num("bufferPoolGB", "Memory for data", "Storage", 16, 1, 4096, 1, "GB", "RAM the database can use to keep data pages (its buffer pool or page cache).").adv(),
				sel("model", "Data model", "Storage", "relational", "A preset that summarises what a data model is good at. Relational is the baseline. Document: single-key reads and writes are about 20% cheaper. Wide-column: writes cost 40% of the baseline, but a lookup by anything other than the partition key scans every partition. Time-series: writes cost 30%, random point reads 50% more.",
					opt("relational", "Relational"), opt("document", "Document"), opt("wideColumn", "Wide-column"), opt("timeSeries", "Time-series")).needs().adv(),
				num("contention", "Contention", "Speed", 0, 0, 10, 0.5, "x", "How much the database slows when too many queries run at once (lock waits, cache thrash, context switches). A query that starts while more than the knee are running takes read or write time x (1 + contention x (running - knee) / knee). At 2 with a knee of 8, 24 running queries take 5 times as long, so the database finishes fewer queries a second than with 8: throughput peaks at the knee and then falls. 0 = off.").adv(),
				num("contentionKnee", "Contention knee", "Speed", 8, 1, 256, 1, "", "How many queries can run at once before they start slowing each other down.").adv(),
				num("diskIops", "Disk IOPS", "Speed", 0, 0, 100000, 50, "/s", "Disk operations per second. Each write needs one (plus one per secondary index) and they queue one after another, so writes slow down when the disk cannot keep up. 0 = not modelled.").adv(),
				variability("Speed"),
				sel("onFailure", "If it fails", "Failover", "stayDown", "What happens when this database crashes. Promoting a replica needs a read replica; writes acknowledged but not yet copied to it are lost.",
					opt("stayDown", "Stay down until it restarts"), opt("promote", "Promote a replica")),
				num("detectSec", "Failure detection", "Failover", 10, 1, 120, 1, "s", "How long until the failure is noticed.").showIf("onFailure", "promote"),
				num("promoteSec", "Promotion time", "Failover", 20, 1, 300, 1, "s", "How long the replica takes to become the primary once the failure is noticed.").showIf("onFailure", "promote"),
				sel("electionBy", "Who decides on failover", "Failover", "timer", "A timer promotes the replica on schedule whatever else is going on. With a coordinator (connect the database to one) promotion waits until the coordination service has a leader and a majority: promoting without one is how two primaries appear.",
					opt("timer", "A timer"), opt("coordinator", "A coordinator (needs a connection to one)")).showIf("onFailure", "promote").needs(KCoordinator),
				flag("fencing", "Fence the old primary", "Failover", true, "Keep a recovered old primary out. Without it, the old primary comes back, clients return to it, and every write the new primary took in the meantime is lost.").showIf("onFailure", "promote"),
				num("indexes", "Secondary indexes", "Data", 0, 0, 6, 1, "", "Each index speeds up lookups by another field and makes every write 30% slower (it must update the index too). With none, a lookup by another field scans the table."),
				sel("engine", "Storage engine", "Data", "btree", "How data is laid out on disk. B-tree: balanced reads and writes, no pauses. LSM tree: writes are cheap and sequential (half the write time) but a read may touch several levels (1.5x the read time), and the database periodically pauses writes to compact its files.",
					opt("btree", "B-tree"), opt("lsm", "LSM tree")),
				num("compactionEverySec", "Compaction every", "Data", 30, 5, 600, 5, "s", "How often an LSM database compacts its files.").showIf("engine", "lsm").adv(),
				num("compactionStallMs", "Compaction stalls writes for", "Data", 400, 50, 5000, 50, "ms", "How long writes wait while it compacts. Reads continue.").showIf("engine", "lsm").adv(),
				sel("isolation", "Transaction isolation", "Data", "readCommitted", "What concurrent transactions may see of each other, for the users' Transfers workload. Read committed: no waiting, but two transfers can overwrite each other (a lost update). Repeatable read: the second to commit aborts instead. Serializable: transfers take locks on both accounts, so they wait for each other and can deadlock.",
					opt("readCommitted", "Read committed"), opt("repeatableRead", "Repeatable read (snapshot)"), opt("serializable", "Serializable (locks)")),
				num("scanMult", "Cost of an unindexed lookup", "Data", 20, 1, 200, 1, "x", "A lookup by another field with no index costs the read time times this.").adv(),
				num("maxConnections", "Max connections", "Capacity", 500, 10, 5000, 10, "", "Hard limit on open connections from all servers; beyond it connections are refused."),
				num("queueCap", "Queue size", "Capacity", 100, 0, 5000, 10, "", "Queries that may wait for a free slot."),
				flag("spatialIndex", "Spatial index", "Features", false, "Makes nearby-location searches cheap."),
				flag("hotRowLock", "Row locks on every write", "Features", false, "Writes to the same row run one after another (a row lock), each keeping its database connection while it waits its turn. One very popular row (a stock counter on a launch day) then caps the writes it can take at 1 / write time, and the waiting writes use up the connections everyone else needs.").adv(),
				flag("conditionalWrite", "Book only if still free", "Features", false, "The write itself checks, atomically, that the item is still free (a compare-and-set, like a unique constraint or UPDATE ... WHERE free). A holder that froze past its lock lease then cannot overwrite a booking someone else made meanwhile, whatever its lock or token says.").adv(),
				flag("fencingTokens", "Refuse stale lock tokens", "Features", false, "A write that carries a fencing token older than one this database has already accepted for the item is refused: the holder's lease ran out and someone else has acted since. Needs a lock service that issues tokens (the coordinator).").adv(),
				flag("rowLocks", "Row locks", "Features", false, "Serialize writes per item so two bookings cannot overlap."),
			},
			cost: func(p Params) float64 {
				c := 0.2 * math.Pow(pf(p, "connections", 16)/16, 1.2)
				if pf(p, "backupEverySec", 0) > 0 { // somewhere to keep the backups, and bandwidth to stream them back
					c += 0.02 + 0.0002*pf(p, "restoreMBps", 200)
				}
				return c
			},
		},
		{
			Kind: KReplica, Name: "Read Replica", Description: "A copy of the database that serves reads, slightly behind.",
			Lesson: "Replicas add read capacity at the price of staleness.",
			Accent: "#f08c00", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("connections", "Concurrent queries", "Capacity", 16, 1, 256, 1, "", "Queries it can work on at once."),
				num("readMs", "Read time", "Speed", 10, 1, 500, 1, "ms", "Time for one read."),
				sel("replication", "Replication", "Freshness", "async", "Asynchronous: the primary acknowledges first, so a failover loses the last few writes. Synchronous: it waits for this copy (slower writes, and writes stop if this copy is down) and nothing is lost.",
					opt("async", "Asynchronous"), opt("sync", "Synchronous")),
				num("replicationLagMs", "Replication lag", "Freshness", 100, 0, 30000, 50, "ms", "The {{replication-lag|replication lag}}: how far behind the primary this copy runs. Writes newer than this are lost if it has to take over (a {{failover}}).").showIf("replication", "async"),
				num("writeMs", "Write time", "Speed", 30, 1, 1000, 1, "ms", "Time for one write, once this replica has been promoted to primary.").adv(),
				num("indexes", "Secondary indexes", "Data", 0, 0, 6, 1, "", "Each index speeds up lookups by another field and makes every write 30% slower. With none, a lookup by another field scans."),
				num("scanMult", "Cost of an unindexed lookup", "Data", 20, 1, 200, 1, "x", "A lookup by another field with no index costs the read time times this.").adv(),
				variability("Speed"),
				num("maxConnections", "Max connections", "Capacity", 500, 10, 5000, 10, "", "Hard limit on open connections."),
				num("queueCap", "Queue size", "Capacity", 100, 0, 5000, 10, "", "Queries that may wait."),
				flag("spatialIndex", "Spatial index", "Features", false, "Makes nearby-location searches cheap."),
			},
			cost: func(p Params) float64 { return 0.15 * math.Pow(pf(p, "connections", 16)/16, 1.2) },
		},
		{
			Kind: KQueue, Name: "Message Queue", Description: "Buffers work so servers can answer fast and workers catch up later.",
			Lesson: "A queue turns a spike into a delay, as long as it does not fill up.",
			Accent: "#7048e8", Width: 160, Height: 72, CanConnect: []string{KWorker},
			Params: []ParamSpec{
				num("capacity", "Capacity", "Buffer", 5000, 10, 100000, 100, "jobs", "Jobs it can hold before rejecting."),
				num("maxAttempts", "Attempts per job", "Delivery", 3, 1, 10, 1, "", "How many times a failing job is tried (with a growing pause between tries) before it is given up.").adv(),
				num("visibilityTimeoutMs", "Visibility timeout", "Delivery", 0, 0, 600000, 100, "ms", "A job a worker has taken but not acknowledged within this time becomes visible again and another worker takes it: delivery is at least once, so slow jobs run twice. 0 = off.").adv(),
				flag("priorityQueue", "Urgent first", "Delivery", false, "Hand out urgent jobs (the users' urgent share) before any other job, however long the backlog.").adv(),
				num("delayMs", "Delivery delay", "Delivery", 0, 0, 600000, 100, "ms", "Hold every job back this long before workers may take it (delayed or scheduled delivery). Held jobs still count against the queue's capacity.").adv(),
				flag("deadLetter", "Dead-letter queue", "Delivery", false, "Park jobs that used up their attempts for inspection instead of dropping them. They are counted as dead, not lost.").adv(),
				flag("dedupeIds", "Drop repeated message ids", "Delivery", false, "A message whose id the queue has already accepted is acknowledged but not stored again, as a deduplicating queue does. Without it, a producer that retries after a timeout (an outbox relay, say) can leave the same event in the queue twice.").adv(),
				num("maxMessageKB", "Largest message", "Buffer", 256, 1, 10240, 1, "KB", "Messages above this size are refused (managed queues commonly allow 256 KB; a log, 1 MB). Only applies when the users' request size is set. Store big bodies elsewhere and queue a pointer.").adv().fixed(),
			},
			cost: func(Params) float64 { return 0.04 },
		},
		{
			Kind: KTopic, Name: "Topic", Description: "Publish once, every subscriber gets its own copy.",
			Lesson: "Each subscriber falls behind on its own: a slow one grows a backlog while the others keep up.",
			Accent: "#9c36b5", Width: 160, Height: 72, CanConnect: []string{KWorker},
			Params: []ParamSpec{
				num("capacity", "Backlog per subscriber", "Buffer", 5000, 10, 100000, 100, "messages", "Messages one subscriber may have waiting; beyond it that subscriber's copies are dropped (the publisher is never told)."),
				num("maxAttempts", "Attempts per delivery", "Delivery", 3, 1, 10, 1, "", "How many times a failing delivery is tried before it is given up.").adv(),
				flag("deadLetter", "Dead-letter queue", "Delivery", false, "Park deliveries that used up their attempts instead of dropping them.").adv(),
			},
			cost: func(Params) float64 { return 0.04 },
		},
		{
			Kind: KLog, Name: "Log", Description: "A partitioned, ordered log. Consumer groups read it; a partition has one reader per group.",
			Lesson: "Order is per partition, and parallelism is capped by the number of partitions.",
			Accent: "#c2255c", Width: 160, Height: 72, CanConnect: []string{KWorker},
			Params: []ParamSpec{
				num("partitions", "Partitions", "Layout", 4, 1, 64, 1, "", "A message goes to one partition; one member of a consumer group reads a partition, so more members than partitions sit idle."),
				sel("partitionBy", "Partition by", "Layout", "key", "Key: all messages of one key share a partition (and so stay in order). Round robin: spread evenly, order across a key is lost.",
					opt("key", "Key"), opt("roundRobin", "Round robin")),
				num("capacity", "Backlog per partition", "Buffer", 5000, 10, 100000, 100, "messages", "Messages one partition may hold for a group; beyond it that group's copies are dropped."),
				num("maxAttempts", "Attempts per message", "Delivery", 3, 1, 10, 1, "", "How many times a failing message is tried in place (the partition waits) before it is given up.").adv(),
				flag("deadLetter", "Dead-letter queue", "Delivery", false, "Park messages that used up their attempts instead of dropping them.").adv(),
				sel("ackMode", "Acknowledge when", "Durability", "instant", "Instant: no copies are modelled, an acknowledged message is never lost. Leader: the producer is told the message is safe as soon as the leader has it and the followers copy it a moment later, so a leader crash in that gap loses messages that were acknowledged. All: the acknowledgement waits for the followers' copies, so nothing acknowledged is lost, and every publish takes that much longer.",
					opt("instant", "Instant (no copies modelled)"), opt("leader", "The leader has it"), opt("all", "Every copy has it")).adv(),
				num("retentionSec", "Keep messages for", "Buffer", 0, 0, 86400, 10, "s", "How long the log keeps a message. A group that is away for longer than this (consumers down, or too slow) finds the oldest messages gone when it returns. 0 = forever.").adv(),
				num("leaderElectionSec", "Time for a follower to take over", "Durability", 0, 0, 120, 1, "s", "When the leader dies, publishes fail until a follower has been elected. 0 = no follower takes over: the log stays down until the node is back.").adv().showIf("ackMode", "leader", "all"),
				num("replicationLagMs", "Time to copy to the followers", "Durability", 100, 1, 10000, 10, "ms", "How long the followers take to have a message the leader accepted.").adv().showIf("ackMode", "leader", "all"),
				num("rebalanceSec", "Rebalance pause", "Delivery", 0, 0, 120, 1, "s", "When a member joins or leaves a consumer group, the whole group stops reading for this long while the partitions are handed out again (a stop-the-world rebalance), so the backlog grows for everyone. 0 = the partitions move over instantly.").adv(),
			},
			cost: func(p Params) float64 { return 0.03 * pf(p, "partitions", 4) / 4 },
		},
		{
			Kind: KWorker, Name: "Worker", Description: "Pulls jobs from a queue and does the slow work.",
			Lesson: "Workers set how fast the backlog drains.",
			Accent: "#0b7285", Width: 160, Height: 72,
			CanConnect: []string{KDatabase, KShardRouter, KExternalAPI, KRedis, KCache},
			Params: []ParamSpec{
				num("concurrency", "Concurrency", "Capacity", 4, 1, 64, 1, "", "Jobs it works on at once."),
				num("processMs", "Processing time", "Speed", 10, 1, 2000, 1, "ms", "Compute per job."),
				variability("Speed"),
				num("group", "Consumer group", "Log", 1, 1, 8, 1, "", "When fed by a log: workers with the same group number share its partitions (each partition has one reader per group); a different number is a separate group that reads every message.").adv(),
				num("poisonPct", "Poison jobs", "Failure modes", 0, 0, 100, 1, "%", "Share of jobs that can never succeed (a malformed message). Each is retried until its attempts run out, using worker time every time.").adv(),
			},
			cost: func(p Params) float64 { return 0.05 * pf(p, "concurrency", 4) / 4 },
		},
		{
			Kind: KCDN, Name: "CDN", Description: "Serves popular reads from the edge, close to users.",
			Lesson: "The cheapest request is the one that never reaches your servers.",
			Accent: "#087f5b", Width: 160, Height: 72, CanConnect: []string{KLoadBalancer, KAppServer, KRateLimiter, KCDN},
			Params: []ParamSpec{
				num("capacityItems", "Capacity", "Memory", 5000, 0, 200000, 100, "items", "How many items it can hold."),
				num("capacityMB", "Capacity in bytes", "Memory", 0, 0, 1000000, 10, "MB", "Disk or memory for cached bodies. Items are as large as the users' response size, so big objects fit fewer; the smaller of this and the item count applies. 0 = only count items.").adv(),
				num("rangeKB", "Range size", "Memory", 0, 0, 102400, 10, "KB", "Cache big objects in pieces of this size, each kept or evicted on its own. A request needs all the pieces of its object; only the missing ones go to the origin (and with sharing of concurrent misses, once each). 0 = whole objects. Needs the users' response size; an object no bigger than one piece is not cut up.").adv(),
				num("ttlSec", "Time to live", "Freshness", 60, 0, 3600, 1, "s", "Entries expire after this long."),
				clockSkew(),
				ttlJitter(),
				flag("startWarm", "Start warm", "Memory", true, "Begin with popular items cached."),
				flag("coalesce", "Collapse concurrent misses", "Freshness", false, "A shield tier: when several requests miss on the same key at once, only one goes on to the origin and the rest wait for its answer.").adv(),
			},
			cost: func(Params) float64 { return 0.1 },
		},
		{
			Kind: KRateLimiter, Name: "Rate Limiter", Description: "Rejects traffic above a configured rate.",
			Lesson: "Shedding load early protects what is behind it.",
			Accent: "#862e9c", Width: 160, Height: 72, CanConnect: []string{KLoadBalancer, KAppServer},
			Params: []ParamSpec{
				sel("unit", "Count", "Limit", "requests", "Requests: every request costs one. Tokens: a request costs as many units as it carries tokens (the size of its prompt), so a few huge requests use the allowance up faster than many small ones.",
					opt("requests", "Requests"), opt("tokens", "Tokens (weighted)")).adv(),
				num("limitTokensPerMin", "Allowance", "Limit", 60000, 1000, 100000000, 1000, "tokens/min", "Tokens allowed through per minute, refilled steadily; the burst is a full minute's worth.").showIf("unit", "tokens"),
				num("limitRps", "Sustained rate", "Limit", 200, 1, 10000, 10, "rps", "Requests per second allowed through."),
				num("burst", "Burst", "Limit", 50, 1, 5000, 10, "", "Extra requests allowed in a short burst."),
				flag("shared", "Share the count", "Limit", false, "Several limiter instances normally each keep their own count, so a fleet of N admits N times the limit. With this on, every limiter that has it on counts into one shared allowance (as if the counters lived in a common store), so the fleet keeps to the limit. The store's own latency and races are not modelled.").adv(),
				sel("algorithm", "Algorithm", "Limit", "tokenBucket", "Token bucket: a steady refill with a burst allowance. Leaky bucket: lets requests out at exactly the sustained rate, queueing a burst (up to the burst size) instead of turning it away. Fixed window: a count that resets at each window edge (a burst on either side of the edge can double the limit). Sliding window log: exact, never more than the limit in any window, remembers every request. Sliding window counter: weights the previous window by how much of it still overlaps (cheap and close).",
					opt("tokenBucket", "Token bucket"), opt("leakyBucket", "Leaky bucket (smooths, queues)"), opt("fixedWindow", "Fixed window"), opt("slidingWindowLog", "Sliding window log"), opt("slidingWindowCounter", "Sliding window counter")).adv(),
				num("windowSec", "Window", "Limit", 1, 1, 60, 1, "s", "Length of the window for the window algorithms; the limit is the sustained rate times this many seconds.").adv().showIf("algorithm", "fixedWindow", "slidingWindowLog", "slidingWindowCounter"),
				sel("keyBy", "Limit per", "Limit", "none", "None: one limit for all traffic together. User: each account gets its own allowance (needs the users to have distinct accounts), so a heavy account is throttled without turning others away.",
					opt("none", "Everyone together"), opt("user", "Each user"), opt("tenant", "Each tenant (group of users)")).adv(),
			},
			cost: func(Params) float64 { return 0.01 },
		},
		{
			Kind: KShardRouter, Name: "Shard Router", Description: "Splits data across several databases by key.",
			Lesson: "Sharding multiplies capacity but makes cross-shard work hard.",
			Accent: "#5f3dc4", Width: 160, Height: 72, CanConnect: []string{KDatabase, KRedis},
			Params: []ParamSpec{
				sel("hashing", "Key placement", "Routing", "mod", "How a key picks its shard. Hash mod n is simple, but adding or removing a shard remaps almost every key. A consistent-hashing ring puts each shard at several points on a circle and sends a key to the next point, so a new shard takes over only about 1/n of the keys.",
					opt("mod", "Hash mod n"), opt("ring", "Consistent hashing ring")).adv(),
				num("vnodes", "Points per shard", "Routing", 64, 1, 512, 1, "", "How many points each shard has on the ring. More points even the load out; with few, one shard can own a much bigger arc.").adv().showIf("hashing", "ring"),
				num("rebalanceSec", "Time to move the data", "Routing", 0, 0, 600, 5, "s", "When a shard is added or removed, how long the data of the keys that changed shard takes to move. While it moves, a request for one of those keys is slower (it is fetched from the old shard). 0 = the move is instant.").adv(),
				num("rebalanceMs", "Extra time for a moving key", "Routing", 200, 0, 5000, 10, "ms", "What a request for a key that is still moving pays on top of its normal time.").adv(),
				sel("shardBy", "Shard by", "Routing", "key", "What decides which shard gets a request. The key keeps one item (or one map cell) on one shard, so a hot one overloads it. Spreading by the request evens the load, but then a read of one key would have to ask every shard, which is not modelled here.",
					opt("key", "The item's key (cell, row)"), opt("request", "The request itself (spread evenly)")).adv(),
			},
			cost: func(Params) float64 { return 0.02 },
		},
		{
			Kind: KExternalAPI, Name: "External API", Description: "A third-party service you do not control.",
			Lesson: "Vendors fail. Budget for it.",
			Accent: "#495057", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("latencyMs", "Latency", "Behavior", 80, 1, 5000, 5, "ms", "Typical response time."),
				variability("Behavior"),
				num("errorPct", "Error rate", "Behavior", 1, 0, 100, 1, "%", "Share of calls that fail."),
				num("politeRps", "Calls it accepts", "Behavior", 0, 0, 1000, 1, "/s", "A site that protects itself: past this many calls a second it answers 429 (rate limited). 0 = no limit. A crawler is expected to stay below it.").adv(),
				sel("calledFor", "Called for", "Behavior", "writes", "Which requests call this service.",
					opt("writes", "Writes"), opt("all", "Everything")),
			},
			cost: func(Params) float64 { return 0 },
		},
		{
			Kind: KObjectStore, Name: "Object Store", Description: "Cheap, huge, durable storage for big bodies: uploads, images, video.",
			Lesson: "Put the bytes here and pass a pointer: queues and databases are not made for large files.",
			Accent: "#2b8a3e", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("firstByteMs", "Time to first byte", "Speed", 30, 1, 2000, 5, "ms", "How long before an upload starts being accepted."),
				num("throughputMBps", "Throughput per upload", "Speed", 50, 1, 2000, 5, "MB/s", "How fast one upload's body is written once it has started."),
				num("maxConcurrent", "Uploads at once", "Capacity", 1000, 1, 100000, 10, "", "Uploads it handles at the same time; beyond it, new ones are refused.").adv(),
				num("multipartKB", "Multipart chunk", "Speed", 0, 0, 102400, 100, "KB", "Upload big bodies in parts of this size, several at a time: the store's part of an upload takes the time to first byte plus the number of rounds (parts divided by uploads in parallel, rounded up) times one part's transfer time. The link in front of the store still carries every byte, so it stays the floor. 0 = one upload; a body no bigger than a part is not split.").adv(),
				num("multipartParallel", "Parts in parallel", "Speed", 4, 1, 64, 1, "", "How many parts of one upload are in flight at once.").adv(),
				num("egressPerGB", "Download price", "Cost", 0.09, 0, 1, 0.01, "$/GB", "What each GB served to users costs. Charged from the rate seen so far, so it appears in the hourly cost once downloads run.").adv(),
				num("requestPer1k", "Request price", "Cost", 0.0004, 0, 0.1, 0.0001, "$/1k", "Price per 1,000 requests, uploads and downloads alike.").adv(),
				num("dataGB", "Data stored", "Cost", 0, 0, 10000000, 100, "GB", "How much is kept in the store; charged per GB per month.").adv(),
				num("growthGBPerDay", "Growth", "Cost", 0, 0, 1000000, 10, "GB/day", "How much the stored data grows each day. Shown as a projected storage cost for the next year (a run lasts seconds, so growth is projected, not simulated).").adv(),
				num("storagePerGBMonth", "Storage price", "Cost", 0.023, 0, 1, 0.001, "$/GB-mo", "Price of keeping one GB for a month.").adv(),
			},
			cost: func(Params) float64 { return 0.03 },
		},
		{
			Kind: KKVStore, Name: "Key-Value Store", Description: "A cluster of nodes that keeps several copies of every key and answers when enough of them agree.",
			Lesson: "Writes wait for W copies and reads for R: if R + W is more than the number of copies, a read always meets the latest write.",
			Accent: "#9c36b5", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("nodes", "Nodes", "Cluster", 5, 1, 64, 1, "", "How many machines are in the cluster. Keys are spread over them."),
				num("replicas", "Copies (N)", "Cluster", 3, 1, 7, 1, "", "How many nodes hold a copy of each key."),
				num("readQuorum", "Read quorum (R)", "Consistency", 2, 1, 7, 1, "", "A read is answered once this many copies have replied, with the newest version among them."),
				num("writeQuorum", "Write quorum (W)", "Consistency", 2, 1, 7, 1, "", "A write is acknowledged once this many copies have it. With R + W above N, every read overlaps the latest write."),
				num("readMs", "Read time", "Speed", 3, 0.1, 500, 0.5, "ms", "Time one node takes to read a key."),
				num("writeMs", "Write time", "Speed", 5, 0.1, 500, 0.5, "ms", "Time one node takes to store a key."),
				num("concurrency", "Requests per node", "Capacity", 8, 1, 1024, 1, "", "How many requests one node works on at once; the rest wait.").adv(),
				flag("sloppyQuorum", "Sloppy quorum", "Repair", false, "When nodes that should hold a key are down, the next healthy nodes take its writes so they still succeed. Those stand-ins hold the write for the down node; reads of the key do not see it until it is handed back.").adv(),
				flag("hintedHandoff", "Hinted handoff", "Repair", false, "A stand-in keeps the writes meant for a node that is down and hands them over when it returns, so it is not left holding old data.").adv(),
				sel("failureDetection", "How nodes learn a node is down", "Cluster", "instant", "Instant: every node knows at once. Gossip: the node that notices tells a few others each round, and they tell more, so the news reaches all in about log N rounds with few messages per node. Broadcast: the node that notices tells every other node, quick but a message to each. Until a node has heard, requests it receives are sent to the dead node and wait for a reply that never comes, and no stand-in keeps the write (so no hint is kept for it either).",
					opt("instant", "Everyone knows at once"), opt("gossip", "Gossip"), opt("broadcast", "Tell every node")).adv(),
				num("detectMs", "Time to notice a failure", "Cluster", 500, 10, 60000, 10, "ms", "How long the first node takes to notice that a peer is down.").adv().showIf("failureDetection", "gossip", "broadcast"),
				num("gossipIntervalMs", "Gossip round every", "Cluster", 1000, 50, 60000, 50, "ms", "How often each node that knows the news tells others.").adv().showIf("failureDetection", "gossip"),
				num("gossipFanout", "Peers told per round", "Cluster", 2, 1, 8, 1, "", "How many other nodes each node tells every round.").adv().showIf("failureDetection", "gossip"),
				num("broadcastMs", "Time to tell every node", "Cluster", 10, 1, 1000, 1, "ms", "How long the message takes to reach all the other nodes.").adv().showIf("failureDetection", "broadcast"),
				num("probeTimeoutMs", "Wait for a node that does not answer", "Cluster", 1000, 10, 30000, 10, "ms", "How long a request waits for a copy on a dead node it did not know about before it gives up.").adv().showIf("failureDetection", "gossip", "broadcast"),
				num("antiEntropySec", "Repair round every", "Repair", 0, 0, 3600, 1, "s", "Every this many seconds the nodes compare what they hold (cheaply, with hash trees) and bring older copies up to date, so a node back from an outage catches up even if nobody reads its keys and no hints were kept. 0 = off.").adv(),
				flag("readRepair", "Read repair", "Repair", false, "A read that finds a copy older than the newest it saw writes the newest back to it.").adv(),
				flag("siblings", "Keep conflicting writes as siblings", "Conflicts", false, "Off: when two clients write a key without having seen each other's write, the later one silently replaces the other (last write wins) and an update is lost. On: every write carries a version vector, concurrent writes are both kept as siblings, a read returns all of them, and a client that has read them and writes again resolves them.").adv(),
				sel("placement", "Key placement", "Cluster", "mod", "How a key picks its nodes. Hash mod n is simple, but changing the number of nodes remaps almost every key. A consistent-hashing ring puts each node at several points on a circle, so adding or removing a node moves only about 1/n of the keys.",
					opt("mod", "Hash mod n"), opt("ring", "Consistent hashing ring")).adv(),
				num("vnodes", "Points per node", "Cluster", 100, 1, 512, 1, "", "How many points each node has on the ring. More points even the load out.").adv().showIf("placement", "ring"),
				variability("Speed"),
			},
			cost: func(p Params) float64 { return 0.1 * pf(p, "nodes", 5) },
		},
		{
			Kind: KCoordinator, Name: "Coordinator", Description: "The simulator models a leader-based cluster, as Raft does: writes wait for a majority. Real ZooKeeper may serve reads from a follower, so they can be stale. It answers nothing without a leader.",
			Lesson: "Losing the leader costs an election; losing the majority costs the service. An odd number of nodes tolerates (n-1)/2 failures.",
			Accent: "#364fc7", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("nodes", "Nodes", "Cluster", 3, 1, 9, 2, "", "How many nodes. A majority must be up for a leader to exist: three tolerate one failure, five tolerate two."),
				num("electionTimeoutMs", "Election timeout", "Cluster", 1000, 50, 20000, 50, "ms", "How long a node waits without hearing from the leader before it stands for election (randomised between one and two times this). Shorter recovers sooner but risks needless elections on a slow network."),
				num("heartbeatMs", "Heartbeat", "Cluster", 150, 10, 5000, 10, "ms", "How often the leader tells the others it is alive.").adv(),
				num("rttMs", "Round trip between nodes", "Cluster", 2, 0.1, 500, 0.5, "ms", "Every write waits for a majority to acknowledge, so it costs about one round trip.").adv(),
				num("stepDownSec", "Cut-off leader steps down after", "Cluster", 0, 0, 60, 0.1, "s", "When the leader is cut off from the others it does not know yet. For this long it goes on acknowledging writes that no other member has, and they are lost when the others elect a new leader. 0 = it stops answering at once.").adv(),
				sel("faultModel", "Faults to tolerate", "Cluster", "crash", "Crash: members fail by stopping, so a majority is enough, and a member that gives a wrong answer is believed. Arbitrary: members may give wrong answers, so the group waits for all but k replies and needs k+1 matching ones; that takes at least 3k+1 members, and one more round. Only reads and writes are voted on, not lock grants.",
					opt("crash", "Crashes (2k+1 members)"), opt("arbitrary", "Arbitrary answers (3k+1 members)")).adv(),
				num("faultsTolerated", "Faulty members to tolerate (k)", "Cluster", 1, 1, 4, 1, "", "How many members may give wrong answers. The group needs 3k+1 members; fewer, and it cannot tell the truth from a lie and refuses to answer.").adv().showIf("faultModel", "arbitrary"),
				flag("answerChecksums", "Checksum every answer", "Cluster", false, "A damaged answer is seen and dropped, as if the member had not answered. A member that lies sends a matching checksum, so this does not help against lies.").adv(),
				num("opMs", "Time per operation", "Speed", 1, 0.1, 100, 0.5, "ms", "Work done by the leader for each request.").adv(),
				variability("Speed"),
			},
			cost: func(p Params) float64 { return 0.08 * pf(p, "nodes", 3) },
		},
		{
			Kind: KStream, Name: "Stream Job", Description: "Counts events in windows of event time (when they happened, not when they arrived), with a watermark that decides when a window is final.",
			Lesson: "Events arrive late and out of order: the watermark trades waiting for completeness against answering on time.",
			Accent: "#087f5b", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("windowSec", "Window", "Windows", 10, 1, 3600, 1, "s", "Length of each window of event time."),
				num("slideSec", "Slide", "Windows", 0, 0, 3600, 1, "s", "How often a new window starts. 0 = the same as the window (tumbling windows, each event counted once); a smaller value gives overlapping sliding windows, and an event is counted in window / slide of them.").adv(),
				num("watermarkDelaySec", "Watermark delay", "Windows", 2, 0, 600, 1, "s", "The watermark is the newest event time seen minus this. A window is emitted when the watermark passes its end: a longer delay waits for stragglers, a shorter one answers sooner."),
				num("allowedLatenessSec", "Allowed lateness", "Windows", 0, 0, 3600, 1, "s", "How long after a window was emitted a late event still updates it (and the window is emitted again). After that the event is dropped.").adv(),
				sel("delivery", "Delivery", "Delivery", "atLeastOnce", "At-least-once: after a crash the windows that fired since the last checkpoint fire again, so the sink sees duplicates. Exactly-once: outputs are committed with each checkpoint so a crash repeats nothing, at the cost of a little more time per event.",
					opt("atLeastOnce", "At-least-once"), opt("exactlyOnce", "Exactly-once")),
				num("checkpointSec", "Checkpoint every", "Delivery", 10, 1, 600, 1, "s", "How often the job saves its place. After a crash it restarts from the last one.").adv(),
				sel("recovery", "Recovery", "Delivery", "checkpoint", "Checkpoint only: after a crash the job reloads its last checkpoint and handles the events since again, at full cost. Message log: every event is written to a log first, so the job replays the log after a crash, faster and without repeating any output, at the price of a log write per event.",
					opt("checkpoint", "Checkpoint only"), opt("messageLog", "Message log")).adv(),
				num("logMs", "Log write", "Delivery", 1, 0.1, 50, 0.1, "ms", "Time to write one event to the log.").adv().showIf("recovery", "messageLog"),
				num("replayMs", "Replay per event", "Delivery", 0.2, 0.01, 10, 0.01, "ms", "Time to replay one logged event during recovery.").adv().showIf("recovery", "messageLog"),
				num("lateEventPct", "Late events", "Input", 0, 0, 100, 1, "%", "Share of events that happened earlier than they arrive (a phone that was offline).").adv(),
				num("lateBySec", "Late by", "Input", 5, 0, 3600, 1, "s", "How much earlier than their arrival the late events happened.").adv(),
				sel("counting", "Count different visitors", "Counting", "none", "Also count how many different visitors (by id) the job has seen. Exactly: a table holds every id, which grows with the audience. HyperLogLog: a few thousand registers whatever the audience, with a small error.",
					opt("none", "Not counting"), opt("exact", "Exactly (a table of ids)"), opt("hyperLogLog", "HyperLogLog (approximate)")),
				num("hllPrecision", "HyperLogLog precision", "Counting", 10, 4, 16, 1, "bits", "The sketch has 2^precision registers (one byte each); its error is about 1.04 / the square root of that: 3% at 10 bits, 0.8% at 14.").showIf("counting", "hyperLogLog"),
				sel("topK", "Find the most popular items", "Counting", "none", "Also find the ten items (by id) that occur most. Exactly: a counter for every item seen, which grows with the number of different items. Count-min sketch: a fixed table of counters shared by all items plus a short list of the best candidates, in far less memory and with some risk of missing one.",
					opt("none", "Not tracking"), opt("exact", "Exactly (a counter per item)"), opt("countMin", "Count-min sketch")),
				num("sketchWidth", "Sketch width", "Counting", 1000, 10, 100000, 10, "counters", "Counters in each row of the sketch. More width means fewer items sharing a counter, so less overcounting.").showIf("topK", "countMin"),
				num("sketchDepth", "Sketch depth", "Counting", 3, 1, 10, 1, "rows", "Rows of counters, each with its own hash. An item's estimate is its smallest counter, so more rows cut the chance that every one of them is inflated by another item.").showIf("topK", "countMin"),
				num("opMs", "Time per event", "Speed", 1, 0.1, 100, 0.5, "ms", "Work to take in one event.").adv(),
			},
			cost: func(Params) float64 { return 0.1 },
		},
		{
			Kind: KLLMServer, Name: "LLM Server", Description: "GPUs that read a prompt and then write the answer one token at a time, many answers sharing each step.",
			Lesson: "Time to first token is the queue plus the prompt; the rest of the answer is a stream whose speed depends on how many share the GPU.",
			Accent: "#7048e8", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("gpus", "GPUs", "Capacity", 1, 1, 64, 1, "", "Each GPU serves its own batch; all share one queue."),
				num("batchSize", "Answers at once per GPU", "Capacity", 8, 1, 256, 1, "", "How many answers one GPU writes together (continuous batching). A bigger batch gives more tokens per second but each answer is written more slowly."),
				num("kvCacheTokens", "KV cache per GPU", "Capacity", 32000, 1000, 10000000, 1000, "tokens", "Memory for the tokens of every answer in progress (prompt plus answer, reserved when it starts). A GPU takes no new answer that does not fit, whatever its batch size allows."),
				num("promptTokens", "Typical prompt", "Requests", 500, 1, 200000, 50, "tokens", "Tokens in a request's prompt; the GPU reads them before it writes anything."),
				num("outputTokens", "Typical answer", "Requests", 150, 1, 100000, 10, "tokens", "Tokens in the answer."),
				num("tokenSpreadPct", "Size spread", "Requests", 0, 0, 90, 5, "%", "Requests differ in size by up to this much either way of the typical sizes.").adv(),
				num("prefillTokPerSec", "Prompt reading speed", "Speed", 4000, 100, 200000, 100, "tok/s", "Prompt tokens one GPU reads a second. While it reads, the GPU's other answers wait.").adv(),
				num("decodeBaseMs", "Step time", "Speed", 20, 1, 500, 1, "ms", "Time of one decode step with a single answer in the batch.").adv(),
				num("decodePerSeqMs", "Added step time per answer", "Speed", 0.5, 0, 50, 0.1, "ms", "Each further answer in the batch makes every step this much longer.").adv(),
				num("queueCap", "Waiting requests", "Capacity", 200, 1, 100000, 10, "", "Requests that may wait for a place in a batch; beyond it they are refused.").adv(),
				num("gpuHourly", "Price per GPU", "Cost", 2.5, 0, 100, 0.1, "$/hr", "What one GPU costs for an hour.").adv(),
			},
			cost: func(p Params) float64 { return pf(p, "gpus", 1) * pf(p, "gpuHourly", 2.5) },
		},
		{
			Kind: KVectorStore, Name: "Vector Store", Description: "Finds the stored items whose embeddings are nearest to a query's, approximately.",
			Lesson: "Approximate search trades recall for speed: looking at more candidates finds more of the true neighbours and takes longer.",
			Accent: "#9c36b5", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("vectors", "Stored vectors", "Index", 1, 0.01, 10000, 1, "millions", "How many embeddings the index holds."),
				num("dims", "Dimensions", "Index", 768, 8, 8192, 64, "", "Numbers per embedding. Memory is vectors x dimensions x 4 bytes, plus about half again for the graph that makes search fast."),
				num("efSearch", "Candidates examined", "Search", 64, 8, 2048, 8, "", "How many candidates a search examines. More finds more of the true nearest neighbours (recall: 1 - 0.5^(this / 32)) and takes longer."),
				num("searchBaseMs", "Fixed time per search", "Speed", 4, 0.1, 500, 0.5, "ms", "Time for every search whatever it examines.").adv(),
				num("perCandidateMs", "Time per candidate", "Speed", 0.05, 0.001, 5, 0.01, "ms", "Time to examine one candidate.").adv(),
				num("connections", "Searches at once", "Capacity", 32, 1, 1024, 1, "", "Searches it works on at the same time.").adv(),
				num("queueCap", "Waiting searches", "Capacity", 200, 1, 100000, 10, "", "Searches that may wait for a free slot.").adv(),
				num("gbPrice", "Price per GB of memory", "Cost", 0.4, 0, 10, 0.05, "$/GB-hr", "What holding the index in memory costs: the index must fit in RAM to be fast.").adv(),
			},
			cost: func(p Params) float64 { // the memory it holds, plus the cores that search
				return pf(p, "vectors", 1)*1e6*pf(p, "dims", 768)*4*1.5/1e9*pf(p, "gbPrice", 0.4)/24*0.6 + 0.05*pf(p, "connections", 32)
			},
		},
		{
			Kind: KIndex, Name: "Search Shard", Description: "One slice of a search index. A query has to ask every slice.",
			Lesson: "Wait for all of 16 shards and you get the slowest of 16: tail latency at scale.",
			Accent: "#0b7285", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("queryMs", "Query time", "Speed", 30, 1, 2000, 1, "ms", "Average time for this shard to answer one query."),
				variability("Speed"),
				num("connections", "Concurrent queries", "Capacity", 16, 1, 256, 1, "", "Queries it works on at once."),
				num("queueCap", "Queue size", "Capacity", 50, 0, 2000, 10, "", "Queries that may wait for a free slot; beyond it they are refused."),
			},
			cost: func(Params) float64 { return 0.05 },
		},
		{
			Kind: KAuth, Name: "Auth Service", Description: "Says whether a token is valid.",
			Lesson: "If every request asks it, it is on the critical path of everything, and when it is down you choose between refusing everyone and letting everyone in.",
			Accent: "#862e9c", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("checkMs", "Check time", "Speed", 15, 1, 2000, 1, "ms", "Average time to check one token."),
				variability("Speed"),
				num("connections", "Concurrent checks", "Capacity", 16, 1, 256, 1, "", "Checks it works on at once."),
				num("queueCap", "Queue size", "Capacity", 50, 0, 2000, 10, "", "Checks that may wait for a free slot; beyond it they are refused."),
			},
			cost: func(Params) float64 { return 0.05 },
		},
		{
			Kind: KRedis, Name: "Redis", Description: "A fast single-threaded store for locks, geo lookups and leaderboards.",
			Lesson: "One thread means commands queue up behind each other.",
			Accent: "#c92a2a", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				sel("persistence", "Persistence", "Durability", "none", "What a crash loses. None: every write since it started. Append-only file: at most the last second.", opt("none", "None (memory only)"), opt("aof", "Append-only file")),
				num("geoSearchMs", "Geo search time", "Speed", 0.4, 0.05, 20, 0.05, "ms", "Time for one nearby search."),
				num("scoreMs", "Update time", "Speed", 0.1, 0.05, 20, 0.05, "ms", "Time for one write (a score change or a location ping). One thread, so this sets how many writes a second one instance can take.").adv(),
				num("queueCap", "Queue size", "Capacity", 2000, 0, 20000, 100, "", "Commands that may wait."),
			},
			cost: func(Params) float64 { return 0.08 },
		},
	}
	for i := range specs { // everything, users included, is somewhere in the world
		specs[i].Params = append(specs[i].Params, sel("region", "Region", "Placement", "us", "Where in the world it runs (for users: where they are). A call between regions pays the distance each way (us-eu 45 ms, us-ap 75 ms, eu-ap 100 ms), and a region outage takes down everything placed in it.",
			opt("us", "US"), opt("eu", "Europe"), opt("ap", "Asia-Pacific")).adv())
	}
	for i := range specs { // every component except the users can be placed in an availability zone
		if specs[i].Kind != KClient {
			specs[i].Params = append(specs[i].Params, sel("zone", "Availability zone", "Placement", "az-a", "Where it runs. A call between two zones pays a hop each way, and a zone outage takes down everything placed in it.",
				opt("az-a", "Zone A"), opt("az-b", "Zone B"), opt("az-c", "Zone C")).adv())
		}
	}
	for i := range specs { // every component except the users has a network card
		if specs[i].Kind != KClient {
			specs[i].Params = append(specs[i].Params, num("nicMbps", "Network card", "Capacity", 0, 0, 100000, 100, "Mbit/s", "How fast this machine can send, shared by everything that leaves it (its requests to other components and its answers to callers): bytes queue behind bytes on one transmitter, so a card smaller than the traffic caps throughput at card speed / message size, whatever the links beyond it can carry. 0 = unlimited. Needs the users' request or response sizes.").adv())
		}
	}
	specByKind = map[string]*NodeSpec{}
	for i := range specs {
		specByKind[specs[i].Kind] = &specs[i]
	}
}

// Specs returns all node specs in display order.
func Specs() []NodeSpec { return specs }

// Spec returns the spec for a kind, or nil.
func Spec(kind string) *NodeSpec { return specByKind[kind] }

// Defaults returns default parameter values for a kind.
func Defaults(kind string) Params {
	out := Params{}
	if s := Spec(kind); s != nil {
		for _, p := range s.Params {
			out[p.Key] = p.Default
		}
	}
	return out
}

// Resolve merges explicit params over the defaults.
func Resolve(kind string, p Params) Params {
	out := Defaults(kind)
	for k, v := range p {
		out[k] = v
	}
	return out
}

// CanConnect reports whether an edge from one kind to another is allowed.
func CanConnect(from, to string) bool {
	s := Spec(from)
	if s == nil {
		return false
	}
	for _, k := range s.CanConnect {
		if k == to {
			return true
		}
	}
	return false
}

// NodeCost is the hourly cost of one node in dollars.
func NodeCost(n Node) float64 {
	s := Spec(n.Kind)
	if s == nil {
		return 0
	}
	p := Resolve(n.Kind, n.Params)
	c := s.cost(p)
	if n.Kind == KAppServer && pb(p, "autoscale") { // a fleet costs at least its minimum number of servers
		c *= math.Max(1, pf(p, "minInstances", 1))
	}
	return c
}

// DiagramCost sums node costs.
func DiagramCost(d Diagram) float64 {
	var c float64
	for _, n := range d.Nodes {
		c += NodeCost(n)
	}
	return c
}

// LatencyRule gives the default one-way latency for edges matching From and To; an empty field
// matches anything. The first matching rule wins, so the last one is the catch-all.
type LatencyRule struct {
	From string  `json:"from,omitempty"`
	To   string  `json:"to,omitempty"`
	Ms   float64 `json:"ms"`
}

var latencyRules = []LatencyRule{
	{From: KClient, To: KCDN, Ms: 10},
	{From: KClient, Ms: 40},
	{From: KCDN, Ms: 40},
	{To: KExternalAPI, Ms: 20},
	{To: KObjectStore, Ms: 20},
	{Ms: 5},
}

// LatencyRules returns the default-latency table, served to clients so they apply the same rules.
func LatencyRules() []LatencyRule { return latencyRules }

// DefaultLatency is the default one-way latency of an edge between two kinds.
func DefaultLatency(from, to string) float64 {
	for _, r := range latencyRules {
		if (r.From == "" || r.From == from) && (r.To == "" || r.To == to) {
			return r.Ms
		}
	}
	return 5
}
