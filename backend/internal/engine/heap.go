package engine

// event is a scheduled callback. Ordering is (t, seq) so equal times run in insertion order.
type event struct {
	t   float64
	seq uint64
	fn  func()
}

type eventHeap struct{ a []event }

func less(x, y event) bool { return x.t < y.t || (x.t == y.t && x.seq < y.seq) }

// Len is the number of queued events.
func (h *eventHeap) Len() int { return len(h.a) }

// Push queues an event.
func (h *eventHeap) Push(e event) {
	h.a = append(h.a, e)
	i := len(h.a) - 1
	for i > 0 {
		p := (i - 1) / 2
		if !less(h.a[i], h.a[p]) {
			break
		}
		h.a[i], h.a[p] = h.a[p], h.a[i]
		i = p
	}
}

// peekTime returns the next event time or +Inf-like when empty.
func (h *eventHeap) peekTime() float64 {
	if len(h.a) == 0 {
		return 1e308
	}
	return h.a[0].t
}

// pop removes and returns the earliest event; ties go to the one queued first.
func (h *eventHeap) pop() event {
	top := h.a[0]
	n := len(h.a) - 1
	h.a[0] = h.a[n]
	h.a[n] = event{}
	h.a = h.a[:n]
	i := 0
	for {
		l, r, m := 2*i+1, 2*i+2, i
		if l < n && less(h.a[l], h.a[m]) {
			m = l
		}
		if r < n && less(h.a[r], h.a[m]) {
			m = r
		}
		if m == i {
			break
		}
		h.a[i], h.a[m] = h.a[m], h.a[i]
		i = m
	}
	return top
}
