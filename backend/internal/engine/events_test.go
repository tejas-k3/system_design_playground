package engine

import (
	"math"
	"reflect"
	"testing"
)

func eventAt(t *testing.T, ev []Event, typ string) Event {
	t.Helper()
	for _, e := range ev {
		if e.Type == typ {
			return e
		}
	}
	t.Fatalf("no %q event in %+v", typ, ev)
	return Event{}
}

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.05 {
		t.Errorf("%s at %.3f s, want %.3f s", what, got, want)
	}
}

func TestEventLogRecordsTheFailoverTimeline(t *testing.T) {
	s := failoverSim(failover, Params{"replicationLagMs": 500.0}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 300})
	s.Advance(30000)
	ev := s.Snapshot().Events
	near(t, "crash", eventAt(t, ev, "crash").T, 10)
	near(t, "failover detected", eventAt(t, ev, "failoverDetected").T, 15)
	p := eventAt(t, ev, "promotion")
	near(t, "promotion", p.T, 25)
	if p.NodeID != "r" || p.Lost == 0 {
		t.Errorf("promotion should name the replica and the writes lost: %+v", p)
	}
	if got := eventAt(t, ev, "crash").NodeID; got != "db" {
		t.Errorf("crash is about db, got %q", got)
	}
}

func TestEventLogRecordsStartAndEndOfSlowAndPartition(t *testing.T) {
	s := partitionRun(ChaosEvent{Type: "partition", NodeID: "db", AtSec: 3, DurationSec: 4})
	s.Advance(9000)
	ev := s.Snapshot().Events
	near(t, "partition", eventAt(t, ev, "partition").T, 3)
	near(t, "heal", eventAt(t, ev, "heal").T, 7)

	b := stack(1, Params{"rps": 20.0}, Params{}, nil, nil)
	d, _ := Sanitize(b.d)
	sl := New(d, Options{Seed: 3, Chaos: []ChaosEvent{{Type: "slow", NodeID: "a0", AtSec: 2, Factor: 6, DurationSec: 5}}})
	sl.Advance(9000)
	ev = sl.Snapshot().Events
	near(t, "slow start", eventAt(t, ev, "slow").T, 2)
	near(t, "slow end", eventAt(t, ev, "slowEnd").T, 7)
}

func TestEventLogRecordsRestarts(t *testing.T) {
	s := failoverSim(Params{}, Params{"replicationLagMs": 500.0}, ChaosEvent{Type: "crash", NodeID: "db", AtSec: 4, DownSec: 6})
	s.Advance(15000)
	near(t, "restart", eventAt(t, s.Snapshot().Events, "restart").T, 10)
}

func TestEventLogRecordsAutoscalingSteps(t *testing.T) {
	s := scaleSim(Params{"scaleUpDelaySec": 20.0}, step, 3)
	s.Advance(120000)
	ev := s.Snapshot().Events
	ordered, up := eventAt(t, ev, "scaleUp"), eventAt(t, ev, "serverUp")
	if gap := up.T - ordered.T; math.Abs(gap-20) > 0.15 {
		t.Errorf("a server ordered at %.2f s should be running 20 s later, got %.2f s", ordered.T, gap)
	}
	if ordered.NodeID != "a" || ordered.Label == "" {
		t.Errorf("autoscale events name the app server: %+v", ordered)
	}
}

func TestEventLogIsDeterministicAndBounded(t *testing.T) {
	var many []ChaosEvent
	for k := 0; k < 250; k++ {
		many = append(many, ChaosEvent{Type: "slow", NodeID: "a0", AtSec: float64(1 + k), Factor: 2, DurationSec: 0.5})
	}
	run := func() []Event {
		b := stack(1, Params{"rps": 10.0}, Params{}, nil, nil)
		d, _ := Sanitize(b.d)
		s := New(d, Options{Seed: 3, Chaos: many})
		s.Advance(260000)
		return s.Snapshot().Events
	}
	a, b := run(), run()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed, different event log")
	}
	if len(a) > 200 {
		t.Errorf("the log is capped at 200 events, got %d", len(a))
	}
	if last := a[len(a)-1]; last.T < 249 {
		t.Errorf("the newest events are kept, last at %.1f s", last.T)
	}
}
