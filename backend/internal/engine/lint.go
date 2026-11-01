package engine

import (
	"fmt"
)

// Finding is one piece of advice about a design. Lint is advisory: it never changes a simulation.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity string   `json:"severity"` // error | warn | info
	Message  string   `json:"message"`
	Fix      string   `json:"fix"`
	Nodes    []string `json:"nodes"`
}

func ps(p Params, k string) string { s, _ := p[k].(string); return s }

// Lint checks a design for known failure shapes (each one is reproduced by an engine test or a
// challenge). It is pure and deterministic, and conservative: it prefers a missed finding to a wrong
// one, and reports `error` only for shapes that are broken, not merely risky.
func Lint(d Diagram) []Finding {
	l := newLintRun(d)
	l.ruleSingleServers()
	l.ruleIgnoredRouting()
	l.ruleSmallVote()
	l.ruleIdentity()
	l.ruleAuth()
	l.ruleTimeouts()
	l.ruleRetries()
	l.ruleCapacityAndPatience()
	l.ruleSecondaryReads()
	l.ruleQuorumOverlap()
	l.ruleClientGuarantees()
	l.ruleMessageSize()
	l.ruleDatabaseFailover()
	l.ruleDanglingNodes()
	l.ruleUnreachable()
	return l.fs
}

// lintRun is the facts about one design that the rules share, and the findings collected so far.
type lintRun struct {
	d          Diagram
	kind       map[string]string // node id to kind
	res        map[string]Params // node id to settings with defaults filled in
	out, in    map[string]int    // edges leaving and entering each node
	apps, dbs  []string
	clients    []string
	hasReplica bool
	fs         []Finding
}

func (l *lintRun) add(rule, sev, msg, fix string, nodes ...string) {
	l.fs = append(l.fs, Finding{Rule: rule, Severity: sev, Message: msg, Fix: fix, Nodes: nodes})
}

func newLintRun(d Diagram) *lintRun {
	l := &lintRun{d: d, kind: map[string]string{}, res: map[string]Params{}, out: map[string]int{}, in: map[string]int{}}
	kind, res, out, in := l.kind, l.res, l.out, l.in
	for _, n := range d.Nodes {
		kind[n.ID] = n.Kind
		res[n.ID] = Resolve(n.Kind, n.Params)
	}
	for _, e := range d.Edges {
		out[e.From]++
		in[e.To]++
	}
	var apps, dbs, clients []string
	hasReplica := false
	for _, n := range d.Nodes {
		switch n.Kind {
		case KAppServer:
			apps = append(apps, n.ID)
		case KDatabase:
			dbs = append(dbs, n.ID)
		case KClient:
			clients = append(clients, n.ID)
		case KReplica:
			hasReplica = true
		}
	}

	l.apps, l.dbs, l.clients, l.hasReplica = apps, dbs, clients, hasReplica
	return l
}

func (l *lintRun) ruleSingleServers() {
	res := l.res
	in := l.in
	apps := l.apps
	dbs := l.dbs
	hasReplica := l.hasReplica
	add := l.add
	if len(apps) == 1 && !pb(res[apps[0]], "autoscale") {
		add("spof-server", "warn", "One app server: if it fails or is restarted, nothing answers.", "Run at least two behind a load balancer, or turn on autoscaling.", apps...)
	}
	for _, id := range dbs {
		if !hasReplica && in[id] > 0 {
			add("spof-database", "warn", "The only copy of the data lives on one database: if it crashes, every request that needs it fails.", "Add a read replica and set the database to promote it on failure.", id)
		}
	}
}

func (l *lintRun) ruleIgnoredRouting() {
	d := l.d
	res := l.res
	add := l.add
	var ignoredRouting []string
	for _, n := range d.Nodes {
		if n.Kind == KLoadBalancer && ps(res[n.ID], "layer") == "layer4" && (pf(res[n.ID], "canaryPct", 0) > 0 || pb(res[n.ID], "sticky")) {
			ignoredRouting = append(ignoredRouting, n.ID)
		}
	}
	if len(ignoredRouting) > 0 {
		add("layer4-ignores-content-routing", "warn", "A layer-4 balancer does not read requests, so its canary share and sticky routing are ignored.", "Use layer 7 if you need to route by content, or remove the setting.", ignoredRouting...)
	}
}

