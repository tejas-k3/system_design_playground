package engine

// rankingMs is the model time a read spends scoring the candidate posts of a feed before it shows the best: it grows with
// the number of candidates, and a write is not ranked.
func (s *Sim) rankingMs(n *node, c *call) float64 {
	if c.write || !n.b("ranking") {
		return 0
	}
	return n.f("rankCandidates") * n.f("rankUsPerCandidate") / 1000
}
