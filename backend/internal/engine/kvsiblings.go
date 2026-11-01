package engine

import "sort"

// Conflicts in the quorum store. Two clients that write the same key without having seen each other's write are
// concurrent. With last-write-wins the later acknowledgement silently replaces the earlier one (an update is lost).
// With siblings the store keeps both: every write carries a version vector, one counter per writer; a copy keeps a
// value unless a newer write has a vector that is at least as large in every entry (it has seen it). A read returns
// all the values that are left, and a client that has read them and writes again produces a vector that covers them,
// which replaces them.

type kvVVEnt struct{ w, n int } // writer, number of its writes seen
type kvVV []kvVVEnt             // sorted by writer

type kvSib struct {
	vv  kvVV
	ver int // the store's version number of that write
	by  int // the writer
}

// bump returns a copy of v with writer w's counter raised by one.
func (v kvVV) bump(w int) kvVV {
	out := make(kvVV, 0, len(v)+1)
	done := false
	for _, e := range v {
		if e.w == w {
			e.n++
			done = true
		} else if e.w > w && !done {
			out = append(out, kvVVEnt{w, 1})
			done = true
		}
		out = append(out, e)
	}
	if !done {
		out = append(out, kvVVEnt{w, 1})
	}
	return out
}

// merge returns the entry-wise maximum.
func (v kvVV) merge(o kvVV) kvVV {
	out := make(kvVV, 0, len(v)+len(o))
	i, j := 0, 0
	for i < len(v) || j < len(o) {
		switch {
		case j == len(o) || (i < len(v) && v[i].w < o[j].w):
			out = append(out, v[i])
			i++
		case i == len(v) || o[j].w < v[i].w:
			out = append(out, o[j])
			j++
		default:
			out = append(out, kvVVEnt{v[i].w, max(v[i].n, o[j].n)})
			i, j = i+1, j+1
		}
	}
	return out
}

// dominates reports whether v has seen everything o has (v >= o in every entry).
func (v kvVV) dominates(o kvVV) bool {
	i := 0
	for _, e := range o {
		for i < len(v) && v[i].w < e.w {
			i++
		}
		if i == len(v) || v[i].w != e.w || v[i].n < e.n {
			return false
		}
	}
	return true
}

// kvAddSibling puts x into a copy's set: dropped if some value there has already seen it, otherwise it replaces
// every value it has seen and the rest stay beside it.
func kvAddSibling(set []kvSib, x kvSib) []kvSib {
	for _, s := range set {
		if s.vv.dominates(x.vv) {
			return set
		}
	}
	out := make([]kvSib, 0, len(set)+1)
	for _, s := range set {
		if !x.vv.dominates(s.vv) {
			out = append(out, s)
		}
	}
	return append(out, x)
}

func kvUnion(a, b []kvSib) []kvSib {
	for _, s := range b {
		a = kvAddSibling(a, s)
	}
	return a
}

type kvWho struct {
	n *node
	u int
}

type kvLast struct{ ver, by int }

// kvExtra is everything the key-value store keeps beyond versions: who wrote what, conflicts, ring placement.
type kvExtra struct {
	ids    map[kvWho]int
	writes int
	// last-write-wins bookkeeping: what each writer has seen of each key, and who wrote it last
	seen map[int]map[int]int
	last map[int]kvLast
	// siblings
	ctx        map[int]map[int]kvVV // writer -> key -> what it has read or written
	cnt        map[int]int
	truth      map[int][]kvSib // the set every copy would hold if all writes had arrived everywhere
	sib        []map[int][]kvSib
	hintSibs   map[int][]kvHintSib
	lost       int
	concurrent int
	seenSibs   int
	sibReads   int
	resolved   int
	ring       *kvRing
	ringNodes  int
	ringVnodes int
}

type kvHintSib struct {
	key int
	s   kvSib
}

func (n *node) kvX() *kvExtra {
	if n.kx == nil {
		n.kx = &kvExtra{ids: map[kvWho]int{}, seen: map[int]map[int]int{}, last: map[int]kvLast{}}
	}
	return n.kx
}

// writer numbers the clients (and users behind them) in the order they first appear, from 1.
func (x *kvExtra) writer(c *call) int {
	if c.lg == nil || c.lg.client == nil {
		return 0
	}
	k := kvWho{c.lg.client, c.lg.user}
	id, ok := x.ids[k]
	if !ok {
		id = len(x.ids) + 1
		x.ids[k] = id
	}
	return id
}

func (x *kvExtra) copies(nodes int) []map[int][]kvSib {
	for len(x.sib) < nodes {
		x.sib = append(x.sib, map[int][]kvSib{})
	}
	return x.sib
}

// sawVersion records that a writer has seen version v of a key (last-write-wins bookkeeping).
func (x *kvExtra) sawVersion(w, key, v int) {
	m := x.seen[w]
	if m == nil {
		m = map[int]int{}
		x.seen[w] = m
	}
	if v > m[key] {
		m[key] = v
	}
}

