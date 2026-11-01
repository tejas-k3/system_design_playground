package engine

// The share of all counted requests that go to the single busiest item, so a hot key shows up as a number. Items are
// counted in a fixed table of buckets (a few collisions can only make the busiest bucket slightly busier).
const hotBuckets = 4096

func (s *Sim) countKey(key int) {
	if s.keyCount == nil {
		s.keyCount = make([]int32, hotBuckets)
	}
	s.keyCount[hash32(uint32(key))%hotBuckets]++
	s.keyTotal++
}

func (s *Sim) hottestKeyShare() float64 {
	if s.keyTotal == 0 {
		return 0
	}
	var top int32
	for _, c := range s.keyCount {
		if c > top {
			top = c
		}
	}
	return float64(top) / float64(s.keyTotal)
}
