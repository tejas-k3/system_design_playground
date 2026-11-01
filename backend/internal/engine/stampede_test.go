package engine

import "testing"

// Warm start fills the cache at t=0 with the same TTL for every entry, so the whole popular set expires at
// once and the database takes the full miss traffic in one burst. Jitter spreads the expiries; coalescing
// makes concurrent misses for one key share a single fetch.
func stampede(cache Params) (peakPerSec, totalReads float64) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 400.0, "readPct": 100.0, "timeoutMs": 5000.0, "keyDist": "zipf", "keySpace": 1000.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 1000.0})
	p := Params{"capacityItems": 2000.0, "ttlSec": 10.0, "startWarm": true}
	for k, v := range cache {
		p[k] = v
	}
	b.node("c", KCache, p)
	b.node("db", KDatabase, Params{"connections": 16.0, "readMs": 100.0})
	b.edge("u", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 3})
	for sec := 1; sec <= 20; sec++ {
		s.Advance(1000)
		if sec > 8 { // from just before the first expiry
			arr := s.Snapshot().Nodes["db"].Arr
			if arr > peakPerSec {
				peakPerSec = arr
			}
			totalReads += arr
		}
	}
	return
}

func TestTTLJitterFlattensTheExpiryBurst(t *testing.T) {
	basePeak, baseTotal := stampede(nil)
	jPeak, _ := stampede(Params{"ttlJitterPct": 50.0})
	if jPeak > 0.65*basePeak {
		t.Errorf("jitter should cut the burst: peak %.0f vs %.0f reads/s without it", jPeak, basePeak)
	}
	cPeak, cTotal := stampede(Params{"coalesce": true})
	if cTotal >= baseTotal || cPeak > basePeak {
		t.Errorf("coalescing should never add database reads: %.0f (peak %.0f) vs %.0f (peak %.0f)", cTotal, cPeak, baseTotal, basePeak)
	}
	bothPeak, _ := stampede(Params{"ttlJitterPct": 50.0, "coalesce": true})
	if bothPeak > jPeak {
		t.Errorf("both defences should be at least as good as jitter alone: %.0f vs %.0f", bothPeak, jPeak)
	}
}

// One very popular key with a slow database: while the first miss is being answered, every other request for
// the same key piles onto the database too, unless misses are shared.
func TestCoalescingCollapsesMissesOnOneHotKey(t *testing.T) {
	reads := func(coalesce bool) float64 {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 400.0, "readPct": 100.0, "timeoutMs": 5000.0, "keySpace": 1.0})
		b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 1000.0})
		b.node("c", KCache, Params{"capacityItems": 100.0, "ttlSec": 5.0, "startWarm": true, "coalesce": coalesce})
		b.node("db", KDatabase, Params{"connections": 64.0, "readMs": 200.0})
		b.edge("u", "a").edge("a", "c").edge("c", "db")
		d, _ := Sanitize(b.d)
		s := New(d, Options{Seed: 3})
		s.Advance(4900)
		var total float64
		for range 20 { // 100 ms ticks across the first expiry at 5 s
			s.Advance(100)
			total += s.Snapshot().Nodes["db"].Arr * 0.1
		}
		return total
	}
	plain, shared := reads(false), reads(true)
	if plain < 20 || shared > 3 {
		t.Errorf("database reads after the expiry: %.0f without coalescing (want a pile-up of 20+), %.0f with it (want at most a few)", plain, shared)
	}
}
