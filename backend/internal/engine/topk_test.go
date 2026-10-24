package engine

import "testing"

// The ten most popular items out of 200,000, from a stream of 300,000 requests with Zipf popularity. Counting every
// item exactly takes a counter for each item seen; a count-min sketch with a short candidate list takes a fixed few
// thousand counters and still finds nearly the same ten.
func topKStream() []uint32 {
	var ks KeySampler
	rng := NewRNG(21)
	out := make([]uint32, 300000)
	for i := range out {
		out[i] = uint32(ks.Sample("zipf", 200000, 1.1, 0, rng))
	}
	return out
}

func TestSketchTopKFindsMostOfTheTrueTopTen(t *testing.T) {
	stream := topKStream()
	exact := NewExactTopK()
	sk := NewSketchTopK(10, 1000, 3, 7)
	for _, x := range stream {
		exact.Add(x)
		sk.Add(x)
	}
	truth, got := exact.Top(10), sk.Top(10)
	hit := 0
	in := map[uint32]bool{}
	for _, x := range truth {
		in[x] = true
	}
	for _, x := range got {
		if in[x] {
			hit++
		}
	}
	t.Logf("recall@10 %d/10; memory: exact %d counters, sketch %d counters + %d candidates", hit, exact.Counters(), sk.Counters(), sk.Candidates())
	if hit < 9 {
		t.Errorf("the sketch should find at least 9 of the true top 10: %d", hit)
	}
	if sk.Counters()*10 > exact.Counters() {
		t.Errorf("the sketch should use a small fraction of the exact table's memory: %d vs %d", sk.Counters(), exact.Counters())
	}
	// a sketch narrower than the crowd of rare items finds fewer
	tiny := NewSketchTopK(10, 40, 2, 7)
	for _, x := range stream {
		tiny.Add(x)
	}
	tinyHit := 0
	for _, x := range tiny.Top(10) {
		if in[x] {
			tinyHit++
		}
	}
	t.Logf("a 40 x 2 sketch: recall@10 %d/10", tinyHit)
	if tinyHit >= hit {
		t.Errorf("less memory should cost accuracy: %d vs %d", tinyHit, hit)
	}
}
