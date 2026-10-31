package engine

import "testing"

// Two app servers behind a balancer; one crashes at 10 s for 10 s. How many requests fail depends on how soon the
// balancer stops sending to it and starts again after the restart.
func membershipRun(lb Params) *Sim { return membershipRunFor(lb, 40) }

func membershipRunFor(lb Params, secs float64) *Sim {
	b := &builder{}
	base := Params{"healthCheckSec": 1.0}
	for k, v := range lb {
		base[k] = v
	}
	b.node("u", KClient, Params{"rps": 80.0, "readPct": 100.0, "timeoutMs": 1000.0}).
		node("lb", KLoadBalancer, base).
		node("a1", KAppServer, Params{"threads": 32.0}).
		node("a2", KAppServer, Params{"threads": 32.0}).
		node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "lb").edge("lb", "a1").edge("lb", "a2").edge("a1", "db").edge("a2", "db")
	d, _ := Sanitize(b.d)
	o := Options{Seed: 3, Chaos: []ChaosEvent{{Type: "crash", NodeID: "a1", AtSec: 10, DownSec: 10}}}
	return run(d, o, secs)
}

func TestARegistryKeepsADeadServerListedUntilItsEntryExpires(t *testing.T) {
	health := membershipRun(nil).RunSnapshot().Failed
	registry := membershipRun(Params{"membership": "registry", "registryTtlSec": 6.0, "registryRefreshSec": 3.0}).RunSnapshot().Failed
	t.Logf("failed requests: %d with a health check, %d with a registry (6 s entry, 3 s refresh)", health, registry)
	if registry < 3*health+20 {
		t.Errorf("an entry that lives 9 s after the crash must fail far more than a 1 s check: %d vs %d", registry, health)
	}
}

func TestARestartedServerRejoinsOnlyAtTheNextRefresh(t *testing.T) {
	served := func(refresh float64) float64 {
		// the server is back at 20 s; look at 24 s
		return membershipRunFor(Params{"membership": "registry", "registryTtlSec": 1.0, "registryRefreshSec": refresh}, 24).Snapshot().Nodes["a1"].Arr
	}
	fast, slow := served(1), served(8)
	t.Logf("requests a second a1 serves at 24 s: %.0f with a 1 s refresh, %.0f with 8 s", fast, slow)
	if slow >= fast {
		t.Errorf("a longer refresh delays the rejoin, so the server serves less: %.0f vs %.0f", slow, fast)
	}
}

func TestMembershipByHealthCheckIsTheDefault(t *testing.T) {
	a, b := membershipRun(nil).RunSnapshot(), membershipRun(Params{"membership": "health"}).RunSnapshot()
	if a.OK != b.OK || a.Failed != b.Failed {
		t.Errorf("naming the default must change nothing: %d/%d vs %d/%d", a.OK, a.Failed, b.OK, b.Failed)
	}
}

// A server that comes back before its entry expired was never dropped, so it keeps its place.
func TestAQuickRestartNeverLeftThePool(t *testing.T) {
	quick := func(refresh float64) float64 {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 80.0, "readPct": 100.0, "timeoutMs": 1000.0}).
			node("lb", KLoadBalancer, Params{"membership": "registry", "registryTtlSec": 6.0, "registryRefreshSec": refresh}).
			node("a1", KAppServer, Params{"threads": 32.0}).node("a2", KAppServer, Params{"threads": 32.0}).
			node("db", KDatabase, Params{"connections": 64.0})
		b.edge("u", "lb").edge("lb", "a1").edge("lb", "a2").edge("a1", "db").edge("a2", "db")
		d, _ := Sanitize(b.d)
		return run(d, Options{Seed: 3, Chaos: []ChaosEvent{{Type: "crash", NodeID: "a1", AtSec: 10, DownSec: 2}}}, 13).Snapshot().Nodes["a1"].Arr
	}
	if a := quick(8); a < 20 {
		t.Errorf("a server back after 2 s was still listed, so it serves at once: %.0f/s", a)
	}
}