func (l *lintRun) ruleSmallVote() {
	d := l.d
	res := l.res
	add := l.add
	var smallVote []string
	for _, n := range d.Nodes {
		if n.Kind == KCoordinator && ps(res[n.ID], "faultModel") == "arbitrary" && min(9, pf(res[n.ID], "nodes", 3)) < 3*pf(res[n.ID], "faultsTolerated", 1)+1 {
			smallVote = append(smallVote, n.ID)
		}
	}
	if len(smallVote) > 0 {
		add("group-too-small-to-vote", "warn", "To outvote k members that give wrong answers a group needs at least 3k+1 members. With fewer, it cannot find enough matching answers and refuses requests while a member gives wrong answers.", "Add members, or lower the number of faulty members to tolerate.", smallVote...)
	}
}

func (l *lintRun) ruleIdentity() {
	d := l.d
	kind := l.kind
	res := l.res
	apps := l.apps
	clients := l.clients
	add := l.add
	var noStore, noIdentity []string
	anyIdentity := false
	for _, id := range clients {
		anyIdentity = anyIdentity || pf(res[id], "identities", 0) > 0
	}
	for _, id := range apps {
		mode := ps(res[id], "sessionState")
		if mode != "local" && mode != "shared" {
			continue
		}
		if !anyIdentity {
			noIdentity = append(noIdentity, id)
		}
		hasStore := false
		for _, e := range d.Edges {
			hasStore = hasStore || (e.From == id && kind[e.To] == KRedis)
		}
		if mode == "shared" && !hasStore {
			noStore = append(noStore, id)
		}
	}
	if len(noIdentity) > 0 {
		add("sessions-need-identities", "warn", "These servers keep login sessions, but no user has an identity, so no session is ever looked up.", "Set Distinct users on the Users component.", noIdentity...)
	}
	if len(noStore) > 0 {
		add("session-store-missing", "warn", "These servers keep sessions in a shared store, but no Redis is connected to them, so nothing is checked.", "Connect a Redis, or keep the sessions in the server's memory.", noStore...)
	}
}

func (l *lintRun) ruleAuth() {
	d := l.d
	kind := l.kind
	res := l.res
	apps := l.apps
	add := l.add
	var noAuth, askEvery, failOpen []string
	for _, id := range apps {
		if ps(res[id], "authMode") != "service" {
			continue
		}
		connected := false
		for _, e := range d.Edges {
			connected = connected || (e.From == id && kind[e.To] == KAuth)
		}
		switch {
		case !connected:
			noAuth = append(noAuth, id)
		case ps(res[id], "authFailMode") == "open":
			failOpen = append(failOpen, id)
		case pf(res[id], "tokenCacheSec", 0) == 0:
			askEvery = append(askEvery, id)
		}
	}
	if len(noAuth) > 0 {
		add("auth-not-connected", "warn", "These servers are set to ask an auth service, but none is connected, so no request is checked.", "Connect an Auth Service, or set the check to a local signature check.", noAuth...)
	}
	if len(askEvery) > 0 {
		add("auth-on-every-request", "warn", "Every request waits for the auth service, which then limits the whole site and takes it down with it.", "Remember a valid token for a short time, or verify the signature on the server itself.", askEvery...)
	}
	if len(failOpen) > 0 {
		add("auth-fails-open", "warn", "If the auth service fails, these servers let every request through unchecked.", "Accept that risk only on purpose; otherwise fail closed and keep the auth service available.", failOpen...)
	}
}

func (l *lintRun) ruleTimeouts() {
	res := l.res
	out := l.out
	apps := l.apps
	add := l.add
	var noTimeout, breaker []string
	for _, id := range apps {
		if out[id] == 0 {
			continue
		}
		if pf(res[id], "callTimeoutMs", 0) == 0 {
			noTimeout = append(noTimeout, id)
			if pb(res[id], "circuitBreaker") {
				breaker = append(breaker, id)
			}
		}
	}
	if len(noTimeout) > 0 {
		add("no-call-timeout", "warn", "These servers wait forever on a dependency that stops answering, so their threads fill up and they stop serving everything else.", "Set a call timeout a little above the dependency's normal worst case.", noTimeout...)
	}
	if len(breaker) > 0 {
		add("breaker-without-timeout", "warn", "The circuit breaker counts failures, but a call that never times out never fails, so it can never trip.", "Set a call timeout so a silent dependency counts as a failure.", breaker...)
	}
}

