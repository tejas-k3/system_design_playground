package engine

import (
	"math"
	"sort"
)

// windowAgg counts events in windows of event time (when the event happened, not when it arrived). A window is
// emitted once the watermark, the newest event time seen minus Delay, passes its end. An event for a window already
// emitted updates it (and the window is emitted again) as long as the watermark has not passed the end by more than
// Lateness; after that the event is dropped. Size == Slide gives tumbling windows; a smaller Slide gives sliding
// windows, and an event then belongs to Size / Slide of them.
type windowAgg struct {
	Size, Slide, Delay, Lateness float64
	Emitted                      []windowEmission
	LateDropped, Updates         int

	maxEvent float64
	open     map[int64]*winState
}

type winState struct {
	count   int
	emitted bool
}

// windowEmission is one result of a window: its first firing, or an update after a late event.
type windowEmission struct {
	Start, End float64
	Count      int
	Update     bool
}

// newWindowAgg returns an aggregator of event-time windows of the given size and slide, a watermark that trails the
// newest event by delay, and an allowed lateness (all in milliseconds).
func newWindowAgg(size, slide, delay, lateness float64) *windowAgg {
	return &windowAgg{Size: size, Slide: slide, Delay: delay, Lateness: lateness, maxEvent: math.Inf(-1), open: map[int64]*winState{}}
}

func (w *windowAgg) watermark() float64 { return w.maxEvent - w.Delay }

// Add places an event with the given event time into every window it belongs to, and drops it when all of them are
// already closed.
func (w *windowAgg) Add(at float64) {
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
			w.Emitted = append(w.Emitted, windowEmission{float64(k) * w.Slide, end, st.count, true})
		}
	}
	if !placed {
		w.LateDropped++
	}
	w.emitReady(wm)
}

func (w *windowAgg) keys() []int64 {
	ks := make([]int64, 0, len(w.open))
	for k := range w.open {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	return ks
}

func (w *windowAgg) emitReady(wm float64) {
	for _, k := range w.keys() {
		st, end := w.open[k], float64(k)*w.Slide+w.Size
		if !st.emitted && wm >= end {
			st.emitted = true
			w.Emitted = append(w.Emitted, windowEmission{float64(k) * w.Slide, end, st.count, false})
		}
		if wm >= end+w.Lateness { // nothing can change it any more
			delete(w.open, k)
		}
	}
}

// Flush emits every window that has not fired yet, at the end of the stream.
func (w *windowAgg) Flush() {
	for _, k := range w.keys() {
		if st := w.open[k]; !st.emitted {
			st.emitted = true
			w.Emitted = append(w.Emitted, windowEmission{float64(k) * w.Slide, float64(k)*w.Slide + w.Size, st.count, false})
		}
	}
}

// StreamResult is what the sink received from a stream job that may have crashed once.
type StreamResult struct {
	Delivered      []windowEmission
	Duplicates     int // outputs the sink received for a window it had already been given
	Checkpoints    int
	CheckpointCost float64 // relative cost: at-least-once only saves state, exactly-once also commits the outputs with it
}

func (w *windowAgg) clone() *windowAgg {
	c := *w
	c.Emitted = append([]windowEmission(nil), w.Emitted...)
	c.open = make(map[int64]*winState, len(w.open))
	for k, v := range w.open {
		s := *v
		c.open[k] = &s
	}
	return &c
}

// runStream feeds events (their event times, in arrival order) to a windowed aggregation that checkpoints every
// checkpointSec of event time and crashes once, just before event number crashAt (-1 = never). After a crash it
// restores the last checkpoint and reads the input again from there. With atLeastOnce every output goes to the sink as
// soon as it is produced, so the ones produced since the checkpoint are produced, and delivered, again. With
// exactlyOnce outputs are held and handed over together with the next checkpoint, so a crash discards the held ones.
func runStream(events []float64, size, lateness, checkpointSec float64, delivery string, crashAt int) StreamResult {
	var res StreamResult
	w := newWindowAgg(size, size, 0, lateness)
	snap, snapIdx, delivered, nextCP := w.clone(), 0, 0, checkpointSec
	flushOut := func() {
		res.Delivered = append(res.Delivered, w.Emitted[delivered:]...)
		delivered = len(w.Emitted)
	}
	crashed := false
	for i := 0; i < len(events); i++ {
		if i == crashAt && !crashed {
			crashed = true
			w = snap.clone()
			delivered = len(w.Emitted) // what the checkpoint already handed over
			i = snapIdx - 1
			continue
		}
		w.Add(events[i])
		if delivery == "atLeastOnce" {
			flushOut()
		}
		if events[i] >= nextCP {
			nextCP += checkpointSec
			res.Checkpoints++
			res.CheckpointCost++
			if delivery == "exactlyOnce" {
				flushOut()
				res.CheckpointCost++
			}
			snap, snapIdx = w.clone(), i+1
		}
	}
	w.Flush()
	flushOut()
	seen := map[[2]float64]bool{}
	for _, e := range res.Delivered {
		k := [2]float64{e.Start, map[bool]float64{true: 1}[e.Update]}
		if seen[k] {
			res.Duplicates++
		}
		seen[k] = true
	}
	return res
}
