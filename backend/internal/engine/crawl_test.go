package engine

import (
	"math"
	"reflect"
	"testing"
)

// The web-crawl workload: every request fetches a URL from the frontier; a fetched page links to more pages, some of
// them already known; a trap host links to endless new pages; a dedup structure decides which links join the frontier.

func crawlDesign(client Params, hosts int, app Params) Diagram {
	b := &builder{}
	c := Params{"rps": 200.0, "readPct": 100.0, "workload": "crawl", "timeoutMs": 5000.0, "retries": 0.0}
	for k, v := range client {
		c[k] = v
	}
	b.node("u", KClient, c)
	ap := Params{"threads": 64.0, "queueCap": 1000.0}
	for k, v := range app {
		ap[k] = v
	}
	b.node("a", KAppServer, ap)
	b.edge("u", "a")
	if hosts == 0 {
		b.node("db", KDatabase, Params{"connections": 64.0}).edge("a", "db")
	}
	for i := 0; i < hosts; i++ {
		h := string(rune('p' + i))
		b.node(h, KExternalAPI, Params{"latencyMs": 100.0, "errorPct": 0.0, "calledFor": "all", "politeRps": 8.0})
		b.edge("a", h)
	}
	d, _ := Sanitize(b.d)
	return d
}

func crawlStats(t *testing.T, s *Sim) *CrawlStats {
	t.Helper()
	c := s.RunSnapshot().Crawl
	if c == nil {
		t.Fatal("the crawl workload reports no crawl statistics")
	}
	return c
}

// (a) With a share d of the discovered links pointing at known pages and exact dedup, a share 1-d of the links is new.
func TestCrawlDuplicatesAreSkippedAtTheDuplicateShare(t *testing.T) {
	const d = 0.4
	cs := crawlStats(t, run(crawlDesign(Params{"crawlLinks": 5.0, "crawlDupPct": d * 100, "crawlDedup": "exact"}, 0, nil), Options{Seed: 3}, 60))
	share := float64(cs.Discovered-cs.DuplicatesSkipped) / float64(cs.Discovered)
	t.Logf("exact dedup: %d links found, %d duplicates skipped, new share %.4f (expected %.2f), %d pages fetched, %d refetched, exact set %.0f KB", cs.Discovered, cs.DuplicatesSkipped, share, 1-d, cs.Fetched, cs.Refetched, cs.DedupKB)
	if math.Abs(share-(1-d)) > 0.02 {
		t.Errorf("fetched share of discovered links %.4f, want %.2f +-0.02", share, 1-d)
	}
	if cs.Refetched != 0 {
		t.Errorf("exact dedup never fetches a page twice: %d", cs.Refetched)
	}
	// without dedup the duplicates are fetched again
	cn := crawlStats(t, run(crawlDesign(Params{"crawlLinks": 5.0, "crawlDupPct": d * 100, "crawlDedup": "none"}, 0, nil), Options{Seed: 3}, 60))
	wasted := float64(cn.Refetched) / float64(cn.Fetched)
	t.Logf("no dedup: %d fetched, %d of them repeats (%.3f of fetches; the links are %.2f repeats)", cn.Fetched, cn.Refetched, wasted, d)
	if wasted < 0.3 || wasted > 0.5 {
		t.Errorf("without dedup about %.0f%% of the fetches repeat known pages: %.3f", d*100, wasted)
	}
}

// (b) A Bloom filter as the seen-set skips new pages at its false-positive rate (1 - e^(-kn/m))^k.
func TestCrawlBloomDedupMissesPagesAtTheFalsePositiveRate(t *testing.T) {
	p := Params{"rps": 500.0, "crawlLinks": 2.0, "crawlDupPct": 50.0, "crawlDedup": "bloom", "crawlUrls": 100000.0, "crawlBitsPerUrl": 6.0, "crawlHashes": 3.0}
	cs := crawlStats(t, run(crawlDesign(p, 0, nil), Options{Seed: 5}, 200))
	newLinks := cs.Discovered - cs.DuplicatesSkipped
	got := float64(cs.Missed) / float64(newLinks)
	want := cs.ExpectedMissed / float64(newLinks)
	t.Logf("bloom dedup: %d new links checked, %d skipped by mistake: %.4f measured, %.4f by (1-e^(-kn/m))^k summed over the run; %.0f KB fixed; recall %.3f", newLinks, cs.Missed, got, want, cs.DedupKB, cs.RecallPct/100)
	if cs.Missed < 1000 {
		t.Fatalf("too few false positives to measure: %d", cs.Missed)
	}
	if math.Abs(got-want)/want > 0.15 {
		t.Errorf("measured miss rate %.4f is more than 15%% from the formula %.4f", got, want)
	}
	// the same pages with an exact set: nothing missed, memory grows with the pages
	p["crawlDedup"] = "exact"
	ce := crawlStats(t, run(crawlDesign(p, 0, nil), Options{Seed: 5}, 200))
	t.Logf("exact set: %d missed, %.0f KB", ce.Missed, ce.DedupKB)
	if ce.Missed != 0 {
		t.Errorf("an exact set misses nothing: %d", ce.Missed)
	}
}