func (l *lintRun) ruleRetries() {
	res := l.res
	out := l.out
	apps := l.apps
	clients := l.clients
	add := l.add
	var noBackoff, amplifiers []string
	retrying, patience := false, 0.0
	for _, id := range clients {
		c := res[id]
		if pf(c, "dnsTtlSec", 0) > 60 && out[id] > 1 {
			add("long-dns-ttl", "warn", "A long DNS TTL keeps a failed target in rotation longer and delays failover.", "Lower the DNS record TTL if callers must move to a healthy target sooner; the trade-off is more lookups.", id)
		}
		if pf(c, "retries", 0) > 0 {
			if ps(c, "backoff") == "none" {
				noBackoff = append(noBackoff, id)
			}
			if pf(c, "retries", 0) >= 2 {
				if t := pf(c, "timeoutMs", 2000); !retrying || t < patience {
					patience = t
				}
				retrying = true
			}
		}
	}
	if len(noBackoff) > 0 {
		add("retries-without-backoff", "warn", "Immediate retries arrive at the moment the system is struggling and multiply the load.", "Use exponential backoff with jitter.", noBackoff...)
	}
	if retrying {
		for _, id := range apps {
			a := res[id]
			// A full queue takes at least queueCap * cpu time / threads to drain: longer than the users'
			// patience means the head of the queue is already abandoned work.
			drain := pf(a, "queueCap", 0) * pf(a, "cpuMs", 20) / pf(a, "threads", 8)
			if drain > patience && ps(a, "queueOrder") != "lifo" && pf(a, "queueTimeoutMs", 0) == 0 {
				amplifiers = append(amplifiers, id)
			}
		}
		if len(amplifiers) > 0 {
			add("retry-amplification", "warn", "Clients retry, and these servers hold a deep first-in-first-out queue with no wait limit: queued work goes stale, users retry, and the queue never drains.", "Shorten the queue, set a maximum queue wait, or serve newest first.", amplifiers...)
		}
	}
}

func (l *lintRun) ruleCapacityAndPatience() {
	d := l.d
	res := l.res
	apps := l.apps
	clients := l.clients
	add := l.add
	// users' patience: the shortest client timeout; load: the busiest rate the clients offer
	minPatience, rps, writes, strict := 0.0, 0.0, false, false
	for _, id := range clients {
		c := res[id]
		if t := pf(c, "timeoutMs", 2000); minPatience == 0 || t < minPatience {
			minPatience = t
		}
		r := pf(c, "rps", 50)
		if ps(c, "pattern") != "constant" && ps(c, "pattern") != "" {
			r = pf(c, "peakRps", 200)
		}
		if ps(c, "arrival") == "closed" {
			r = 0 // offered load depends on response times: not known up front
		}
		rps += r
		if pf(c, "readPct", 80) < 100 {
			writes = true
			if cn := ps(c, "consistency"); cn == "strong" || cn == "bounded" {
				strict = true
			}
		}
	}
	var deep []string
	for _, id := range apps {
		a := res[id]
		drain := pf(a, "queueCap", 0) * pf(a, "cpuMs", 20) / pf(a, "threads", 8)
		if wait := pf(a, "queueTimeoutMs", 0); minPatience > 0 && drain > minPatience && (wait == 0 || wait > minPatience) {
			deep = append(deep, id)
		}
	}
	if len(deep) > 0 {
		add("queue-longer-than-patience", "warn", "A full queue here takes longer to drain than users are willing to wait, so the oldest queued requests are already abandoned when they reach a thread.", "Shorten the queue or set a maximum queue wait below the users' timeout.", deep...)
	}
	if writes && strict {
		for _, n := range d.Nodes {
			if n.Kind == KCache && pf(res[n.ID], "ttlSec", 0) == 0 && ps(res[n.ID], "writePolicy") == "none" {
				add("cache-no-ttl-with-writes", "warn", "Readers ask for fresh data, writes happen, and this cache neither expires entries nor updates them on a write, so it will serve stale values forever.", "Invalidate or write through on writes, or give entries a time to live.", n.ID)
			}
		}
	}
	for _, id := range apps {
		a := res[id]
		if !pb(a, "autoscale") || rps == 0 || pf(a, "cpuMs", 20) <= 0 {
			continue
		}
		// each server can do at most threads / cpu time requests a second, even if it never waits on anything
		ceiling := pf(a, "maxInstances", 8) * pf(a, "threads", 8) * 1000 / pf(a, "cpuMs", 20)
		share := rps / float64(len(apps))
		if share > ceiling {
			add("autoscale-max-low", "warn", fmt.Sprintf("The most this fleet can ever do is about %.0f requests a second, and the clients offer about %.0f.", ceiling, share), "Raise the maximum number of servers.", id)
		}
	}
}

