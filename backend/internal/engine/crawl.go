package engine

import (
	"math"
	"sort"
)

// The web-crawl workload (client workload "crawl"). Every request fetches the next URL of a frontier. Fetching a page
// reveals its links: some point at pages already found, the others are new URLs on sites chosen by Zipf popularity (a
// few sites are huge). A seen-URL check decides which links join the frontier: none (known pages are fetched again),
// an exact set (nothing repeats, memory grows with every distinct URL) or a Bloom filter (memory is fixed, but each
// false positive is a new page skipped by mistake, so the crawl's recall falls). A trap site links every page to new
// pages forever; maxDepth stops following links from pages that deep.
//
// All choices come from hashes of URL numbers, so no random number is drawn and the run stays deterministic.

// CrawlStats is what a crawl workload reports.
type CrawlStats struct {
	Fetched           int     `json:"fetched"`           // pages fetched (requests made for a URL of the frontier)
	Discovered        int     `json:"discovered"`        // links found on fetched pages
	DuplicatesSkipped int     `json:"duplicatesSkipped"` // links to known pages that the seen-URL check kept out of the frontier
	Missed            int     `json:"missed"`            // new pages skipped by mistake: Bloom false positives
	Refetched         int     `json:"refetched"`         // fetches of a page already fetched (no seen-URL check)
	TrapDepth         int     `json:"trapDepth"`         // deepest page fetched on the trap site
	Frontier          int     `json:"frontier"`          // URLs waiting to be fetched
	DedupKB           float64 `json:"dedupKB"`           // memory of the seen-URL check
	ExpectedMissed    float64 `json:"expectedMissed"`    // misses the Bloom formula predicts for the filter's fill at each check
	RecallPct         float64 `json:"recallPct"`         // share of new pages that were not skipped by mistake
	Dropped           int     `json:"dropped,omitempty"` // links lost because the frontier was full
}

type crawlPage struct {
	id    uint32
	host  int32
	depth int32
	trap  bool
	dup   bool // a repeat fetch of a known page: its links are known already
}

type crawlRef struct {
	id   uint32
	host int32
}

type crawlState struct {
	cdf      []float64
	skew     float64
	q        []crawlPage
	head     int
	known    []crawlRef
	next     uint32
	bloom    *Bloom
	bloomKB  float64
	added    int // URLs in the seen set
	fresh    int // links to pages not found before
	fetched  int
	found    int
	skipped  int
	missed   int
	repeats  int
	trapDeep int
	dropped  int
	expected float64
}

const (
	crawlFrontierCap = 2_000_000
	crawlEntryBytes  = 16 // an exact set holds a fingerprint and a table slot per URL
)

func (s *Sim) crawlOf(c *node) *crawlState {
	if s.crawl == nil {
		s.crawl = map[string]*crawlState{}
	}
	st := s.crawl[c.id]
	if st == nil {
		st = &crawlState{}
		s.crawl[c.id] = st
	}
	hosts, skew := max(1, int(c.f("crawlHosts"))), c.f("crawlHostSkew")
	if len(st.cdf) != hosts || st.skew != skew {
		st.cdf, st.skew = make([]float64, hosts), skew
		sum := 0.0
		for i := range st.cdf {
			sum += math.Pow(float64(i+1), -skew)
			st.cdf[i] = sum
		}
		for i := range st.cdf {
			st.cdf[i] /= sum
		}
	}
	if c.s("crawlDedup") == "bloom" && st.bloom == nil {
		m := max(64, int(c.f("crawlBitsPerUrl")*c.f("crawlUrls")))
		st.bloom, st.bloomKB = NewBloom(m, int(c.f("crawlHashes"))), float64(m)/8/1024
	}
	return st
}

// hostOf is the site a new URL is on: Zipf over the sites, from the URL number.
func (st *crawlState) hostOf(id uint32) int32 {
	u := float64(Hash32(id*0x9e3779b1+0x1234567)) / 4294967296
	i := sort.SearchFloat64s(st.cdf, u)
	return int32(min(i, len(st.cdf)-1))
}

// accept is the seen-URL check for a link to a page not found before: whether it joins the frontier.
func (st *crawlState) accept(c *node, id uint32) bool {
	switch c.s("crawlDedup") {
	case "exact":
		st.added++
	case "bloom":
		k, m := float64(st.bloom.k), float64(st.bloom.m)
		st.expected += math.Pow(1-math.Exp(-k*float64(st.added)/m), k)
		if st.bloom.Has(id) {
			st.missed++
			return false
		}
		st.bloom.Add(id)
		st.added++
	}
	return true
}