// (c) A trap host keeps the frontier growing; maxDepth stops it.
func TestCrawlTrapGrowsTheFrontierUnlessDepthIsCapped(t *testing.T) {
	trap := func(maxDepth float64) (a, b CrawlStats) {
		d := crawlDesign(Params{"rps": 100.0, "crawlLinks": 2.0, "crawlDupPct": 60.0, "crawlDedup": "exact", "crawlHosts": 10.0, "crawlTrap": true, "maxDepth": maxDepth}, 0, nil)
		s := New(d, Options{Seed: 11})
		s.Advance(60000)
		a = *crawlStats(t, s)
		s.Advance(60000)
		return a, *crawlStats(t, s)
	}
	a1, a2 := trap(0)
	t.Logf("trap, no depth limit: frontier %d at 60 s, %d at 120 s; trap depth %d", a1.Frontier, a2.Frontier, a2.TrapDepth)
	if a1.Frontier < 1000 || float64(a2.Frontier) < 1.5*float64(a1.Frontier) {
		t.Errorf("the frontier should keep growing: %d then %d", a1.Frontier, a2.Frontier)
	}
	b1, b2 := trap(6)
	t.Logf("trap, maxDepth 6: frontier %d at 60 s, %d at 120 s; trap depth %d", b1.Frontier, b2.Frontier, b2.TrapDepth)
	if b2.TrapDepth > 6 || b2.TrapDepth == 0 {
		t.Errorf("trap depth must reach the cap and stop there: %d", b2.TrapDepth)
	}
	if b2.Frontier > 200 || b2.Frontier*10 > a2.Frontier {
		t.Errorf("a depth cap keeps the frontier small: %d against %d", b2.Frontier, a2.Frontier)
	}
	if a2.TrapDepth <= 6 {
		t.Errorf("without a cap the trap goes deeper than 6: %d", a2.TrapDepth)
	}
}

// (d) Politeness holds with the crawl workload: never more than perHostRps calls a second to one site.
func TestCrawlKeepsPerHostPoliteness(t *testing.T) {
	measure := func(limit float64) (peak float64, refused int) {
		app := Params{"hostChoice": "byKey"}
		if limit > 0 {
			app["perHostRps"] = limit
		}
		d := crawlDesign(Params{"rps": 12.0, "arrival": "bursty", "burstFactor": 4.0, "burstSec": 2.0, "crawlHosts": 3.0, "crawlLinks": 3.0, "crawlDupPct": 30.0}, 3, app)
		s := New(d, Options{Seed: 9})
		for sec := 0; sec < 120; sec++ {
			s.Advance(1000)
			st := s.Snapshot()
			for _, h := range []string{"p", "q", "r"} {
				peak = math.Max(peak, st.Nodes[h].Arr)
			}
		}
		return peak, s.RunSnapshot().Errors["rateLimited"]
	}
	freePeak, freeRefused := measure(0)
	politePeak, politeRefused := measure(6)
	t.Logf("no limit: busiest site gets up to %.1f calls/s, %d refused; perHostRps 6: up to %.1f calls/s, %d refused", freePeak, freeRefused, politePeak, politeRefused)
	if freePeak <= 6 || freeRefused == 0 {
		t.Errorf("without a limit the big site is pushed past 6 calls a second: %.1f, %d refused", freePeak, freeRefused)
	}
	if politePeak > 6 || politeRefused != 0 {
		t.Errorf("never more than 6 calls a second to one site: %.1f, %d refused", politePeak, politeRefused)
	}
}

func TestCrawlOffChangesNothing(t *testing.T) {
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 80.0, "readPct": 90.0, "timeoutMs": 3000.0}).node("a", KAppServer, Params{"threads": 16.0}).node("db", KDatabase, nil).edge("u", "a").edge("a", "db")
	d0, _ := Sanitize(plain.d)
	mention := &builder{}
	mention.node("u", KClient, Params{"rps": 80.0, "readPct": 90.0, "timeoutMs": 3000.0, "workload": "kv", "crawlHosts": 7.0, "crawlLinks": 9.0, "crawlTrap": true, "maxDepth": 3.0, "crawlDedup": "bloom"}).node("a", KAppServer, Params{"threads": 16.0}).node("db", KDatabase, nil).edge("u", "a").edge("a", "db")
	d1, _ := Sanitize(mention.d)
	a := run(d0, Options{Seed: 21}, 30).Snapshot()
	b := run(d1, Options{Seed: 21}, 30).Snapshot()
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("crawl settings with another workload must change nothing:\n%+v\n%+v", a.Global.Run, b.Global.Run)
	}
	if a.Global.Run.Crawl != nil {
		t.Fatal("no crawl statistics without the crawl workload")
	}
	x := run(d0, Options{Seed: 21}, 30).Snapshot()
	if !reflect.DeepEqual(a, x) {
		t.Fatal("same seed, same result")
	}
}