func (l *lintRun) ruleSecondaryReads() {
	d := l.d
	res := l.res
	in := l.in
	clients := l.clients
	add := l.add
	// reads by another field with no index to serve them
	secondary, ranges := false, false
	for _, id := range clients {
		secondary = secondary || pf(res[id], "secondaryPct", 0) > 0
		ranges = ranges || pf(res[id], "rangePct", 0) > 0
	}
	if secondary || ranges {
		var scans []string
		for _, n := range d.Nodes {
			if (n.Kind == KDatabase || n.Kind == KReplica) && pf(res[n.ID], "indexes", 0) < 1 && in[n.ID] > 0 {
				if !secondary && ps(res[n.ID], "model") == "timeSeries" {
					continue // a time-series store reads a range without an index
				}
				scans = append(scans, n.ID)
			}
		}
		if len(scans) > 0 {
			add("secondary-reads-without-index", "warn", "Some reads look items up by a field other than the key, or return a range of rows, and this database has no secondary index, so each of those reads scans.", "Add a secondary index (each index makes writes about 30% slower).", scans...)
		}
	}
}

func (l *lintRun) ruleQuorumOverlap() {
	d := l.d
	res := l.res
	add := l.add
	for _, n := range d.Nodes {
		if n.Kind != KKVStore {
			continue
		}
		p := res[n.ID]
		copies, writeQuorum := pf(p, "replicas", 3), pf(p, "writeQuorum", 2)
		if writeQuorum > copies/2 { // two writes that miss each other can conflict whatever the read quorum is
			continue
		}
		msg := fmt.Sprintf("With N=%.0f and W=%.0f, two writes can succeed without meeting, so they can conflict.", copies, writeQuorum)
		add("write-quorum-no-overlap", "warn", msg, "Raise the write quorum above half the copies.", n.ID)
	}
}

func (l *lintRun) ruleClientGuarantees() {
	d := l.d
	res := l.res
	clients := l.clients
	add := l.add
	// a per-client guarantee that the store behind the users cannot judge reads zero, which looks like a pass
	for _, id := range clients {
		if ps(res[id], "consistency") != "writesFollowReads" {
			continue
		}
		hasKV, hasStore := false, false
		for _, k := range reachableKinds(d, id) {
			hasKV = hasKV || k == KKVStore
			hasStore = hasStore || k == KDatabase || k == KReplica
		}
		if hasStore && !hasKV {
			add("guarantee-not-judged", "warn", "Writes follow reads is only judged against a key-value store's copies. Here the users reach only a plain database, so the count stays at 0, and a 0 is not a pass.", "Use a key-value store, or ask for monotonic writes or read your writes, which every store can judge.", id)
		}
	}
}

func (l *lintRun) ruleMessageSize() {
	d := l.d
	kind := l.kind
	res := l.res
	clients := l.clients
	add := l.add
	// a payload the queue cannot carry: the users' request size against each queue's message limit
	biggest := 0.0
	for _, id := range clients {
		if v := pf(res[id], "reqKB", 0); v > biggest {
			biggest = v
		}
	}
	if biggest > 0 {
		var small []string
		for _, n := range d.Nodes {
			if n.Kind == KQueue && pf(res[n.ID], "maxMessageKB", 256) < biggest && !claimChecked(d, kind, n.ID) {
				small = append(small, n.ID)
			}
		}
		if len(small) > 0 {
			add("large-payload-into-queue", "warn", fmt.Sprintf("Requests are %.0f KB but this queue refuses messages above its limit, so every one of those writes will fail.", biggest), "Store the body in an object store and queue a pointer to it (the claim-check pattern), or raise the queue's message limit if it really allows it.", small...)
		}
	}
}

