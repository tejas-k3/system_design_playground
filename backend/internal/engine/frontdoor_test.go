package engine

import "testing"

// The entry balancer can be the first thing to fill up: it passes only so many requests a second (a layer-4 switch,
// which reads no content, passes about four times as many as a layer-7 one), and only a layer-7 balancer can route on
// what a request contains.
func frontDoor(lb Params, rps float64, app Params) (*Sim, RunStats) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 100.0, "keySpace": 1000.0, "timeoutMs": 2000.0, "retries": 0.0, "identities": 200.0})
	b.node("lb", KLoadBalancer, lb)
	ap := Params{"threads": 32.0, "cpuMs": 5.0, "queueCap": 500.0}
	for k, v := range app {
		ap[k] = v
	}
	b.node("a1", KAppServer, ap)
	v2 := Params{"version": "v2"}
	for k, v := range ap {
		v2[k] = v
	}
	b.node("a2", KAppServer, v2)
	b.node("db", KDatabase, Params{"connections": 64.0, "readMs": 1.0})
	b.edge("u", "lb").edge("lb", "a1").edge("lb", "a2").edge("a1", "db").edge("a2", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 4})
	s.Advance(20000)
	return s, s.RunSnapshot()
}

func TestABalancerThatPassesOnlySoManyRequestsIsTheBottleneck(t *testing.T) {
	_, free := frontDoor(Params{}, 2000, nil)
	s, l7 := frontDoor(Params{"maxRps": 1000.0}, 2000, nil)
	_, l4 := frontDoor(Params{"maxRps": 1000.0, "layer": "layer4"}, 2000, nil)
	t.Logf("no limit %d/%d ok; layer 7 at 1000/s %d/%d; layer 4 %d/%d; balancer busy %.2f", free.OK, free.Resolved, l7.OK, l7.Resolved, l4.OK, l4.Resolved, s.Snapshot().Nodes["lb"].Util)
	if free.OK < free.Resolved*98/100 {
		t.Errorf("without a limit the balancer is never the bottleneck: %d of %d", free.OK, free.Resolved)
	}
	if l7.OK > l7.Resolved*60/100 || l7.OK < l7.Resolved*40/100 {
		t.Errorf("half of 2,000 a second gets through a balancer that passes 1,000: %d of %d", l7.OK, l7.Resolved)
	}
	if l4.OK < l4.Resolved*98/100 {
		t.Errorf("a layer-4 switch passes about four times as many: %d of %d", l4.OK, l4.Resolved)
	}
	if u := s.Snapshot().Nodes["lb"].Util; u < 0.95 {
		t.Errorf("a full balancer shows as busy, so the bottleneck check can name it: %.2f", u)
	}
}

func TestOnlyALayerSevenBalancerRoutesByContent(t *testing.T) {
	v2Share := func(lb Params) float64 {
		s, _ := frontDoor(lb, 200, nil)
		n := s.Snapshot().Nodes
		return n["a2"].Arr / (n["a1"].Arr + n["a2"].Arr)
	}
	l7, l4 := v2Share(Params{"canaryPct": 10.0}), v2Share(Params{"canaryPct": 10.0, "layer": "layer4"})
	t.Logf("share sent to the new version: layer 7 %.2f, layer 4 %.2f", l7, l4)
	if l7 < 0.05 || l7 > 0.15 || l4 < 0.4 {
		t.Errorf("a canary share needs to read the request: layer 7 %.2f, layer 4 %.2f", l7, l4)
	}
}

func TestALayerFourSwitchIsCheaperAndAddsLessTime(t *testing.T) {
	if c7, c4 := Spec(KLoadBalancer).cost(Params{"layer": "layer7"}), Spec(KLoadBalancer).cost(Params{"layer": "layer4"}); c4 >= c7 {
		t.Errorf("the simpler switch costs less: %.3f against %.3f", c4, c7)
	}
	if def := Spec(KLoadBalancer).cost(Params{}); def != 0.02 {
		t.Errorf("the default cost is unchanged: %.3f", def)
	}
	_, a := frontDoor(Params{}, 100, nil)
	_, b := frontDoor(Params{"layer": "layer4"}, 100, nil)
	if b.Mean >= a.Mean {
		t.Errorf("less work per request: %.3f against %.3f ms", b.Mean, a.Mean)
	}
}

// A function that scales to zero starts cold: the first request after it has been idle waits for it to boot.
func TestTheFirstRequestAfterAnIdleSpellWaitsForAColdStart(t *testing.T) {
	cold := func(app Params) float64 {
		_, r := frontDoor(Params{}, 1, app)
		return r.Mean
	}
	warm := cold(nil)
	cs := cold(Params{"coldStartMs": 800.0, "idleSec": 0.2})
	kept := cold(Params{"coldStartMs": 800.0, "idleSec": 0.2, "keepWarm": true})
	t.Logf("mean latency at 1 request a second: always on %.0f ms, scale to zero %.0f ms, kept warm %.0f ms", warm, cs, kept)
	if cs-warm < 200 {
		t.Errorf("requests a second apart each find the function cold: +%.0f ms", cs-warm)
	}
	if kept-warm > 5 {
		t.Errorf("keeping it warm removes the cold start: +%.0f ms", kept-warm)
	}
	_, busy := frontDoor(Params{}, 100, Params{"coldStartMs": 800.0, "idleSec": 0.2})
	_, busyWarm := frontDoor(Params{}, 100, nil)
	if busy.Mean-busyWarm.Mean > 20 {
		t.Errorf("under steady traffic it never goes idle: +%.0f ms", busy.Mean-busyWarm.Mean)
	}
}

// A container boots in seconds, a virtual machine in tens of seconds: an autoscaled fleet catches up with a spike sooner.
func TestContainersBootFasterThanVirtualMachines(t *testing.T) {
	run := func(p Params) float64 {
		p["scaleUpDelaySec"] = 30.0
		return recoverSec(scaleSim(p, step, 3), 300)
	}
	vm, ctr := run(Params{}), run(Params{"runtime": "container", "containerBootSec": 3.0})
	t.Logf("back to normal after the spike: virtual machines %.0f s, containers %.0f s", vm, ctr)
	if ctr < 0 || vm < 0 || ctr >= vm-10 {
		t.Errorf("containers catch up sooner: %.0f against %.0f s", ctr, vm)
	}
}
