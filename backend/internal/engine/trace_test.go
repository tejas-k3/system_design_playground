package engine

import (
	"encoding/json"
	"testing"
)

func traced(d Diagram, opts Options, sec float64) *Sim {
	d, _ = Sanitize(d)
	s := New(d, opts)
	s.EnableTraces(true)
	s.Advance(sec * 1000)
	return s
}

func kindsOf(t TraceOut) []string {
	var k []string
	for _, sp := range t.Spans {
		k = append(k, sp.Kind)
	}
	return k
}

func TestTracesFollowTheRequestAcrossNodes(t *testing.T) {
	b := stack(2, Params{"rps": 80.0, "readPct": 100.0}, nil, Params{"startWarm": false}, nil)
	ts := traced(b.d, Options{Seed: 1}, 10).Traces()
	if len(ts.OK) == 0 {
		t.Fatal("no successful traces were kept")
	}
	tr := ts.OK[0]
	if !tr.OK || tr.Attempts < 1 || tr.LatencyMs <= 0 {
		t.Fatalf("unexpected trace header: %+v", tr)
	}
	if got := kindsOf(tr); got[0] != KLoadBalancer || got[1] != KAppServer {
		t.Fatalf("a request should enter through the load balancer and then an app server, got %v", got)
	}
	byNode := map[string]SpanOut{}
	for _, sp := range tr.Spans {
		if sp.Open {
			continue
		}
		if !(sp.At <= sp.Start && sp.Start <= sp.End) {
			t.Errorf("span times out of order: %+v", sp)
		}
		byNode[sp.Node] = sp
	}
	// the app server holds its thread for the whole time its dependencies are working
	for _, sp := range tr.Spans {
		if sp.Kind == KDatabase || sp.Kind == KCache {
			parent := byNode["a0"]
			if p2, ok := byNode["a1"]; ok && sp.At >= p2.At {
				parent = p2
			}
			if sp.Depth != 2 || sp.End > parent.End+1e-6 {
				t.Errorf("dependency span should sit inside its app server's span: %+v within %+v", sp, parent)
			}
		}
	}
}

func TestFailedTracesNameTheNodeAndReason(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 400.0, "readPct": 100.0, "timeoutMs": 3000.0})
	b.node("a", KAppServer, Params{"threads": 2.0, "queueCap": 2.0}).node("db", KDatabase, nil).edge("u", "a").edge("a", "db")
	ts := traced(b.d, Options{Seed: 2}, 8).Traces()
	if len(ts.Failed) == 0 {
		t.Fatal("overload should have produced failed traces")
	}
	found := false
	for _, tr := range ts.Failed {
		if tr.OK || tr.Reason == "" {
			t.Fatalf("failed trace without a reason: %+v", tr)
		}
		for _, sp := range tr.Spans {
			if sp.Kind == KAppServer && !sp.OK && sp.Reason == "queueFull" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no failed trace blamed the app server's full queue: %+v", ts.Failed[0])
	}
}

func TestAbandonedWorkIsMarkedLate(t *testing.T) {
	// the database is far slower than the client's patience: users give up, the work continues
	b := &builder{}
	b.node("u", KClient, Params{"rps": 20.0, "readPct": 100.0, "timeoutMs": 300.0})
	b.node("a", KAppServer, Params{"threads": 32.0, "dbPool": 200.0}).node("db", KDatabase, Params{"readMs": 2000.0, "connections": 64.0})
	b.edge("u", "a").edge("a", "db")
	ts := traced(b.d, Options{Seed: 3}, 10).Traces()
	late := false
	for _, tr := range ts.Failed {
		if tr.Reason != "timeout" {
			continue
		}
		for _, sp := range tr.Spans {
			late = late || (sp.Kind == KDatabase && (sp.Late || sp.Open || sp.End > tr.LatencyMs))
		}
	}
	if !late {
		t.Fatal("expected a timed-out request whose database work continued after the user gave up")
	}
}

func TestTracingDoesNotChangeTheRun(t *testing.T) {
	series := func(on bool) string {
		d, _ := Sanitize(stack(2, Params{"rps": 300.0}, nil, Params{}, nil).d)
		s := New(d, Options{Seed: 9})
		s.EnableTraces(on)
		s.Advance(15000)
		b, _ := json.Marshal(s.Snapshot().Series)
		return string(b)
	}
	if series(true) != series(false) {
		t.Fatal("tracing altered the simulation")
	}
}

func TestTraceSamplingStaysBounded(t *testing.T) {
	d, _ := Sanitize(stack(8, Params{"rps": 2000.0}, Params{"threads": 32.0}, Params{}, Params{"connections": 128.0}).d)
	s := New(d, Options{Seed: 4})
	s.EnableTraces(true)
	s.Advance(10000)
	ts := s.Traces()
	if len(ts.OK) > traceReturn || len(ts.Failed) > traceReturn {
		t.Fatalf("returned too many traces: %d ok, %d failed", len(ts.OK), len(ts.Failed))
	}
	if s.traceEvery < 5 {
		t.Fatalf("at 2000 rps sampling should thin out, traceEvery=%d", s.traceEvery)
	}
	for _, tr := range append(ts.OK, ts.Failed...) {
		if len(tr.Spans) > maxSpans {
			t.Fatalf("trace has %d spans", len(tr.Spans))
		}
	}
}