func (l *lintRun) ruleDatabaseFailover() {
	d := l.d
	kind := l.kind
	res := l.res
	dbs := l.dbs
	add := l.add
	for _, db := range dbs {
		pool, who := 0.0, []string{}
		for _, e := range d.Edges {
			if e.To == db && kind[e.From] == KAppServer {
				pool += pf(res[e.From], "dbPool", 20)
				who = append(who, e.From)
			}
		}
		if limit := pf(res[db], "maxConnections", 500); pool > limit {
			add("pool-exceeds-db", "error", fmt.Sprintf("The servers may open %.0f connections in total but the database accepts %.0f, so under load some connections are refused.", pool, limit), "Shrink the pools or raise the database's connection limit.", append([]string{db}, who...)...)
		}
		if ps(res[db], "onFailure") == "promote" {
			for _, r := range d.Nodes {
				if r.Kind == KReplica && ps(res[r.ID], "replication") != "sync" {
					add("async-replica-failover", "info", "The replica is asynchronous, so a promotion loses the writes made in the last replication lag.", "Use a synchronous replica if you cannot lose acknowledged writes (it slows every write).", db)
					break
				}
			}
		}
		if ps(res[db], "onFailure") == "promote" && !pb(res[db], "fencing") {
			add("unfenced-failover", "error", "After a promotion the old primary can come back and accept writes next to the new one (split brain).", "Turn on fencing.", db)
		}
	}
}

func (l *lintRun) ruleDanglingNodes() {
	d := l.d
	res := l.res
	out := l.out
	in := l.in
	add := l.add
	for _, n := range d.Nodes {
		if n.Kind == KClient {
			continue
		}
		if in[n.ID]+out[n.ID] == 0 {
			add("dangling-node", "info", "This component is not connected to anything, so it takes no part in the run.", "Connect it or delete it.", n.ID)
			continue
		}
		if (n.Kind == KCache || n.Kind == KCDN) && pf(res[n.ID], "ttlSec", 0) > 0 && pf(res[n.ID], "ttlJitterPct", 0) == 0 && pb(res[n.ID], "startWarm") {
			add("synchronised-expiry", "warn", "Everything is cached at the same moment with the same lifetime, so it all expires together and the database takes every miss at once.", "Add TTL jitter, or start cold.", n.ID)
		}
	}
}

func (l *lintRun) ruleUnreachable() {
	d := l.d
	add := l.add
	for _, n := range Unwired(d) {
		add("unreachable-store", "error", "This component never reaches a data store, so it cannot serve a real request.", "Connect it to a database, cache or queue.", n.ID)
	}
}

// claimChecked reports whether every app server that feeds this queue also uploads to an object store, which makes
// what it queues a pointer instead of the body (the claim-check pattern).
func claimChecked(d Diagram, kind map[string]string, queue string) bool {
	feeders := 0
	for _, e := range d.Edges {
		if e.To != queue || kind[e.From] != KAppServer {
			continue
		}
		feeders++
		hasStore := false
		for _, x := range d.Edges {
			if x.From == e.From && kind[x.To] == KObjectStore {
				hasStore = true
			}
		}
		if !hasStore {
			return false
		}
	}
	return feeders > 0
}

// reachableKinds lists the kinds of every component reachable from a node by following connections.
func reachableKinds(d Diagram, from string) []string {
	kind := map[string]string{}
	for _, n := range d.Nodes {
		kind[n.ID] = n.Kind
	}
	seen := map[string]bool{from: true}
	queue := []string{from}
	var kinds []string
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, e := range d.Edges {
			if e.From == id && !seen[e.To] {
				seen[e.To] = true
				queue = append(queue, e.To)
				kinds = append(kinds, kind[e.To])
			}
		}
	}
	return kinds
}
