package engine

// A leaky bucket does not turn a burst away: it lets it out at the steady rate. Each request is given the next free
// slot, 1/limit apart; one that would have to wait for more than a queue's worth (burst) of slots is refused. So the
// servers behind see a smooth stream and the price is the wait, not a refusal, until the queue is full.

// leak returns whether the request fits and how long (ms) it waits for its slot.
func (s *Sim) leak(n *node, c *call) (bool, float64) {
	if n.b("shared") {
		if s.sharedLimit == nil || s.sharedLimit.removed {
			s.sharedLimit = n
		}
		n = s.sharedLimit
	}
	gap := 1000 / n.f("limitRps")
	next := &n.leakNext
	if key := limiterKey(n, c); key != 0 {
		if n.buckets == nil {
			n.buckets = map[int]*bucket{}
		}
		b := n.buckets[key]
		if b == nil {
			b = &bucket{}
			n.buckets[key] = b
		}
		next = &b.leakNext
	}
	slot := max(s.now, *next)
	if (slot-s.now)/gap > n.f("burst") {
		return false, 0
	}
	*next = slot + gap
	return true, slot - s.now
}

// limiterKey is what a limiter counts separately: the account (keyBy user), the tenant the account belongs to
// (keyBy tenant), or 0 for everything together.
func limiterKey(n *node, c *call) int {
	switch n.s("keyBy") {
	case "user":
		return c.user
	case "tenant":
		return tenantOf(c)
	}
	return 0
}

// tenantOf groups a client's accounts into its tenants in blocks, so the first tenant holds the lowest-numbered
// accounts, which with a skewed identity distribution are the busiest: the noisy neighbour.
func tenantOf(c *call) int {
	if c.user == 0 || c.lg == nil {
		return 0
	}
	return tenantOfUser(c.lg.client, c.user)
}

func tenantOfUser(cl *node, user int) int {
	t, ids := int(cl.f("tenants")), int(cl.f("identities"))
	if user == 0 || t < 1 || ids < 1 {
		return 0
	}
	idx := user - cl.idBase // 1-based among the client's accounts
	return 1 + min(t-1, (idx-1)*t/ids)
}

func (s *Sim) tenantCount() int {
	n := 0
	for _, t := range s.tenantTotal {
		if t > 0 {
			n++
		}
	}
	return n
}

// worstTenant is the success rate of the worst-served tenant other than the busiest one (the noisy neighbour is not
// who a limit is meant to protect). 0 when fewer than two tenants are tracked.
func (s *Sim) worstTenant() float64 {
	busiest, best := -1, 0
	for i, t := range s.tenantTotal {
		if t > best {
			busiest, best = i, t
		}
	}
	worst, seen := 1.0, false
	for i, t := range s.tenantTotal {
		if t == 0 || i == busiest {
			continue
		}
		seen = true
		worst = min(worst, float64(s.tenantOK[i])/float64(t))
	}
	if !seen {
		return 0
	}
	return worst
}
