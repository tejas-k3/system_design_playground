package engine

import (
	"testing"
	"time"
)

// lintDiagram builds users -> lb -> app1..appN -> db with the given params.
func lintDiagram(servers int, client, app, db Params) Diagram {
	d := Diagram{Nodes: []Node{
		{ID: "users", Kind: KClient, Params: client},
		{ID: "lb", Kind: KLoadBalancer},
		{ID: "db", Kind: KDatabase, Params: db},
	}, Edges: []Edge{{ID: "users>lb", From: "users", To: "lb"}}}
	for k := 1; k <= servers; k++ {
		id := "app" + string(rune('0'+k))
		d.Nodes = append(d.Nodes, Node{ID: id, Kind: KAppServer, Params: app})
		d.Edges = append(d.Edges, Edge{ID: "lb>" + id, From: "lb", To: id}, Edge{ID: id + ">db", From: id, To: "db"})
	}
	return d
}

func rules(fs []Finding) map[string]Finding {
	m := map[string]Finding{}
	for _, f := range fs {
		m[f.Rule] = f
	}
	return m
}

func TestLintRulesFireOnTheRiskyShapeAndStaySilentOtherwise(t *testing.T) {
	good := Params{"callTimeoutMs": 500.0}
	cases := []struct {
		name string
		d    Diagram
		rule string
		want bool
		sev  string
	}{
		{"one server is a SPOF", lintDiagram(1, nil, good, nil), "spof-server", true, "warn"},
		{"two servers are not", lintDiagram(2, nil, good, nil), "spof-server", false, ""},
		{"a lone database is a SPOF", lintDiagram(2, nil, good, nil), "spof-database", true, "warn"},
		{"no call timeout on a dependency", lintDiagram(2, nil, nil, nil), "no-call-timeout", true, "warn"},
		{"with a call timeout it is fine", lintDiagram(2, nil, good, nil), "no-call-timeout", false, ""},
		{"breaker without a timeout can never trip", lintDiagram(2, nil, Params{"circuitBreaker": true}, nil), "breaker-without-timeout", true, "warn"},
		{"breaker with a timeout is fine", lintDiagram(2, nil, Params{"circuitBreaker": true, "callTimeoutMs": 500.0}, nil), "breaker-without-timeout", false, ""},
		{"immediate retries", lintDiagram(2, Params{"retries": 3.0, "backoff": "none"}, good, nil), "retries-without-backoff", true, "warn"},
		{"backed-off retries", lintDiagram(2, Params{"retries": 3.0}, good, nil), "retries-without-backoff", false, ""},
		{"no retries, no finding", lintDiagram(2, Params{"backoff": "none"}, good, nil), "retries-without-backoff", false, ""},
		{"retries into a deep FIFO queue with no wait limit", lintDiagram(2, Params{"retries": 2.0}, Params{"callTimeoutMs": 500.0, "queueCap": 1000.0}, nil), "retry-amplification", true, "warn"},
		{"a queue wait limit removes it", lintDiagram(2, Params{"retries": 2.0}, Params{"callTimeoutMs": 500.0, "queueCap": 1000.0, "queueTimeoutMs": 500.0}, nil), "retry-amplification", false, ""},
		{"pools bigger than the database allows", lintDiagram(3, nil, Params{"callTimeoutMs": 500.0, "dbPool": 100.0}, Params{"maxConnections": 200.0}), "pool-exceeds-db", true, "error"},
		{"pools within the limit", lintDiagram(3, nil, Params{"callTimeoutMs": 500.0, "dbPool": 50.0}, Params{"maxConnections": 200.0}), "pool-exceeds-db", false, ""},
		{"promotion without fencing", lintDiagram(2, nil, good, Params{"onFailure": "promote", "fencing": false}), "unfenced-failover", true, "error"},
		{"promotion with fencing", lintDiagram(2, nil, good, Params{"onFailure": "promote"}), "unfenced-failover", false, ""},
	}
	for _, c := range cases {
		got, ok := rules(Lint(c.d))[c.rule]
		if ok != c.want {
			t.Errorf("%s: rule %s fired=%v, want %v", c.name, c.rule, ok, c.want)
			continue
		}
		if ok && got.Severity != c.sev {
			t.Errorf("%s: severity %s, want %s", c.name, got.Severity, c.sev)
		}
		if ok && (got.Message == "" || got.Fix == "" || len(got.Nodes) == 0) {
			t.Errorf("%s: finding must say what, how to fix, and where: %+v", c.name, got)
		}
	}
}

func TestLintDanglingAndSynchronisedExpiry(t *testing.T) {
	d := lintDiagram(2, nil, Params{"callTimeoutMs": 500.0}, nil)
	d.Nodes = append(d.Nodes, Node{ID: "c", Kind: KCache, Params: Params{"ttlSec": 30.0, "startWarm": true, "ttlJitterPct": 0.0}}, Node{ID: "lonely", Kind: KWorker})
	d.Edges = append(d.Edges, Edge{ID: "app1>c", From: "app1", To: "c"})
	m := rules(Lint(d))
	if f, ok := m["synchronised-expiry"]; !ok || f.Nodes[0] != "c" {
		t.Errorf("a warm cache with a TTL and no jitter should be flagged: %+v", m)
	}
	if f, ok := m["dangling-node"]; !ok || f.Nodes[0] != "lonely" {
		t.Errorf("an unconnected node should be flagged: %+v", m)
	}
}

func TestLintIsPureAndDeterministic(t *testing.T) {
	d := lintDiagram(1, Params{"retries": 3.0, "backoff": "none"}, nil, nil)
	a, b := Lint(d), Lint(d)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("expected the same non-empty findings twice, got %d and %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Rule != b[i].Rule {
			t.Fatalf("order differs at %d: %s vs %s", i, a[i].Rule, b[i].Rule)
		}
	}
}

func TestLintIsFastOnABigDesign(t *testing.T) {
	d := lintDiagram(90, Params{"retries": 3.0, "backoff": "none"}, Params{"queueCap": 900.0}, nil)
	best := time.Hour
	for i := 0; i < 20; i++ {
		t0 := time.Now()
		Lint(d)
		if el := time.Since(t0); el < best {
			best = el
		}
	}
	t.Logf("94-node lint: %v", best)
	if best > 20*time.Millisecond {
		t.Fatalf("lint must stay under 20 ms for a 94-node design, took %v", best)
	}
}
