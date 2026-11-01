package engine

import (
	"fmt"
	"reflect"
	"testing"
)

// four app servers, the last two already on the new version
func canaryFleet(canaryPct float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 80.0, "readPct": 100.0, "timeoutMs": 3000.0}).
		node("lb", KLoadBalancer, Params{"healthCheckSec": 1.0, "canaryPct": canaryPct})
	b.edge("u", "lb")
	b.node("db", KDatabase, Params{"connections": 64.0})
	for i := range 4 {
		v := "v1"
		if i >= 2 {
			v = "v2"
		}
		id := fmt.Sprintf("a%d", i+1)
		b.node(id, KAppServer, Params{"threads": 16.0, "version": v})
		b.edge("lb", id).edge(id, "db")
	}
	d, _ := Sanitize(b.d)
	return d
}

func badV2() Options {
	return Options{Seed: 7, Chaos: []ChaosEvent{{Type: "badDeploy", Kind: KAppServer, Version: "v2", FractionPct: 100, Mode: "errors", AtSec: 10, DurationSec: 20}}}
}

// Without a canary split the new version gets its fair share of traffic (half, here); with one it gets a trickle.
func TestACanarySplitLimitsTheDamageOfABadRelease(t *testing.T) {
	even := okShare(run(canaryFleet(0), badV2(), 50), 12, 30)
	canary := okShare(run(canaryFleet(5), badV2(), 50), 12, 30)
	t.Logf("success during the bad release: no canary %.3f, 5%% canary %.3f", even, canary)
	if even < 0.45 || even > 0.55 {
		t.Fatalf("two of four servers are bad and traffic is spread evenly: about half should fail, success %.3f", even)
	}
	if canary < 0.93 {
		t.Fatalf("only 5%% of the traffic should reach the new version: success %.3f", canary)
	}
}

func TestTheCanaryServesItsShareWhenHealthy(t *testing.T) {
	s := run(canaryFleet(10), Options{Seed: 7}, 40)
	v2 := s.Snapshot().Nodes["a3"].Arr + s.Snapshot().Nodes["a4"].Arr
	if v2 < 6 || v2 > 10 { // 10% of 80/s
		t.Fatalf("the new version should see about 8 requests a second: %.1f", v2)
	}
}

func TestCanaryOffAndVersionsChangeNothing(t *testing.T) {
	if !reflect.DeepEqual(run(canaryFleet(0), Options{Seed: 7}, 20).RunSnapshot(), run(fleetDesign(4, false), Options{Seed: 7}, 20).RunSnapshot()) {
		t.Fatal("canaryPct 0 and versions alone must change nothing")
	}
}
