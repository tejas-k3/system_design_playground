package engine

import "testing"

// An iPhone goes on sale: nearly every write is to the same row (the stock counter). With row locks the writes to
// that row run one after another, each holding a database connection while it waits its turn.
func hotRowRun(db, app Params, hotPct float64) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 200.0, "readPct": 0.0, "keySpace": 5000.0, "keyDist": "uniform", "hotKeyPct": hotPct, "timeoutMs": 2000.0, "retries": 0.0})
	ap := Params{"threads": 128.0, "queueCap": 5000.0, "dbPool": 200.0}
	for k, v := range app {
		ap[k] = v
	}
	b.node("a", KAppServer, ap)
	dp := Params{"connections": 64.0, "writeMs": 20.0}
	for k, v := range db {
		dp[k] = v
	}
	b.node("db", KDatabase, dp)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 17})
	s.Advance(30000)
	return s
}

func okRate(s *Sim) float64 {
	r := s.RunSnapshot()
	return float64(r.OK) / float64(r.Resolved+r.Pending)
}

func TestWritesToOneRowAreLimitedByTheRowNotTheDatabase(t *testing.T) {
	free := hotRowRun(nil, nil, 90)
	locked := hotRowRun(Params{"hotRowLock": true}, nil, 90)
	t.Logf("90%% of 200 writes/s on one row, 20 ms each: %.3f ok without row locks, %.3f with them; p95 %.0f vs %.0f ms",
		okRate(free), okRate(locked), free.RunSnapshot().P95, locked.RunSnapshot().P95)
	// one row can take 1 / 0.02 s = 50 writes a second; 180 are asked for
	if okRate(free) < 0.99 {
		t.Errorf("without row locks the database copes: %.3f", okRate(free))
	}
	if okRate(locked) > 0.5 {
		t.Errorf("with row locks the hot row caps the traffic it is asked for: %.3f", okRate(locked))
	}
}

func TestTheRowLockCapsThroughputAtOneOverTheWriteTime(t *testing.T) {
	s := hotRowRun(Params{"hotRowLock": true}, nil, 100)
	// the row does 50 writes a second whether or not anyone is still waiting for the answer
	done := float64(len(s.committed)) / 30
	useful := float64(s.RunSnapshot().OK) / 30
	t.Logf("every write on one row: the row commits %.1f writes a second (theory: 1 / 20 ms = 50), but only %.1f a second reach a client still waiting", done, useful)
	if done < 45 || done > 55 {
		t.Errorf("the row takes one write per 20 ms: %.1f", done)
	}
	if useful > done/4 {
		t.Errorf("most of what the row commits is for clients that already gave up: %.1f of %.1f", useful, done)
	}
}

func TestSplittingTheCounterSpreadsTheRow(t *testing.T) {
	split := hotRowRun(Params{"hotRowLock": true}, Params{"hotKeyMode": "split", "splitShards": 8.0}, 90)
	t.Logf("split over 8 sub-rows: %.3f ok", okRate(split))
	if okRate(split) < 0.99 {
		t.Errorf("8 sub-rows take 22 writes a second each, well under the 50 one row can: %.3f", okRate(split))
	}
}

func TestRowLocksOffChangeNothing(t *testing.T) {
	a := okRate(hotRowRun(nil, nil, 90))
	b := okRate(hotRowRun(Params{"hotRowLock": false}, nil, 90))
	if a != b {
		t.Errorf("off must equal the default: %v vs %v", a, b)
	}
}

// A counter split over 8 rows: each write goes to the row its request's hash picks, so over a long run the busiest
// row takes about 1/8 of the writes, within 1.3 of the mean row. (The hash is the one every request carries.)
func TestASplitCounterSpreadsItsWritesEvenly(t *testing.T) {
	const rows, writes = 8, 20000
	n := &node{p: Params{"hotKeyMode": "split", "splitShards": float64(rows)}}
	count := map[int]int{}
	for i := 0; i < writes; i++ {
		c := splitHotKey(n, &call{key: 0, hash: Hash32(uint32(i))}, false)
		count[c.key]++
	}
	most := 0
	for _, c := range count {
		most = max(most, c)
	}
	ratio := float64(most) / (float64(writes) / rows)
	t.Logf("%d rows used; busiest row over the mean: %.3f", len(count), ratio)
	if len(count) != rows || ratio > 1.3 {
		t.Errorf("writes should spread over all %d rows within 1.3 of the mean: %d rows, %.3f", rows, len(count), ratio)
	}
}
