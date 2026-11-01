package engine

import "sync"

// Option is one choice of a select setting: the stored value and the label shown to the learner.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ShowIf makes a setting visible only while another setting holds one of the listed values.
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

// specTable is the registry, built once: the specs in display order and an index by kind.
type specTable struct {
	list   []NodeSpec
	byKind map[string]*NodeSpec
}

var specsOnce = sync.OnceValue(buildSpecs)

func buildSpecs() specTable {
	specs := []NodeSpec{
		specClient(),
		specLoadBalancer(),
		specAppServer(),
		specCache(),
		specDatabase(),
		specReplica(),
		specQueue(),
		specTopic(),
		specLog(),
		specWorker(),
		specCDN(),
		specRateLimiter(),
		specShardRouter(),
		specExternalAPI(),
		specObjectStore(),
		specKVStore(),
		specCoordinator(),
		specStream(),
		specLLMServer(),
		specVectorStore(),
		specIndex(),
		specAuth(),
		specRedis(),
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
	byKind := map[string]*NodeSpec{}
	for i := range specs {
		byKind[specs[i].Kind] = &specs[i]
	}
	return specTable{list: specs, byKind: byKind}
}

// Specs returns all node specs in display order.
func Specs() []NodeSpec { return specsOnce().list }

// Spec returns the spec for a kind, or nil.
func Spec(kind string) *NodeSpec { return specsOnce().byKind[kind] }

// defaultParams returns default parameter values for a kind.
func defaultParams(kind string) Params {
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
	out := defaultParams(kind)
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

// nodeCost is the hourly cost of one node in dollars.
func nodeCost(n Node) float64 {
	s := Spec(n.Kind)
	if s == nil {
		return 0
	}
	p := Resolve(n.Kind, n.Params)
	c := s.cost(p)
	if n.Kind == KAppServer && pb(p, "autoscale") { // a fleet costs at least its minimum number of servers
		c *= max(1, pf(p, "minInstances", 1))
	}
	return c
}

// DiagramCost sums node costs.
func DiagramCost(d Diagram) float64 {
	var c float64
	for _, n := range d.Nodes {
		c += nodeCost(n)
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
