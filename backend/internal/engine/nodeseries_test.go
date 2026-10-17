package engine

import (
	"math"
	"reflect"
	"testing"
)

func TestNodeSeriesIsAOneSecondHistory(t *testing.T) {
	s := run(stack(1, Params{"rps": 60.0, "readPct": 100.0}, Params{}, nil, nil).d, Options{Seed: 3}, 30)
	pts := s.NodeSeries("a0")
	if len(pts) < 29 || len(pts) > 30 {
		t.Fatalf("expected about one point per simulated second (29-30), got %d", len(pts))
	}
	for i := 1; i < len(pts); i++ {
		if d := pts[i].T - pts[i-1].T; math.Abs(d-1) > 1e-6 {
			t.Fatalf("points are one second apart, got %.3f between %d and %d", d, i-1, i)
		}
	}
	var u, arr float64
	for _, p := range pts[len(pts)-10:] {
		u += p.Util / 10
		arr += p.Arr / 10
	}
	live := s.Snapshot().Nodes["a0"]
	if math.Abs(u-live.Util) > 0.05 || math.Abs(arr-live.Arr)/live.Arr > 0.1 {
		t.Errorf("the history agrees with the live numbers: util %.2f vs %.2f, arrivals %.1f vs %.1f", u, live.Util, arr, live.Arr)
	}
}

func TestNodeSeriesKeepsTheLastTwoMinutes(t *testing.T) {
	s := run(stack(1, Params{"rps": 20.0}, Params{}, nil, nil).d, Options{Seed: 3}, 300)
	pts := s.NodeSeries("a0")
	if len(pts) != nodeHistory {
		t.Fatalf("history is capped at %d points, got %d", nodeHistory, len(pts))
	}
	if first := pts[0].T; first < 300-nodeHistory-1 {
		t.Errorf("the oldest kept point is from %.0f s, expected the last %d s", first, nodeHistory)
	}
}

func TestNodeSeriesShowsAnOutageAndIsDeterministic(t *testing.T) {
	mk := func() []NodePoint {
		d, _ := Sanitize(stack(1, Params{"rps": 50.0, "readPct": 100.0, "retries": 0.0}, Params{}, nil, nil).d)
		s := New(d, Options{Seed: 3, Chaos: []ChaosEvent{{Type: "crash", NodeID: "db", AtSec: 10, DownSec: 8}}})
		s.Advance(25000)
		return s.NodeSeries("a0")
	}
	a, b := mk(), mk()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed, different history")
	}
	var during, before float64
	for _, p := range a {
		if p.T > 12 && p.T < 17 {
			during += p.Err
		}
		if p.T > 3 && p.T < 8 {
			before += p.Err
		}
	}
	if during <= before+50 {
		t.Errorf("errors per second at the app server should jump while the database is down: %.0f vs %.0f", during, before)
	}
}

func TestNodeSeriesOfAnUnknownNodeIsEmpty(t *testing.T) {
	s := run(stack(1, Params{"rps": 10.0}, Params{}, nil, nil).d, Options{Seed: 3}, 5)
	if got := s.NodeSeries("nope"); got == nil || len(got) != 0 {
		t.Errorf("want an empty (non-nil) list, got %v", got)
	}
}
