package engine

import "testing"

// Location pings from a city where 40% of the activity is in one cell. Sharding by cell sends the whole hot
// cell to one shard; sharding by the sender spreads the same pings evenly.
func shardLoads(shardBy string) (busiestOverMean float64) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 400.0, "workload": "geo", "readPct": 0.0, "keySpace": 400.0, "keyDist": "uniform", "hotKeyPct": 70.0, "timeoutMs": 3000.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0})
	rp := Params{}
	if shardBy != "" {
		rp["shardBy"] = shardBy
	}
	b.node("sr", KShardRouter, rp)
	b.edge("u", "a").edge("a", "sr")
	ids := []string{"r1", "r2", "r3", "r4"}
	for _, id := range ids {
		b.node(id, KRedis, Params{})
		b.edge("sr", id)
	}
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 5})
	s.Advance(3000)
	tot := map[string]float64{}
	for i := 0; i < 10; i++ {
		s.Advance(1000)
		for _, id := range ids {
			tot[id] += s.Snapshot().Nodes[id].Arr
		}
	}
	sum, max := 0.0, 0.0
	for _, id := range ids {
		sum += tot[id]
		if tot[id] > max {
			max = tot[id]
		}
	}
	return max / (sum / float64(len(ids)))
}

func TestShardingByCellConcentratesTheHotCell(t *testing.T) {
	cell, user := shardLoads(""), shardLoads("request")
	t.Logf("busiest shard over the mean: %.2fx by cell, %.2fx by sender", cell, user)
	if cell < 3 {
		t.Errorf("a hot cell should overload its shard: %.2fx", cell)
	}
	if user > 1.15 {
		t.Errorf("sharding by sender should be about even: %.2fx", user)
	}
	if k := shardLoads("key"); k != cell {
		t.Errorf("shardBy key must equal the default: %v vs %v", k, cell)
	}
}
