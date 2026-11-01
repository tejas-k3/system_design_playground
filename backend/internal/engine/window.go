package engine

import "slices"

// Window is an append-only sliding window of (time, ok, latency) samples.
type Window struct {
	t    []float64
	ok   []bool
	lat  []float64
	head int
}

// Push records one finished request: when it ended, whether it succeeded and its latency.
func (w *Window) Push(t float64, ok bool, lat float64) {
	w.t = append(w.t, t)
	w.ok = append(w.ok, ok)
	w.lat = append(w.lat, lat)
}

// Prune drops samples with time <= before.
func (w *Window) Prune(before float64) {
	for w.head < len(w.t) && w.t[w.head] <= before {
		w.head++
	}
	if w.head > 4096 && w.head > len(w.t)/2 {
		w.t = append([]float64(nil), w.t[w.head:]...)
		w.ok = append([]bool(nil), w.ok[w.head:]...)
		w.lat = append([]float64(nil), w.lat[w.head:]...)
		w.head = 0
	}
}

func (w *Window) first(after float64) int {
	lo, hi := w.head, len(w.t)
	for lo < hi {
		m := (lo + hi) / 2
		if w.t[m] <= after {
			lo = m + 1
		} else {
			hi = m
		}
	}
	return lo
}

// WindowStats summarizes samples newer than `after`.
type WindowStats struct {
	OK, Failed    int
	P50, P95, P99 float64
	Mean          float64
}

// Stats summarises the requests that ended after the given time.
func (w *Window) Stats(after float64) WindowStats {
	var s WindowStats
	var lats []float64
	var sum float64
	for i := w.first(after); i < len(w.t); i++ {
		if w.ok[i] {
			s.OK++
			sum += w.lat[i]
			lats = append(lats, w.lat[i])
		} else {
			s.Failed++
		}
	}
	slices.Sort(lats)
	q := func(p float64) float64 {
		if len(lats) == 0 {
			return 0
		}
		i := int(float64(len(lats))*p+0.999999) - 1
		if i < 0 {
			i = 0
		}
		if i >= len(lats) {
			i = len(lats) - 1
		}
		return lats[i]
	}
	s.P50, s.P95, s.P99 = q(0.5), q(0.95), q(0.99)
	if s.OK > 0 {
		s.Mean = sum / float64(s.OK)
	}
	return s
}

// Count returns the number of samples newer than `after`.
func (w *Window) Count(after float64) int { return len(w.t) - w.first(after) }
