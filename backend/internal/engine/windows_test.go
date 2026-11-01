package engine

import (
	"math"
	"sort"
	"testing"
)

// Events carry the time they happened (event time) but arrive later and out of order. A window of event time is
// emitted when the watermark (the newest event time seen, minus a delay) passes its end; an event that arrives
// after that updates the window if it is within the allowed lateness and is dropped beyond it.
type evt struct{ at, arrive float64 }

func feed(w *WindowAgg, events []evt) {
	sort.Slice(events, func(i, j int) bool { return events[i].arrive < events[j].arrive })
	for _, e := range events {
		w.Add(e.at)
	}
	w.Flush()
}

func finalCounts(w *WindowAgg) map[float64]int {
	last := map[float64]int{}
	for _, e := range w.Emitted {
		last[e.Start] = e.Count // the latest emission of a window is its final value
	}
	return last
}

func TestTumblingWindowsCountEveryEventOnce(t *testing.T) {
	rng := NewRNG(5)
	var evs []evt
	for i := range 5000 {
		at := float64(i) * 0.01                       // 100 events a second for 50 s
		evs = append(evs, evt{at, at + rng.Next()*2}) // up to 2 s late
	}
	w := NewWindowAgg(10, 10, 5, 0) // 10 s windows, watermark 5 s behind: nothing is late enough to drop
	feed(w, evs)
	total := 0
	for _, c := range finalCounts(w) {
		total += c
	}
	t.Logf("%d events -> %d in windows, %d dropped, %d windows", len(evs), total, w.LateDropped, len(finalCounts(w)))
	if total != len(evs) || w.LateDropped != 0 {
		t.Errorf("every event lands in exactly one window: %d of %d, %d dropped", total, len(evs), w.LateDropped)
	}
	for start, c := range finalCounts(w) {
		if c != 1000 {
			t.Errorf("window at %.0f s holds %d events, expected 1000", start, c)
		}
	}
}

func TestEventsBeyondTheLatenessAreDropped(t *testing.T) {
	// 100 events a second; 10% of them arrive 8 s late. Watermark delay 1 s, no allowed lateness: a late event is
	// dropped once the watermark has passed its window.
	rng := NewRNG(6)
	var evs []evt
	for i := range 5000 {
		at := float64(i) * 0.01
		d := 0.0
		if rng.Next() < 0.1 {
			d = 8
		}
		evs = append(evs, evt{at, at + d})
	}
	strict := NewWindowAgg(5, 5, 1, 0)
	feed(strict, append([]evt(nil), evs...))
	patient := NewWindowAgg(5, 5, 1, 10)
	feed(patient, append([]evt(nil), evs...))
	counted := func(w *WindowAgg) int {
		n := 0
		for _, c := range finalCounts(w) {
			n += c
		}
		return n
	}
	t.Logf("strict: %d counted, %d dropped; with 10 s lateness: %d counted, %d dropped, %d window updates", counted(strict), strict.LateDropped, counted(patient), patient.LateDropped, patient.Updates)
	if counted(strict)+strict.LateDropped != len(evs) || counted(patient)+patient.LateDropped != len(evs) {
		t.Errorf("every event is either counted or dropped")
	}
	if r := float64(strict.LateDropped) / float64(len(evs)); math.Abs(r-0.1) > 0.02 {
		t.Errorf("about the late tenth are dropped when no lateness is allowed: %.3f", r)
	}
	if patient.LateDropped != 0 || patient.Updates == 0 {
		t.Errorf("10 s of lateness catches them all through window updates: dropped %d, updates %d", patient.LateDropped, patient.Updates)
	}
}

func TestSlidingWindowsPutAnEventInSizeOverSlideWindows(t *testing.T) {
	w := NewWindowAgg(10, 2, 0, 0) // 10 s windows every 2 s: 5 windows hold each event
	for i := range 1000 {
		w.Add(20 + float64(i)*0.01)
	}
	w.Flush()
	sum := 0
	for _, c := range finalCounts(w) {
		sum += c
	}
	if sum != 5*1000 {
		t.Errorf("1000 events in 5 windows each = 5000 window entries, got %d", sum)
	}
}
