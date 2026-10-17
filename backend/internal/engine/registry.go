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
				num("rps", "Requests / sec", "Traffic", 50, 1, 5000, 10, "rps", "Base request rate."),
				sel("pattern", "Traffic pattern", "Traffic", "constant", "Shape of traffic over time; ramp, spike and wave move between base and peak.",
					opt("constant", "Constant"), opt("ramp", "Ramp up"), opt("spike", "Spikes"), opt("wave", "Daily wave")),
				num("peakRps", "Peak requests / sec", "Traffic", 200, 1, 5000, 10, "rps", "Rate reached at the top of a ramp, spike or wave.").showIf("pattern", "ramp", "spike", "wave"),
				num("periodSec", "Period", "Traffic", 20, 2, 300, 1, "s", "Ramp duration, or spike/wave period.").showIf("pattern", "ramp", "spike", "wave"),
				num("readPct", "Reads", "Traffic", 80, 0, 100, 5, "%", "Share of requests that are reads; the rest are writes."),
				num("timeoutMs", "Timeout", "Patience & retries", 2000, 100, 30000, 100, "ms", "Users give up after this long. Abandoned work keeps running downstream."),
				num("retries", "Retries", "Patience & retries", 0, 0, 5, 1, "", "Extra attempts after a failure or timeout. Retries add load exactly when the system is struggling."),
				sel("backoff", "Retry backoff", "Patience & retries", "exponential", "Immediate retries can turn a blip into a retry storm.",
					opt("exponential", "Exponential + jitter"), opt("none", "Immediate")),
				sel("workload", "Workload", "What they do", "kv", "What kind of requests users make.",
					opt("kv", "General: read & write items"), opt("geo", "Location: nearby search & pings"),
					opt("leaderboard", "Leaderboard: rank reads & score updates"), opt("booking", "Booking: reserve scarce seats")),
				sel("keyDist", "Key popularity", "What they do", "zipf", "How requests spread over items.",
					opt("zipf", "Skewed (Zipf)"), opt("uniform", "Uniform"), opt("scan", "Sequential scan")),
				num("keySpace", "Distinct items", "What they do", 10000, 1, 1000000, 100, "", "Size of the item universe."),
				num("zipfS", "Skew", "What they do", 1, 0, 2, 0.1, "", "Zipf exponent: higher means a few items dominate.").showIf("keyDist", "zipf"),
				sel("consistency", "Freshness needed", "Consistency", "eventual", "How stale a read may be.",
					opt("eventual", "Eventual (stale is fine)"), opt("bounded", "Bounded staleness"), opt("strong", "Strong (always fresh)")),
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
				num("healthCheckSec", "Health check interval", "Health", 2, 1, 30, 1, "s", "How long a dead server keeps receiving traffic before the balancer notices."),
			},
			cost: func(Params) float64 { return 0.02 },
		},
		{
			Kind: KAppServer, Name: "App Server", Description: "Runs your code. Each request holds a thread until it finishes.",
			Lesson: "A thread waiting on a slow database cannot serve anyone else.",
			Accent: "#868e96", Width: 160, Height: 72,
			CanConnect: []string{KCache, KDatabase, KReplica, KQueue, KShardRouter, KExternalAPI, KRedis},
			Params: []ParamSpec{
				num("threads", "Threads", "Capacity", 8, 1, 64, 1, "", "Concurrent requests it can work on. Bigger machines cost more than proportionally."),
				num("cpuMs", "CPU time", "Capacity", 20, 1, 500, 1, "ms", "Compute per request, not counting time waiting on dependencies.").fixed(),
				variability("Capacity"),
				num("queueCap", "Queue size", "Capacity", 50, 0, 2000, 10, "", "Requests that may wait for a thread. When full, new ones get a 503."),
				sel("queueOrder", "Queue order", "Capacity", "fifo", "Who gets the next free thread. Under overload newest-first serves people still waiting.",
					opt("fifo", "Oldest first"), opt("lifo", "Newest first")),
				num("queueTimeoutMs", "Max queue wait", "Capacity", 0, 0, 30000, 50, "ms", "Drop queued requests that waited longer than this (0 = never).").adv(),
				num("callTimeoutMs", "Call timeout", "Calls to dependencies", 0, 0, 10000, 50, "ms", "Give up on a dependency call after this long (0 = wait forever)."),
				num("dbPool", "DB pool size", "Calls to dependencies", 20, 1, 200, 1, "", "Max connections this server opens to each database.").needs(KDatabase, KReplica, KShardRouter),
				flag("circuitBreaker", "Circuit breaker", "Calls to dependencies", false, "After 5 failures in a row to a dependency, fail fast for 5 s, then try once."),
				sel("readRouting", "Read routing", "Data access", "nearest", "Where reads go: nearest copy (cache, replica, primary), by freshness, or primary only.",
					opt("nearest", "Nearest copy"), opt("byConsistency", "By freshness"), opt("primary", "Primary only")).needs(KCache, KReplica),
				flag("idempotencyKeys", "Idempotency keys", "Data access", false, "Tag each write with its request id so a retried write is applied once (costs ~1 ms)."),
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
				sel("eviction", "Eviction", "Memory", "lru", "Which item to drop when full.",
					opt("lru", "LRU: least recently used"), opt("lfu", "LFU: least frequently used"), opt("fifo", "FIFO: oldest inserted"), opt("random", "Random")),
				sel("writePolicy", "On writes", "Freshness", "none", "What happens to cached copies when data is written.",
					opt("none", "Do nothing"), opt("invalidate", "Invalidate"), opt("writeThrough", "Write through")),
				num("ttlSec", "Time to live", "Freshness", 0, 0, 3600, 1, "s", "Entries expire after this long (0 = never)."),
				ttlJitter(),
				flag("coalesce", "Share concurrent misses", "Freshness", false, "When several requests miss on the same key at once, only one goes to the data store and the rest wait for its answer.").adv(),
				flag("startWarm", "Start warm", "Memory", true, "Begin the run with popular items already cached."),
				num("latencyMs", "Latency", "Speed", 1, 0.1, 50, 0.1, "ms", "Time per lookup.").adv(),
			},
			cost: func(p Params) float64 { return 0.03 + 0.05*pf(p, "capacityItems", 2000)/2000 },
		},
		{
			Kind: KDatabase, Name: "Database", Description: "The primary data store. Handles reads and writes.",
			Lesson: "Connections and query time cap throughput long before disks do.",
			Accent: "#e8590c", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("connections", "Concurrent queries", "Capacity", 16, 1, 256, 1, "", "Queries it can work on at once."),
				num("readMs", "Read time", "Speed", 10, 1, 500, 1, "ms", "Time for one read."),
				num("writeMs", "Write time", "Speed", 30, 1, 1000, 1, "ms", "Time for one write."),
				variability("Speed"),
				sel("onFailure", "If it fails", "Failover", "stayDown", "What happens when this database crashes. Promoting a replica needs a read replica; writes acknowledged but not yet copied to it are lost.",
					opt("stayDown", "Stay down until it restarts"), opt("promote", "Promote a replica")),
				num("detectSec", "Failure detection", "Failover", 10, 1, 120, 1, "s", "How long until the failure is noticed.").showIf("onFailure", "promote"),
				num("promoteSec", "Promotion time", "Failover", 20, 1, 300, 1, "s", "How long the replica takes to become the primary once the failure is noticed.").showIf("onFailure", "promote"),
				num("maxConnections", "Max connections", "Capacity", 500, 10, 5000, 10, "", "Hard limit on open connections from all servers; beyond it connections are refused."),
				num("queueCap", "Queue size", "Capacity", 100, 0, 5000, 10, "", "Queries that may wait for a free slot."),
				flag("spatialIndex", "Spatial index", "Features", false, "Makes nearby-location searches cheap."),
				flag("rowLocks", "Row locks", "Features", false, "Serialize writes per item so two bookings cannot overlap."),
			},
			cost: func(p Params) float64 { return 0.2 * math.Pow(pf(p, "connections", 16)/16, 1.2) },
		},
		{
			Kind: KReplica, Name: "Read Replica", Description: "A copy of the database that serves reads, slightly behind.",
			Lesson: "Replicas add read capacity at the price of staleness.",
			Accent: "#f08c00", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				num("connections", "Concurrent queries", "Capacity", 16, 1, 256, 1, "", "Queries it can work on at once."),
				num("readMs", "Read time", "Speed", 10, 1, 500, 1, "ms", "Time for one read."),
				num("replicationLagMs", "Replication lag", "Freshness", 100, 0, 30000, 50, "ms", "How far behind the primary this copy runs. Writes newer than this are lost if it has to take over."),
				num("writeMs", "Write time", "Speed", 30, 1, 1000, 1, "ms", "Time for one write, once this replica has been promoted to primary.").adv(),
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
			Params: []ParamSpec{num("capacity", "Capacity", "Buffer", 5000, 10, 100000, 100, "jobs", "Jobs it can hold before rejecting.")},
			cost:   func(Params) float64 { return 0.04 },
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
			},
			cost: func(p Params) float64 { return 0.05 * pf(p, "concurrency", 4) / 4 },
		},
		{
			Kind: KCDN, Name: "CDN", Description: "Serves popular reads from the edge, close to users.",
			Lesson: "The cheapest request is the one that never reaches your servers.",
			Accent: "#087f5b", Width: 160, Height: 72, CanConnect: []string{KLoadBalancer, KAppServer, KRateLimiter},
			Params: []ParamSpec{
				num("capacityItems", "Capacity", "Memory", 5000, 0, 200000, 100, "items", "How many items it can hold."),
				num("ttlSec", "Time to live", "Freshness", 60, 0, 3600, 1, "s", "Entries expire after this long."),
				ttlJitter(),
				flag("startWarm", "Start warm", "Memory", true, "Begin with popular items cached."),
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
			},
			cost: func(Params) float64 { return 0.01 },
		},
		{
			Kind: KShardRouter, Name: "Shard Router", Description: "Splits data across several databases by key.",
			Lesson: "Sharding multiplies capacity but makes cross-shard work hard.",
			Accent: "#5f3dc4", Width: 160, Height: 72, CanConnect: []string{KDatabase, KRedis},
			Params: nil,
			cost:   func(Params) float64 { return 0.02 },
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
			Kind: KRedis, Name: "Redis", Description: "A fast single-threaded store for locks, geo lookups and leaderboards.",
			Lesson: "One thread means commands queue up behind each other.",
			Accent: "#c92a2a", Width: 160, Height: 72, CanConnect: nil,
			Params: []ParamSpec{
				sel("persistence", "Persistence", "Durability", "none", "What a crash loses. None: every write since it started. Append-only file: at most the last second.", opt("none", "None (memory only)"), opt("aof", "Append-only file")),
				num("geoSearchMs", "Geo search time", "Speed", 0.4, 0.05, 20, 0.05, "ms", "Time for one nearby search."),
				num("queueCap", "Queue size", "Capacity", 2000, 0, 20000, 100, "", "Commands that may wait."),
			},
			cost: func(Params) float64 { return 0.08 },
		},
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
	return s.cost(Resolve(n.Kind, n.Params))
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
