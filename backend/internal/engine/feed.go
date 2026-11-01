package engine

import (
	"math"
	"sync"
)

// Feed workload: a post is written by an author and shown to the author's followers; a read fetches a timeline.
// Follower counts are Zipf-skewed over the author ranks, so a few "celebrities" have most of the followers. The
// fan-out is modelled as work, not as thousands of separate events: every timeline insert (push) or every followed
// author read (pull) adds feedUnitMs to the one database call that carries it.

// feedUnitMs is the database time of one timeline insert, or of reading one followed author's recent posts.
const feedUnitMs = 0.1

// celebsFollowed is how many celebrities a reader follows in the hybrid strategy: their posts are pulled at read time.
const celebsFollowed = 5

var (
	harmMu    sync.Mutex
	harmCache = map[[2]float64]float64{}
)

// harmonic is the sum of j^-s for j = 1..K, cached because it is needed for every request.
func harmonic(K int, s float64) float64 {
	key := [2]float64{float64(K), s}
	harmMu.Lock()
	defer harmMu.Unlock()
	if v, ok := harmCache[key]; ok {
		return v
	}
	sum := 0.0
	for j := 1; j <= K; j++ {
		sum += math.Pow(float64(j), -s)
	}
	harmCache[key] = sum
	return sum
}

// followerCount is how many followers the author of rank k (1 = the most followed) has among K authors: a Zipf
// share of mean x K follows in total, never less than one.
func followerCount(k, K int, mean, skew float64) float64 {
	if K < 1 || k < 1 {
		return 1
	}
	return max(1, math.Floor(mean*float64(K)*math.Pow(float64(k), -skew)/harmonic(K, skew)))
}

// feedFan works out how much extra database work a feed request carries and counts it; the answer is the number of
// units (timeline inserts for a post, followed authors read for a timeline).
func (s *Sim) feedFan(n *node, c *call) float64 {
	if c.lg == nil {
		return 0
	}
	cl := c.lg.client
	strategy := n.s("feedStrategy")
	if c.write {
		fol := followerCount(c.key, int(cl.f("keySpace")), cl.f("followersMean"), cl.f("followersSkew"))
		if strategy == "pullOnRead" || (strategy == "hybrid" && fol > n.f("celebrityThreshold")) {
			return 0 // nothing is pushed: readers will pull this author's posts
		}
		if c.counted {
			s.runFanWrites += int(fol)
		}
		return fol
	}
	var fan float64
	switch strategy {
	case "pullOnRead":
		fan = cl.f("followersMean") // a reader follows about as many authors as an author has followers
	case "hybrid":
		fan = celebsFollowed
	}
	if c.counted {
		s.runFanReads += int(fan)
	}
	return fan
}