func (st *crawlState) push(p crawlPage) {
	if len(st.q)-st.head >= crawlFrontierCap {
		st.dropped++
		return
	}
	st.q = append(st.q, p)
	if !p.trap && !p.dup {
		st.known = append(st.known, crawlRef{p.id, p.host})
	}
}

// crawlFetch takes the next URL off the frontier (a new starting page when it is empty), reads its links into the
// frontier, and returns the site the fetch goes to.
func (s *Sim) crawlFetch(c *node) int {
	st := s.crawlOf(c)
	trapOn := c.b("crawlTrap")
	trapHost := int32(min(1, len(st.cdf)-1))
	if st.head >= len(st.q) {
		st.q, st.head = st.q[:0], 0
		id := st.next
		st.next++
		p := crawlPage{id: id, host: st.hostOf(id)}
		p.trap = trapOn && p.host == trapHost
		switch c.s("crawlDedup") {
		case "exact":
			st.added++
		case "bloom":
			st.bloom.Add(id)
			st.added++
		}
		st.push(p)
	}
	pg := st.q[st.head]
	st.head++
	if st.head > 4096 && st.head*2 > len(st.q) {
		st.q, st.head = append(st.q[:0], st.q[st.head:]...), 0
	}
	st.fetched++
	if pg.dup {
		st.repeats++
	}
	if pg.trap {
		st.trapDeep = max(st.trapDeep, int(pg.depth))
	}
	if maxD := int32(c.f("maxDepth")); !pg.dup && (maxD == 0 || pg.depth < maxD) {
		st.expand(c, pg, trapOn, trapHost)
	}
	return int(pg.host)
}

func (st *crawlState) expand(c *node, pg crawlPage, trapOn bool, trapHost int32) {
	links := int(c.f("crawlLinks"))
	if pg.trap {
		links = int(c.f("crawlTrapLinks"))
	}
	dupPct := c.f("crawlDupPct")
	dedup := c.s("crawlDedup")
	for i := 0; i < links; i++ {
		h := Hash32(pg.id*2654435761 ^ uint32(i+1)*40503 + 0x51ed27)
		st.found++
		if !pg.trap && len(st.known) > 0 && float64(h%1000) < dupPct*10 {
			ref := st.known[Hash32(h+1)%uint32(len(st.known))]
			if dedup == "none" {
				st.push(crawlPage{id: ref.id, host: ref.host, depth: pg.depth + 1, dup: true})
			} else {
				st.skipped++
			}
			continue
		}
		st.fresh++
		id := st.next
		st.next++
		host := pg.host
		if !pg.trap && Hash32(h+7)%2 != 0 { // half the links stay on the site, the rest go anywhere
			host = st.hostOf(id)
		}
		if !st.accept(c, id) {
			continue
		}
		st.push(crawlPage{id: id, host: host, depth: pg.depth + 1, trap: pg.trap || (trapOn && host == trapHost)})
	}
}

func (st *crawlState) dedupKB(c *node) float64 {
	switch c.s("crawlDedup") {
	case "exact":
		return float64(st.added) * crawlEntryBytes / 1024
	case "bloom":
		return st.bloomKB
	}
	return 0
}

// crawlTotals adds up the crawl clients of the design (nil when there is none).
func (s *Sim) crawlTotals() *CrawlStats {
	var t CrawlStats
	fresh, any := 0, false
	for _, n := range s.order {
		st := s.crawl[n.id]
		if n.removed || st == nil {
			continue
		}
		any = true
		t.Fetched += st.fetched
		t.Discovered += st.found
		t.DuplicatesSkipped += st.skipped
		t.Missed += st.missed
		t.Refetched += st.repeats
		t.TrapDepth = max(t.TrapDepth, st.trapDeep)
		t.Frontier += len(st.q) - st.head
		t.DedupKB += st.dedupKB(n)
		t.ExpectedMissed += st.expected
		t.Dropped += st.dropped
		fresh += st.fresh
	}
	if !any {
		return nil
	}
	t.RecallPct = 100
	if fresh > 0 {
		t.RecallPct = 100 * float64(fresh-t.Missed) / float64(fresh)
	}
	return &t
}

// crawlExtra adds a crawl client's own numbers to its node statistics.
func (s *Sim) crawlExtra(n *node, ex map[string]float64) {
	st := s.crawl[n.id]
	if st == nil {
		return
	}
	ex["crawlFetched"] = float64(st.fetched)
	ex["crawlFrontier"] = float64(len(st.q) - st.head)
	ex["crawlDuplicates"] = float64(st.skipped)
	ex["crawlMissed"] = float64(st.missed)
	ex["crawlTrapDepth"] = float64(st.trapDeep)
	ex["crawlDedupKB"] = st.dedupKB(n)
}
