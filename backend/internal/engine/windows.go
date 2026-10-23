package engine

import (
	"math"
	"sort"
)

// WindowAgg counts events in windows of event time (when the event happened, not when it arrived). A window is
// emitted once the watermark, the newest event time seen minus Delay, passes its end. An event for a window already
// emitted updates it (and the window is emitted again) as long as the watermark has not passed the end by more than
// Lateness; after that the event is dropped. Size == Slide gives tumbling windows; a smaller Slide gives sliding
// windows, and an event then belongs to Size / Slide of them.
type WindowAgg struct {
	Size, Slide, Delay, Lateness float64
	Emitted                      []Emission
	LateDropped, Updates         int

	maxEvent float64
	open     map[int64]*winState
}

type winState struct {
	count   int
	emitted bool
}

// Emission is one result of a window: its first firing, or an update after a late event.
type Emission struct {
	Start, End float64
	Count      int
	Update     bool
}

func NewWindowAgg(size, slide, delay, lateness float64) *WindowAgg {
	return &WindowAgg{Size: size, Slide: slide, Delay: delay, Lateness: lateness, maxEvent: math.Inf(-1), open: map[int64]*winState{}}
}

func (w *WindowAgg) watermark() float64 { return w.maxEvent - w.Delay }

func (w *WindowAgg) Add(at float64) {
	if at > w.maxEvent {
		w.maxEvent = at
	}
	wm := w.watermark()
	placed := false
	for k := int64(math.Floor(at / w.Slide)); float64(k)*w.Slide+w.Size > at; k-- {
		end := float64(k)*w.Slide + w.Size
		if wm >= end+w.Lateness {
			continue // too late for this window
		}
		placed = true
		st := w.open[k]
		if st == nil {
			st = &winState{}
			w.open[k] = st
		}
		st.count++
		if st.emitted {
			w.Updates++
			w.Emitted = append(w.Emitted, Emission{float64(k) * w.Slide, end, st.count, true})
		}
	}
	if !placed {
		w.LateDropped++
	}
	w.emitReady(wm)
}

func (w *WindowAgg) keys() []int64 {
	ks := make([]int64, 0, len(w.open))
	for k := range w.open {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	return ks
}

func (w *WindowAgg) emitReady(wm float64) {
	for _, k := range w.keys() {
		st, end := w.open[k], float64(k)*w.Slide+w.Size
		if !st.emitted && wm >= end {
			st.emitted = true
			w.Emitted = append(w.Emitted, Emission{float64(k) * w.Slide, end, st.count, false})
		}
		if wm >= end+w.Lateness { // nothing can change it any more
			delete(w.open, k)
		}
	}
}

// Flush emits every window that has not fired yet, at the end of the stream.
func (w *WindowAgg) Flush() {
	for _, k := range w.keys() {
		if st := w.open[k]; !st.emitted {
			st.emitted = true
			w.Emitted = append(w.Emitted, Emission{float64(k) * w.Slide, float64(k)*w.Slide + w.Size, st.count, false})
		}
	}
}
