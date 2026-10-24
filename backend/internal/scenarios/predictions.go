package scenarios

import "hash/fnv"

// shownOrder is the order the options of a prediction are shown in: shown[i] is the stored index of the option in
// position i. It comes from a hash of the challenge and question ids, so it is the same for every player and every
// request (the server needs no per-player state to map a choice back), and it does not follow the order the author
// wrote, which would otherwise leak the answer's position.
func (s *Scenario) shownOrder(q *Prediction) []int {
	n := len(q.Options)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	h := fnv.New64a()
	h.Write([]byte(s.ID + "/" + q.ID))
	x := h.Sum64()
	for i := n - 1; i > 0; i-- { // Fisher-Yates with a small xorshift stream
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		j := int(x % uint64(i+1))
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// ShownOptions are the options in the order a player sees them.
func (s *Scenario) ShownOptions(q *Prediction) []string {
	out := make([]string, len(q.Options))
	for pos, stored := range s.shownOrder(q) {
		out[pos] = q.Options[stored]
	}
	return out
}

// ShownAnswer is the position of the right option as shown.
func (s *Scenario) ShownAnswer(q *Prediction) int {
	for pos, stored := range s.shownOrder(q) {
		if stored == q.Answer {
			return pos
		}
	}
	return q.Answer
}

// StoredIndex turns a position the player chose back into the stored option index.
func (s *Scenario) StoredIndex(q *Prediction, shown int) int {
	o := s.shownOrder(q)
	if shown < 0 || shown >= len(o) {
		return -1
	}
	return o[shown]
}
