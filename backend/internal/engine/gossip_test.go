package engine

import "testing"

// A node dies and the others have to find out. The one that receives a request sends it to the copies it believes are
// up. If it has not heard yet that one of them is dead, the request waits for a reply that never comes. News spread by
// gossip (each node tells a few others every round) reaches everyone in about log N rounds; telling every node at once
// takes one message each.
func gossipRun(kv Params) (RunStats, map[string]float64) {
	base := Params{"nodes": 20.0, "replicas": 3.0, "readQuorum": 1.0, "writeQuorum": 3.0, "sloppyQuorum": true, "hintedHandoff": true, "writeMs": 5.0, "failureDetection": "gossip"}
	for k, v := range kv {
		base[k] = v
	}
	chaos := []ChaosEvent{{Type: "kvNodeDown", NodeID: "kv", AtSec: 5, DurationSec: 0, Factor: 1}}
	s := quorumRunWith(Params{"readPct": 0.0, "rps": 200.0, "keySpace": 2000.0}, base, chaos)
	return s.RunSnapshot(), s.Snapshot().Nodes["kv"].Extra
}

func TestTheSlowerNewsSpreadsTheLongerRequestsWaitOnADeadNode(t *testing.T) {
	instant, _ := gossipRun(Params{"failureDetection": "instant"})
	slow, sx := gossipRun(Params{"gossipIntervalMs": 4000.0, "gossipFanout": 1.0})
	fast, fx := gossipRun(Params{"gossipIntervalMs": 500.0, "gossipFanout": 3.0})
	all, ax := gossipRun(Params{"failureDetection": "broadcast"})
	failed := func(r RunStats) int { return r.Resolved - r.OK }
	t.Logf("failed writes: instant %d, gossip every 4 s to 1 peer %d (all know after %.1f s), every 0.5 s to 3 peers %d (%.1f s), broadcast %d (%.2f s)", failed(instant), failed(slow), sx["timeToAllSec"], failed(fast), fx["timeToAllSec"], failed(all), ax["timeToAllSec"])
	if failed(instant) > failed(slow)/20 {
		t.Errorf("a cluster that knows at once has no such window: %d against %d", failed(instant), failed(slow))
	}
	if !(failed(slow) > failed(fast)*3 && failed(fast) > failed(all)) {
		t.Errorf("failures follow the time everyone takes to find out: %d, %d, %d", failed(slow), failed(fast), failed(all))
	}
	if sx["timeToAllSec"] < 12 || fx["timeToAllSec"] > 4 || ax["timeToAllSec"] > 1 {
		t.Errorf("rounds to reach 20 nodes: slow %.1f s, fast %.1f s, broadcast %.2f s", sx["timeToAllSec"], fx["timeToAllSec"], ax["timeToAllSec"])
	}
}

func TestGossipSendsFewerMessagesPerNodeButBroadcastIsFasterAndCostsMoreAsTheGroupGrows(t *testing.T) {
	msgs := func(nodes float64, mode string) float64 {
		_, x := gossipRun(Params{"nodes": nodes, "failureDetection": mode})
		return x["membershipMsgs"]
	}
	g20, g40, b20, b40 := msgs(20, "gossip"), msgs(40, "gossip"), msgs(20, "broadcast"), msgs(40, "broadcast")
	t.Logf("messages for one failure: gossip %.0f at 20 nodes, %.0f at 40; broadcast %.0f, %.0f", g20, g40, b20, b40)
	if b40 < b20*1.9 || b40 > b20*2.1 {
		t.Errorf("telling everyone costs a message per node: %.0f to %.0f", b20, b40)
	}
	if g40 >= g20*2 {
		t.Errorf("gossip grows slower than the group: %.0f to %.0f", g20, g40)
	}
}

func TestWithoutTheSettingNothingChanges(t *testing.T) {
	a, _ := gossipRun(Params{"failureDetection": "instant"})
	b := quorumRunWith(Params{"readPct": 0.0, "rps": 200.0, "keySpace": 2000.0}, Params{"nodes": 20.0, "replicas": 3.0, "readQuorum": 1.0, "writeQuorum": 3.0, "sloppyQuorum": true, "hintedHandoff": true, "writeMs": 5.0}, []ChaosEvent{{Type: "kvNodeDown", NodeID: "kv", AtSec: 5, DurationSec: 0, Factor: 1}}).RunSnapshot()
	if a.OK != b.OK || a.Resolved != b.Resolved {
		t.Errorf("instant knowledge is the existing behaviour: %d/%d against %d/%d", a.OK, a.Resolved, b.OK, b.Resolved)
	}
}
