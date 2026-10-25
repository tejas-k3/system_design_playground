package engine

import "math"

// Range caching. A cache or CDN with rangeKB set keeps a big object as ceil(size/rangeKB) pieces, each its own entry
// that is cached, expires and is evicted on its own. A read needs every piece; the pieces already there are served,
// the missing ones go to the origin in one call that carries only their bytes. With miss sharing on, a piece that
// someone is already fetching is waited for instead of asked for again, so each piece goes to the origin once.
//
// Simplifications: the object's size is the users' response size; the last piece is accounted at its true size but
// takes a whole slot of the byte capacity; ranges are not passed on to a second ranged tier (the next tier sees a
// call that carries fewer bytes but looks the whole object up again).

const maxPieces = 1000

// rangeOf says how many pieces this read splits into, the object's size and the piece size in bytes. pieces is 0
// when the node does not cut objects up (range off, no response size, or the object fits in one piece).
func (s *Sim) rangeOf(n *node, c *call) (pieces int, size, piece float64) {
	kb := n.f("rangeKB")
	if kb <= 0 || c.lg == nil {
		return 0, 0, 0
	}
	size = c.lg.client.f("respKB") * 1000
	piece = kb * 1000
	if size <= piece {
		return 0, 0, 0
	}
	pieces = int(size / piece)
	if float64(pieces)*piece < size {
		pieces++
	}
	if pieces > maxPieces {
		pieces = maxPieces
		piece = size / maxPieces
	}
	return pieces, size, piece
}

// pieceKey is the store key of one piece. Negative, so it never collides with a whole-object key.
func pieceKey(key, i int) int { return -(key*1024 + i + 1) }

// rangeLookup reads every piece of the object. It returns the pieces that are missing, the version of the first piece
// present, and whether any piece had expired.
func (s *Sim) rangeLookup(n *node, c *call, pieces int) (missing []int, ver int, expired bool) {
	for i := 0; i < pieces; i++ {
		v, hit, exp := n.store.Get(s.now, pieceKey(c.key, i))
		expired = expired || exp
		if !hit {
			missing = append(missing, i)
		} else if i == 0 || len(missing) == 0 {
			ver = v
		}
	}
	return missing, ver, expired
}

func (s *Sim) rangeSetAll(n *node, c *call, pieces, ver int) {
	for i := 0; i < pieces; i++ {
		n.c.evict += n.store.Set(s.now, pieceKey(c.key, i), ver)
	}
}

func (s *Sim) rangeDeleteAll(n *node, c *call, pieces int) {
	for i := 0; i < pieces; i++ {
		n.store.Delete(pieceKey(c.key, i))
	}
}

// rangeMiss fetches the missing pieces. send makes the call to the origin (with the reduced size already set on it)
// and passes the origin's answer to its callback; the answer reaches cb once every piece is in.
func (s *Sim) rangeMiss(n *node, c *call, missing []int, pieces int, size, piece float64, send func(*call, func(Result)), cb func(Result)) {
	share := n.b("coalesce") && !c.write
	pending := 1 // released after every piece has been sorted into "mine" or "waiting"
	var bad *Result
	var good Result
	finish := func() {
		pending--
		if pending == 0 {
			if bad != nil {
				s.passBack(n, *bad, cb)
			} else {
				s.passBack(n, good, cb)
			}
		}
	}
	step := func(res Result) {
		if !res.OK {
			if bad == nil {
				r := res
				bad = &r
			}
		} else {
			good = res
		}
		finish()
	}
	var mine []int
	bytes := 0.0
	for _, i := range missing {
		k := pieceKey(c.key, i)
		if share {
			if waiters, busy := n.fetching[k]; busy {
				n.fetching[k] = append(waiters, step)
				pending++
				continue
			}
			if n.fetching == nil {
				n.fetching = map[int][]func(Result){}
			}
			n.fetching[k] = nil
		}
		mine = append(mine, i)
		if i == pieces-1 {
			bytes += size - float64(pieces-1)*piece
		} else {
			bytes += piece
		}
	}
	if len(mine) > 0 {
		pending++
		cc := *c
		cc.bodyB = int32(bytes)
		n.fillBytes += bytes
		send(&cc, func(res Result) {
			if res.OK && res.Ver >= 0 {
				for _, i := range mine {
					n.c.evict += n.store.Set(s.now, pieceKey(c.key, i), res.Ver)
				}
			}
			for _, i := range mine {
				if share {
					k := pieceKey(c.key, i)
					waiters := n.fetching[k]
					delete(n.fetching, k)
					for _, w := range waiters {
						w(res)
					}
				}
			}
			step(res)
		})
	}
	finish()
}

// preloadRanged fills a ranged store with the pieces of objects 1..n, as if warmed up.
func (s *Sim) preloadRanged(n *node, objects, pieces int) {
	for k := 1; k <= objects; k++ {
		for i := 0; i < pieces; i++ {
			if n.store.Size() >= n.store.Capacity() {
				return
			}
			n.store.Set(0, pieceKey(k, i), 0)
		}
	}
}

// rangePiecesOf is how many pieces the biggest response any client asks for splits into on this node (0 = not ranged).
func (s *Sim) rangePiecesOf(n *node) int {
	kb := n.f("rangeKB")
	if kb <= 0 {
		return 0
	}
	most := 0.0
	for _, c := range s.order {
		if c.kind == KClient && c.f("respKB") > most {
			most = c.f("respKB")
		}
	}
	if most <= kb {
		return 0
	}
	return min(maxPieces, int(math.Ceil(most/kb)))
}
