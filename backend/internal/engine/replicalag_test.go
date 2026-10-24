package engine

import (
	"math"
	"testing"
)

// A replica copies from the primary over the network, so it runs behind by its own copy lag plus the time the
// changes take to get there: the round trip between the primary's place and its own. Same zone: no addition; another
// zone of the region: 2 x 1.5 ms; another region: twice the one-way distance (45 ms between the US and Europe).
func lagOf(primary, replica Params) float64 {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 10.0, "readPct": 100.0, "timeoutMs": 2000.0})
	b.node("a", KAppServer, Params{"threads": 8.0})
	b.node("db", KDatabase, primary)
	rp := Params{"replicationLagMs": 100.0}
	for k, v := range replica {
		rp[k] = v
	}
	b.node("r", KReplica, rp)
	b.edge("u", "a").edge("a", "db").edge("a", "r")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 1})
	return s.replicaLag(s.nodes["r"])
}

func TestAReplicaFarFromItsPrimaryRunsFurtherBehind(t *testing.T) {
	near := lagOf(nil, nil)
	zone := lagOf(nil, Params{"zone": "az-b"})
	eu := lagOf(nil, Params{"region": "eu"})
	ap := lagOf(Params{"region": "eu"}, Params{"region": "ap"})
	t.Logf("lag: same place %.1f ms, other zone %.1f, Europe from the US %.1f, Asia-Pacific from Europe %.1f", near, zone, eu, ap)
	for _, c := range []struct {
		name      string
		got, want float64
	}{{"same place", near, 100}, {"other zone", zone, 103}, {"Europe from the US", eu, 190}, {"Asia-Pacific from Europe", ap, 300}} {
		if math.Abs(c.got-c.want) > 0.01 {
			t.Errorf("%s: %.2f ms, expected %.2f", c.name, c.got, c.want)
		}
	}
}

func TestASynchronousReplicaIsNeverBehindWhereverItIs(t *testing.T) {
	if got := lagOf(nil, Params{"region": "ap", "replication": "sync"}); got != 0 {
		t.Errorf("a synchronous replica has no lag: %.1f", got)
	}
}
