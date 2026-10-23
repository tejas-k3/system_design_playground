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
				sel("arrival", "User behaviour", "Traffic", "open", "Independent users keep arriving whatever happens (a crowd of strangers). Bursty users arrive in short rushes with the same average. A fixed crowd waits for each answer, thinks, then asks again, so it slows down when the system does.",
					opt("open", "Independent users (steady arrivals)"), opt("bursty", "Bursty: short rushes"), opt("closed", "A fixed crowd that waits and thinks"), opt("sessions", "Connected users (long-lived connections)")),
				num("users", "Users", "Traffic", 50, 1, 5000, 10, "", "How many users there are; each has at most one request in flight.").showIf("arrival", "closed"),
				num("thinkMs", "Think time", "Traffic", 1000, 0, 60000, 100, "ms", "Average pause between getting an answer and asking again.").showIf("arrival", "closed"),
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
					opt("transfer", "Transfers: move money between two accounts"), opt("feed", "Feed: posts reach followers, reads fetch a timeline"), opt("oncall", "On-call pairs: read two rows, write one (write skew)")),
				num("followersMean", "Followers per author", "What they do", 100, 1, 100000, 10, "", "Average followers of an author. Counts are Zipf-skewed, so a few authors have far more.").showIf("workload", "feed"),
				num("followersSkew", "Follower skew", "What they do", 1, 0, 2, 0.1, "", "0 = everyone has the same number of followers; higher means a few celebrities hold most of them.").showIf("workload", "feed"),
				sel("keyDist", "Key popularity", "What they do", "zipf", "How requests spread over items.",
					opt("zipf", "Skewed (Zipf)"), opt("uniform", "Uniform"), opt("scan", "Sequential scan")),
				num("keySpace", "Distinct items", "What they do", 10000, 1, 1000000, 100, "", "Size of the item universe."),
				num("zipfS", "Skew", "What they do", 1, 0, 2, 0.1, "", "Zipf exponent: higher means a few items dominate.").showIf("keyDist", "zipf"),
				sel("consistency", "Freshness needed", "Consistency", "eventual", "How fresh a read must be. Strong compares with the latest write by anyone; bounded allows a set age; read-your-writes means a client must see its own writes, monotonic that it never sees data go backwards; causal asks for both. The last three are checked per client.",
					opt("eventual", "Eventual (stale is fine)"), opt("bounded", "Bounded staleness"), opt("strong", "Strong (always fresh)"),
					opt("readYourWrites", "Read your writes"), opt("monotonic", "Monotonic reads"), opt("causal", "Causal: both of those")),
				num("maxStaleMs", "Max staleness", "Consistency", 1000, 0, 60000, 100, "ms", "Reads older than this count as too stale.").showIf("consistency", "bounded"),
				num("hotKeyPct", "Hot item share", "What they do", 0, 0, 100, 1, "%", "Share of requests hitting one single item.").adv(),
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
				flag("outlierEjection", "Eject outliers", "Health", false, "Temporarily remove servers that error a lot or are much slower than their peers."),
				num("canaryPct", "Canary share", "Release", 0, 0, 100, 1, "%", "Send this share of requests to the servers running the new version (v2) and the rest to the old one (v1). 0 = every server gets its fair share.").adv(),
				num("healthCheckSec", "Health check interval", "Health", 2, 1, 30, 1, "s", "How long a dead server keeps receiving traffic before the balancer notices."),
			},
			cost: func(Params) float64 { return 0.02 },
		},
		{
			Kind: KAppServer, Name: "App Server", Description: "Runs your code. Each request holds a thread until it finishes.",
			Lesson: "A thread waiting on a slow database cannot serve anyone else.",
			Accent: "#868e96", Width: 160, Height: 72,
			CanConnect: []string{KCache, KDatabase, KReplica, KQueue, KShardRouter, KExternalAPI, KRedis, KObjectStore, KIndex, KTopic, KLog, KKVStore, KCoordinator},
			Params: []ParamSpec{
				num("threads", "Threads", "Capacity", 8, 1, 64, 1, "", "Concurrent requests it can work on. Bigger machines cost more than proportionally."),
				sel("version", "Release version", "Release", "v1", "Which release this server runs. A load balancer with a canary share sends that share of traffic to v2; a bad-release event can be aimed at one version.", opt("v1", "v1 (current)"), opt("v2", "v2 (new)")).adv(),
				num("cpuMs", "CPU time", "Capacity", 20, 1, 500, 1, "ms", "Compute per request, not counting time waiting on dependencies.").fixed(),
				variability("Capacity"),
				flag("autoscale", "Autoscale", "Scaling", false, "{{autoscaling|Autoscale}}: add and remove whole servers (each with these threads) as load changes. New servers take time to boot, and every running server costs money.").adv(),
				num("minInstances", "Min servers", "Scaling", 1, 1, 32, 1, "", "Never fewer than this many.").adv().showIf("autoscale", "true"),
				num("maxInstances", "Max servers", "Scaling", 8, 1, 32, 1, "", "Never more than this many.").adv().showIf("autoscale", "true"),
				num("targetUtilPct", "Target busy level", "Scaling", 60, 20, 95, 5, "%", "Add servers when threads are busier than this, over the last 30 s.").adv().showIf("autoscale", "true"),
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
				num("handshakeMs", "Connection setup", "Capacity", 0, 0, 1000, 5, "ms", "Time to set up a new long-lived connection (TLS, authentication).").adv(),
				num("memBaseMB", "Base memory", "Failure modes", 200, 0, 65536, 50, "MB", "What an idle server already uses.").adv(),
				num("memPerReqMB", "Memory per request", "Failure modes", 1, 0.1, 500, 0.1, "MB", "Memory held by each request that is running or waiting in the queue.").adv(),
				num("memRestartSec", "Restart time", "Failure modes", 5, 1, 120, 1, "s", "How long the killed server takes to come back.").adv(),
				num("shedAboveQueueMs", "Shed above", "Capacity", 0, 0, 30000, 50, "ms", "When the oldest request in the queue has waited longer than this, ordinary requests are turned away at the door (the users' urgent ones are still admitted), so the queue stays short and urgent work stays fast. 0 = never.").adv(),
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
				sel("eventPublish", "Tell others about a write", "Data access", "none", "When a write must also publish an event. Dual write: save to the database, then publish; if the publish fails the event is lost for good. Outbox: the event is saved with the data, atomically, and a relay publishes it until it is accepted.",
					opt("none", "Not applicable"), opt("dualWrite", "Dual write: save, then publish"), opt("outbox", "Outbox: save both together, relay later")).adv().needs(KQueue, KTopic, KLog),
				num("outboxRetryMs", "Outbox retry", "Data access", 500, 50, 10000, 50, "ms", "How long the relay waits before trying again to publish an event the queue refused.").adv().showIf("eventPublish", "outbox"),
				sel("lockService", "Locks come from", "Locks (Redis)", "redis", "Where this server takes locks for bookings. A coordinator (Raft/ZooKeeper style) also issues a fencing token with every lock, larger than any before; a Redis lock does not.",
					opt("redis", "Redis"), opt("coordinator", "Coordinator (with fencing tokens)")).adv(),
				num("lockWaitMs", "Lock wait", "Locks (Redis)", 0, 0, 5000, 25, "ms", "How long to keep retrying a busy lock before giving up (0 = fail fast).").needs(KRedis),
				num("lockTtlMs", "Lock lease (TTL)", "Locks (Redis)", 2000, 100, 30000, 100, "ms", "Locks expire after this long even if never released.").needs(KRedis).adv(),
			},
			cost: func(p Params) float64 { return 0.1 * math.Pow(pf(p, "threads", 8)/8, 1.5) },
		},
		{
			Kind: KCache, Name: "Cache", Description: "In-memory cache. Misses go on to the data store.",
			Lesson: "Hit rate depends on how much of the popular set fits and what you choose to keep.",
			Accent: "#c2255c", Width: 160, Height: 72, CanConnect: []string{KDatabase, KReplica, KShardRouter},
			Params: []ParamSpec{
				num("capacityItems", "Capacity", "Memory", 2000, 0, 100000, 100, "items", "How many items fit."),
				num("capacityMB", "Capacity in bytes", "Memory", 0, 0, 1000000, 10, "MB", "Memory for cached bodies. Items are as large as the users' response size, so big objects fit fewer; the smaller of this and the item count applies. 0 = only count items.").adv(),
				sel("eviction", "Eviction", "Memory", "lru", "Which item to drop when full.",
					opt("lru", "LRU: least recently used"), opt("lfu", "LFU: least frequently used"), opt("fifo", "FIFO: oldest inserted"), opt("random", "Random")),
				sel("writePolicy", "On writes", "Freshness", "none", "What happens to cached copies when data is written.",
					opt("none", "Do nothing"), opt("invalidate", "Invalidate"), opt("writeThrough", "Write through")),
				num("ttlSec", "Time to live", "Freshness", 0, 0, 3600, 1, "s", "Entries expire after this long, their {{ttl|time to live}} (0 = never)."),
				clockSkew(),
				ttlJitter(),
				flag("coalesce", "Share concurrent misses", "Freshness", false, "When several requests miss on the same key at once (a {{stampede}}), only one goes to the data store and the rest wait for its answer.").adv(),
				flag("startWarm", "Start warm", "Memory", true, "Begin the run with popular items already cached."),
				num("latencyMs", "Latency", "Speed", 1, 0.1, 50, 0.1, "ms", "Time per lookup.").adv(),
			},
			cost: func(p Params) float64 { return 0.03 + 0.05*pf(p, "capacityItems", 2000)/2000 },
		},
		{
			// TODO(#17): database scenarios still to handle later: joins (a join penalty for document stores) and cheap
			// range queries by time with downsampling for time-series. The engine has no join or range-query concept yet.
			Kind: KDatabase, Name: "Database", Description: "The primary data store. Handles reads and writes.",
			Lesson: "Connections and query time cap throughput long before disks do.",
			Accent: "#e8590c", Width: 160, Height: 72, CanConnect: []string{KCoordinator},
			Params: []ParamSpec{
				num("connections", "Concurrent queries", "Capacity", 16, 1, 256, 1, "", "Queries it can work on at once."),
				num("readMs", "Read time", "Speed", 10, 1, 500, 1, "ms", "Time for one read."),
				num("writeMs", "Write time", "Speed", 30, 1, 1000, 1, "ms", "Time for one write."),
				sel("topology", "Leaders", "Replication", "singleLeader", "One leader takes every write. With several leaders each accepts writes for any key and passes them to the others after a delay, so two writes to one key at different leaders inside that delay conflict.",
					opt("singleLeader", "One leader"), opt("multiLeader", "Several leaders")).adv(),
				num("leaders", "Number of leaders", "Replication", 2, 2, 8, 1, "", "How many leaders accept writes.").adv().showIf("topology", "multiLeader"),
				num("syncLagMs", "Time to reach the other leaders", "Replication", 200, 1, 10000, 10, "ms", "How long a write takes to arrive at the other leaders.").adv().showIf("topology", "multiLeader"),
				sel("conflictPolicy", "On a conflict", "Replication", "lastWriterWins", "Last-writer-wins keeps one of the two writes and silently drops the other (a lost update). A merging data type (a counter that adds) keeps both.",
					opt("lastWriterWins", "Last writer wins"), opt("merge", "Merge (counter)")).adv().showIf("topology", "multiLeader"),
				num("backupEverySec", "Backup every", "Backups", 0, 0, 3600, 5, "s", "Take a backup this often (0 = never). When something goes wrong a restore goes back to the last backup before the damage, so this is how much data can be lost: the RPO.").adv(),
				num("restoreMBps", "Restore speed", "Backups", 200, 10, 10000, 10, "MB/s", "How fast a backup streams back. With the data size this sets how long the database is unavailable during a restore: the RTO.").adv(),
				num("dataGB", "Data size", "Storage", 0, 0, 100000, 10, "GB", "How much data the database holds. With more data than the buffer pool, a share of reads must go to disk and cost 10x. 0 = not modelled.").adv(),
				num("growthGBPerDay", "Data growth", "Storage", 0, 0, 100000, 1, "GB/day", "How much the data grows each day. Shown as a projected storage cost for the next year and as the days until the data no longer fits in memory (reads then start going to disk).").adv(),
				num("storagePerGBMonth", "Storage price", "Storage", 0.115, 0, 1, 0.005, "$/GB-mo", "Price of keeping one GB for a month.").adv(),
				num("bufferPoolGB", "Memory for data", "Storage", 16, 1, 4096, 1, "GB", "RAM the database can use to keep data pages (its buffer pool or page cache).").adv(),
				sel("model", "Data model", "Storage", "relational", "A preset that summarises what a data model is good at. Relational is the baseline. Document: single-key reads and writes are about 20% cheaper. Wide-column: writes cost 40% of the baseline, but a lookup by anything other than the partition key scans every partition. Time-series: writes cost 30%, random point reads 50% more.",
					opt("relational", "Relational"), opt("document", "Document"), opt("wideColumn", "Wide-column"), opt("timeSeries", "Time-series")).needs().adv(),
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
				flag("readRepair", "Read repair", "Repair", false, "A read that finds a copy older than the newest it saw writes the newest back to it.").adv(),
				variability("Speed"),
			},
			cost: func(p Params) float64 { return 0.1 * pf(p, "nodes", 5) },
		},
		{
			Kind: KCoordinator, Name: "Coordinator", Description: "A small cluster (Raft or ZooKeeper style) that agrees on a leader. State written through it is strongly consistent, but it answers nothing without a leader.",
			Lesson: "Losing the leader costs an election; losing the majority costs the service. An odd number of nodes tolerates (n-1)/2 failures.",
			Accent: "#364fc7", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("nodes", "Nodes", "Cluster", 3, 1, 9, 2, "", "How many nodes. A majority must be up for a leader to exist: three tolerate one failure, five tolerate two."),
				num("electionTimeoutMs", "Election timeout", "Cluster", 1000, 50, 20000, 50, "ms", "How long a node waits without hearing from the leader before it stands for election (randomised between one and two times this). Shorter recovers sooner but risks needless elections on a slow network."),
				num("heartbeatMs", "Heartbeat", "Cluster", 150, 10, 5000, 10, "ms", "How often the leader tells the others it is alive.").adv(),
				num("rttMs", "Round trip between nodes", "Cluster", 2, 0.1, 500, 0.5, "ms", "Every write waits for a majority to acknowledge, so it costs about one round trip.").adv(),
				num("opMs", "Time per operation", "Speed", 1, 0.1, 100, 0.5, "ms", "Work done by the leader for each request.").adv(),
				variability("Speed"),
			},
			cost: func(p Params) float64 { return 0.08 * pf(p, "nodes", 3) },
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
	for i := range specs { // every component except the users can be placed in an availability zone
		if specs[i].Kind != KClient {
			specs[i].Params = append(specs[i].Params, sel("zone", "Availability zone", "Placement", "az-a", "Where it runs. A call between two zones pays a hop each way, and a zone outage takes down everything placed in it.",
				opt("az-a", "Zone A"), opt("az-b", "Zone B"), opt("az-c", "Zone C")).adv())
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
