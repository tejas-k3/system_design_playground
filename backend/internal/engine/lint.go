package engine

import "fmt"

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
	kind := map[string]string{}
	res := map[string]Params{}
	out, in := map[string]int{}, map[string]int{}
	for _, n := range d.Nodes {
		kind[n.ID] = n.Kind
		res[n.ID] = Resolve(n.Kind, n.Params)
	}
	for _, e := range d.Edges {
		out[e.From]++
		in[e.To]++
	}
	var fs []Finding
	add := func(rule, sev, msg, fix string, nodes ...string) {
		fs = append(fs, Finding{Rule: rule, Severity: sev, Message: msg, Fix: fix, Nodes: nodes})
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

	if len(apps) == 1 && !pb(res[apps[0]], "autoscale") {
		add("spof-server", "warn", "One app server: if it fails or is restarted, nothing answers.", "Run at least two behind a load balancer, or turn on autoscaling.", apps...)
	}
	for _, id := range dbs {
		if !hasReplica && in[id] > 0 {
			add("spof-database", "warn", "The only copy of the data lives on one database: if it crashes, every request that needs it fails.", "Add a read replica and set the database to promote it on failure.", id)
		}
	}
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

	var noBackoff, amplifiers []string
	retrying, patience := false, 0.0
	for _, id := range clients {
		c := res[id]
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
	for _, n := range Unwired(d) {
		add("unreachable-store", "error", "This component never reaches a data store, so it cannot serve a real request.", "Connect it to a database, cache or queue.", n.ID)
	}
	return fs
}
