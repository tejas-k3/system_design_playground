package engine

import "math"

// Time to join one row of a friend list in a relational database; a graph database follows a pointer instead, which
// costs a tenth of it.
const (
	joinRowMs    = 0.005
	pointerRowMs = 0.0005
)

// graphMs is the time a read adds by following the friend links of its user: each hop reaches as many rows as the user
// has friends (or the cap, if the database follows at most that many) times the rows of the hop before. A write and a
// plain read follow no links. A read replica has no data model or cap of its own, so it pays the join price for every
// friend: sending the reads to a replica does not make them cheaper.
func (s *Sim) graphMs(n *node, c *call) float64 {
	if c.write || c.lg == nil || (n.kind != KDatabase && n.kind != KReplica) { // a replica joins like a relational database and has no cap
		return 0
	}
	hops := int(c.lg.client.f("friendHops"))
	if hops < 1 {
		return 0
	}
	per := c.lg.client.f("friendsEach")
	if cp := n.f("graphCap"); cp > 0 && cp < per {
		per = cp
	}
	rows := 0.0
	for k := 1; k <= hops; k++ {
		rows += math.Pow(per, float64(k))
	}
	unit := joinRowMs
	if n.s("model") == "graph" {
		unit = pointerRowMs
	}
	return rows * unit
}
