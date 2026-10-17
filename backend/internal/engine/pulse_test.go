package engine

import "testing"

// A slowdown can come and go in pulses (GC pauses, a noisy neighbour, a flapping dependency): slow for PulseSec,
// normal for PulseSec, and so on until DurationSec is over.
func TestSlowdownCanPulse(t *testing.T) {
	b := stack(1, Params{"rps": 20.0}, Params{}, nil, nil)
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 3, Chaos: []ChaosEvent{{Type: "slow", NodeID: "a0", AtSec: 2, Factor: 6, DurationSec: 8, PulseSec: 2}}})
	slowAt := func(sec float64) float64 {
		s.Advance(sec*1000 - s.now)
		return s.nodes["a0"].slow
	}
	for _, c := range []struct {
		at   float64
		want float64
	}{{1, 1}, {3, 6}, {5, 1}, {7, 6}, {9, 1}, {11, 1}, {14, 1}} {
		if got := slowAt(c.at); got != c.want {
			t.Errorf("t=%.0fs: slow factor %v, want %v", c.at, got, c.want)
		}
	}
}

func TestSteadySlowdownIsUnchanged(t *testing.T) {
	b := stack(1, Params{"rps": 20.0}, Params{}, nil, nil)
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 3, Chaos: []ChaosEvent{{Type: "slow", NodeID: "a0", AtSec: 2, Factor: 6, DurationSec: 8}}})
	s.Advance(4000)
	s.Advance(2000)
	if s.nodes["a0"].slow != 6 {
		t.Fatalf("a steady slowdown must stay slow for its whole duration, got %v at t=6s", s.nodes["a0"].slow)
	}
	s.Advance(5000)
	if s.nodes["a0"].slow != 1 {
		t.Fatalf("and end on time, got %v at t=11s", s.nodes["a0"].slow)
	}
}
