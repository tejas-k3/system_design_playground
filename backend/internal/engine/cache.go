package engine

import "container/list"

type entry struct {
	key  int
	ver  int
	exp  float64 // 0 = never expires
	freq int
	el   *list.Element
	pos  int // index in random-policy slice
}

// Store is a bounded key->version cache with LRU, LFU, FIFO or random eviction and optional TTL.
type Store struct {
	policy            string
	cap               int
	ttlMs             float64
	jitter            float64 // 0..0.9: each entry lives ttl*(1 +/- jitter), spread deterministically by key and time
	shortPct, shortMs float64 // a share of the items (by key) live shortMs instead of ttlMs
	rng               *RNG
	m                 map[int]*entry
	ll                *list.List         // lru / fifo: front = newest
	freq              map[int]*list.List // lfu buckets
	minF              int
	keys              []int // random policy
}

// NewStore returns a cache of the given eviction policy (lru, lfu, fifo or random; anything else means lru), capacity
// and time to live.
func NewStore(policy string, capacity int, ttlMs float64, rng *RNG) *Store {
	switch policy {
	case "lru", "lfu", "fifo", "random":
	default:
		policy = "lru"
	}
	if capacity < 0 {
		capacity = 0
	}
	return &Store{policy: policy, cap: capacity, ttlMs: ttlMs, rng: rng, m: map[int]*entry{}, ll: list.New(), freq: map[int]*list.List{}, minF: 1}
}

// Size is the number of entries held.
func (s *Store) Size() int { return len(s.m) }

// Capacity is the number of entries the store may hold.
func (s *Store) Capacity() int { return s.cap }

// SetTTL changes the time to live of entries stored from now on.
func (s *Store) SetTTL(ms float64) { s.ttlMs = ms }

// SetJitter spreads each time to live by up to this fraction, so entries do not all expire together.
func (s *Store) SetJitter(f float64) { s.jitter = f }

// SetShortTTL gives a share of the items (chosen by key, so the same items every time) a shorter time to live.
func (s *Store) SetShortTTL(pct, ms float64) { s.shortPct, s.shortMs = pct, ms }

// Expired returns the version of an entry that is held but past its time to live, without removing it.
func (s *Store) Expired(now float64, key int) (int, bool) {
	e, ok := s.m[key]
	if !ok || e.exp == 0 || now < e.exp {
		return 0, false
	}
	return e.ver, true
}

// SetCapacity resizes the store, evicting if it shrank. Returns evictions.
func (s *Store) SetCapacity(c int) int {
	if c < 0 {
		c = 0
	}
	s.cap = c
	n := 0
	for len(s.m) > s.cap {
		s.evict()
		n++
	}
	return n
}

// Get returns (version, hit, expired).
func (s *Store) Get(now float64, key int) (int, bool, bool) {
	e, ok := s.m[key]
	if !ok {
		return 0, false, false
	}
	if e.exp > 0 && now >= e.exp {
		s.remove(e)
		return 0, false, true
	}
	switch s.policy {
	case "lru":
		s.ll.MoveToFront(e.el)
	case "lfu":
		s.bump(e)
	}
	return e.ver, true, false
}

// Live reports whether a key is held and not expired, without touching recency or removing anything.
func (s *Store) Live(now float64, key int) bool {
	e, ok := s.m[key]
	return ok && (e.exp == 0 || now < e.exp)
}

// Set inserts or refreshes a key and returns how many entries were evicted.
func (s *Store) Set(now float64, key, ver int) int {
	if s.cap == 0 {
		return 0
	}
	exp := 0.0
	if s.ttlMs > 0 {
		life := s.ttlMs
		if s.shortPct > 0 && s.shortMs > 0 && float64(Hash32(uint32(key)*2246822519+17)%100) < s.shortPct {
			life = min(life, s.shortMs)
		}
		if s.jitter > 0 {
			u := float64(Hash32(uint32(key)*0x9e3779b1^uint32(now))) / 4294967296
			life *= 1 + s.jitter*(2*u-1)
		}
		exp = now + life
	}
	if e, ok := s.m[key]; ok {
		e.ver, e.exp = ver, exp
		if s.policy == "lru" {
			s.ll.MoveToFront(e.el)
		} else if s.policy == "lfu" {
			s.bump(e)
		}
		return 0
	}
	ev := 0
	for len(s.m) >= s.cap {
		s.evict()
		ev++
	}
	e := &entry{key: key, ver: ver, exp: exp, freq: 1}
	s.m[key] = e
	switch s.policy {
	case "lru", "fifo":
		e.el = s.ll.PushFront(e)
	case "lfu":
		s.bucket(1).PushFront(e)
		e.el = s.freq[1].Front()
		s.minF = 1
	case "random":
		e.pos = len(s.keys)
		s.keys = append(s.keys, key)
	}
	return ev
}

// Delete removes the entry for key, if any.
func (s *Store) Delete(key int) {
	if e, ok := s.m[key]; ok {
		s.remove(e)
	}
}

// Preload fills the store with keys 1..n (as if warmed up).
func (s *Store) Preload(n int, ver int) {
	for k := 1; k <= n && len(s.m) < s.cap; k++ {
		s.Set(0, k, ver)
	}
}

func (s *Store) bucket(f int) *list.List {
	b := s.freq[f]
	if b == nil {
		b = list.New()
		s.freq[f] = b
	}
	return b
}

func (s *Store) bump(e *entry) {
	old := s.freq[e.freq]
	old.Remove(e.el)
	if old.Len() == 0 {
		delete(s.freq, e.freq)
		if s.minF == e.freq {
			s.minF = e.freq + 1
		}
	}
	e.freq++
	e.el = s.bucket(e.freq).PushFront(e)
}

func (s *Store) evict() {
	switch s.policy {
	case "lru", "fifo":
		if b := s.ll.Back(); b != nil {
			s.remove(b.Value.(*entry))
		}
	case "lfu":
		for f := s.minF; ; f++ {
			if b := s.freq[f]; b != nil && b.Len() > 0 {
				s.remove(b.Back().Value.(*entry))
				return
			}
			if f > s.minF+1_000_000 {
				return
			}
		}
	case "random":
		if len(s.keys) > 0 {
			s.remove(s.m[s.keys[int(s.rng.Next()*float64(len(s.keys)))]])
		}
	}
}

func (s *Store) remove(e *entry) {
	delete(s.m, e.key)
	switch s.policy {
	case "lru", "fifo":
		s.ll.Remove(e.el)
	case "lfu":
		b := s.freq[e.freq]
		b.Remove(e.el)
		if b.Len() == 0 {
			delete(s.freq, e.freq)
		}
	case "random":
		last := len(s.keys) - 1
		if e.pos != last {
			moved := s.keys[last]
			s.keys[e.pos] = moved
			s.m[moved].pos = e.pos
		}
		s.keys = s.keys[:last]
	}
}

// Clear drops every entry (a purge) and returns how many there were.
func (s *Store) Clear() int {
	n := len(s.m)
	s.m, s.ll, s.freq, s.minF, s.keys = map[int]*entry{}, list.New(), map[int]*list.List{}, 1, nil
	return n
}