// nextVV is the vector of a write a client is about to make: what it has seen, plus one more write by itself.
func (x *kvExtra) nextVV(w, key int) (base, next kvVV) {
	if x.ctx == nil {
		x.ctx, x.cnt = map[int]map[int]kvVV{}, map[int]int{}
	}
	base = x.ctx[w][key]
	return base, base.bump(w)
}

func (x *kvExtra) learn(w, key int, vv kvVV) {
	if x.ctx[w] == nil {
		x.ctx[w] = map[int]kvVV{}
	}
	x.ctx[w][key] = x.ctx[w][key].merge(vv)
}

// committed is called when a write is acknowledged (it gets its version).
func (x *kvExtra) committed(siblings bool, w, key, ver int, base, next kvVV) {
	x.writes++
	if !siblings {
		if rec, ok := x.last[key]; ok && rec.by != w && x.seen[w][key] < rec.ver {
			x.lost++ // it replaces a value its writer had not seen
			x.concurrent++
		}
		x.last[key] = kvLast{ver, w}
		x.sawVersion(w, key, ver)
		return
	}
	if x.truth == nil {
		x.truth = map[int][]kvSib{}
	}
	before := x.truth[key]
	meet := false
	for _, s := range before {
		if !next.dominates(s.vv) {
			meet = true // a value this write has not seen stays beside it
		} else if s.by != w && !base.dominates(s.vv) {
			x.lost++ // dropped without its writer having been seen: cannot happen with vectors
		}
	}
	if meet {
		x.concurrent++
	}
	after := kvAddSibling(before, kvSib{next, ver, w})
	if len(before) > 1 && len(after) == 1 {
		x.resolved++
	}
	x.truth[key] = after
	x.learn(w, key, next)
}

// open is the number of values that are in conflict now: keys with two or more, counting all their values.
func (x *kvExtra) open() int {
	total := 0
	for _, set := range x.truth {
		if len(set) > 1 {
			total += len(set)
		}
	}
	return total
}

func (x *kvExtra) extras(ex map[string]float64, siblings bool) {
	if x.lost > 0 || siblings {
		ex["lostUpdates"] = float64(x.lost)
	}
	if x.concurrent > 0 {
		ex["concurrentWrites"] = float64(x.concurrent)
	}
	if siblings {
		ex["siblingsOpen"] = float64(x.open())
		ex["siblingsSeen"] = float64(x.seenSibs)
		ex["siblingReads"] = float64(x.sibReads)
		ex["siblingsResolved"] = float64(x.resolved)
	}
}

// Ring placement. The nodes sit on a hash ring at `vnodes` points each; a key's copies are the first distinct
// nodes met going round from the key's hash. Adding a node adds only its own points, so it takes over only about
// 1/n of the keys; hash mod n would remap nearly all of them.

type kvPoint struct {
	h    uint32
	node int
}

type kvRing struct {
	nodes int
	pts   []kvPoint
	cache map[int][]int
}

func kvKeyHash(key int) uint32 { return Hash32(uint32(key)*2654435761 + 11) }

// kvModFirst is where hash mod n puts the first copy.
func kvModFirst(key, n int) int { return int(kvKeyHash(key) % uint32(n)) }

func newKVRing(nodes, vnodes int) *kvRing {
	r := &kvRing{nodes: nodes, cache: map[int][]int{}}
	for i := range nodes {
		base := Hash32(uint32(i) + 0x5bd1e995)
		for v := range vnodes {
			r.pts = append(r.pts, kvPoint{Hash32(base + uint32(v)*0x9e3779b1), i})
		}
	}
	sort.Slice(r.pts, func(a, b int) bool {
		if r.pts[a].h != r.pts[b].h {
			return r.pts[a].h < r.pts[b].h
		}
		return r.pts[a].node < r.pts[b].node
	})
	return r
}

const kvRingCache = 1 << 16

// order returns the first `want` distinct nodes clockwise from the key.
func (r *kvRing) order(key, want int) []int {
	want = clampInt(want, 1, r.nodes)
	if want < r.nodes {
		if o, ok := r.cache[key]; ok && len(o) == want {
			return o
		}
	}
	kh := kvKeyHash(key)
	i := sort.Search(len(r.pts), func(i int) bool { return r.pts[i].h >= kh })
	out := make([]int, 0, want)
	var taken uint64
	for step := 0; len(out) < want && step < len(r.pts); step++ {
		p := r.pts[(i+step)%len(r.pts)]
		if bit := uint64(1) << uint(p.node); taken&bit == 0 {
			taken |= bit
			out = append(out, p.node)
		}
	}
	if want < r.nodes && len(r.cache) < kvRingCache {
		r.cache[key] = out
	}
	return out
}

// place returns the node order for a key: nil means hash mod n (node (first + j) mod n).
func (x *kvExtra) place(n *node, nodes, copies int, key int, full bool) []int {
	if n.s("placement") != "ring" {
		return nil
	}
	v := clampInt(int(n.f("vnodes")), 1, 512)
	if x.ring == nil || x.ringNodes != nodes || x.ringVnodes != v {
		x.ring, x.ringNodes, x.ringVnodes = newKVRing(nodes, v), nodes, v
	}
	if full {
		return x.ring.order(key, nodes)
	}
	return x.ring.order(key, copies)
}
