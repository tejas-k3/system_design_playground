package engine

import "testing"

// The classic bug. A server takes a lock with a short lease, freezes (a garbage-collection pause) for longer than the
// lease, another server takes the lock and books the same seat, and the first wakes up and books it too. Fencing
// tokens fix it only if the lock service issues them and the database refuses a token older than one it has seen.
// fencingTotals adds up five seeds, since a double booking needs a pause to land inside a short window.
func fencingTotals(pauseMs float64, app, db Params, coord bool) (conf, stale int) {
	for seed := uint32(14); seed < 19; seed++ {
		r := fencingRunSeed(seed, pauseMs, app, db, coord).RunSnapshot()
		conf += r.Conflicts
		stale += r.Errors["staleToken"]
	}
	return
}

func fencingRunSeed(seed uint32, pauseMs float64, app, db Params, lockOnCoordinator bool) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 400.0, "readPct": 0.0, "workload": "booking", "keySpace": 6000.0, "keyDist": "uniform", "timeoutMs": 4000.0})
	ap := Params{"threads": 64.0, "queueCap": 2000.0, "lockTtlMs": 150.0, "lockWaitMs": 300.0, "dbPool": 100.0}
	for k, v := range app {
		ap[k] = v
	}
	// two servers, one of which freezes for 600 ms every few seconds
	b.node("lb", KLoadBalancer, nil)
	b.node("a1", KAppServer, mergeParams(ap, Params{"gcPauseMs": pauseMs, "gcEverySec": 0.5}))
	b.node("a2", KAppServer, ap)
	dp := Params{"connections": 64.0, "writeMs": 40.0}
	for k, v := range db {
		dp[k] = v
	}
	b.node("db", KDatabase, dp)
	b.edge("u", "lb").edge("lb", "a1").edge("lb", "a2").edge("a1", "db").edge("a2", "db")
	if lockOnCoordinator {
		b.node("zk", KCoordinator, Params{"nodes": 3.0})
		b.edge("a1", "zk").edge("a2", "zk")
	} else {
		b.node("r", KRedis, nil)
		b.edge("a1", "r").edge("a2", "r")
	}
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: seed})
	s.Advance(60000)
	return s
}

func mergeParams(a, b Params) Params {
	out := Params{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func TestAPausedLockHolderDoubleBooksWithRedisLocks(t *testing.T) {
	c, _ := fencingTotals(600, nil, nil, false)
	t.Logf("redis lock, 600 ms pause, 150 ms lease: %d double bookings over five runs", c)
	if c < 10 {
		t.Errorf("a holder frozen past its lease must double book, got %d", c)
	}
}

// Fencing refuses the write of a holder whose lease was superseded, so the damage falls sharply. It does not remove
// every case: a stale write that reaches the database before the newer holder's is accepted, since no newer token
// has been seen yet.
func TestFencingTokensFromTheCoordinatorCutDoubleBookingsAndRefuseStaleWrites(t *testing.T) {
	app := Params{"lockService": "coordinator"}
	without, _ := fencingTotals(600, app, nil, true)
	with, refused := fencingTotals(600, app, Params{"fencingTokens": true}, true)
	t.Logf("coordinator locks, five runs: %d double bookings without enforcing tokens, %d with; %d stale writes refused", without, with, refused)
	if without < 5 {
		t.Errorf("tokens that nobody checks fix nothing: %d", without)
	}
	if refused == 0 {
		t.Errorf("the paused holder's write should be refused as stale")
	}
	if with*4 > without {
		t.Errorf("enforcing tokens should remove at least three quarters of the double bookings: %d vs %d", with, without)
	}
}

func TestARedisLockCannotBeFencedBecauseItHasNoToken(t *testing.T) {
	plain, _ := fencingTotals(600, nil, nil, false)
	checked, refused := fencingTotals(600, nil, Params{"fencingTokens": true}, false)
	t.Logf("redis lock: %d double bookings, %d with a database that checks tokens (%d refused)", plain, checked, refused)
	if checked != plain || refused != 0 {
		t.Errorf("a Redis lock issues no fencing token, so the database has nothing to check: %d vs %d, %d refused", checked, plain, refused)
	}
}

func TestWithoutThePauseTheLeaseHolds(t *testing.T) {
	if c, _ := fencingTotals(0, nil, nil, false); c != 0 {
		t.Errorf("with no pause the lock does its job: %d double bookings", c)
	}
}

// A conditional write ("book this seat only if it is still free") checks at the moment of writing, atomically, so a
// holder frozen past its lease cannot overwrite a booking made in the meantime, whatever its lock or token says.
func TestAConditionalWriteRemovesEveryDoubleBookingEvenWithARedisLock(t *testing.T) {
	plain, _ := fencingTotals(600, nil, nil, false)
	cond, _ := fencingTotals(600, nil, Params{"conditionalWrite": true}, false)
	t.Logf("redis lock: %d double bookings, %d with conditional writes", plain, cond)
	if plain < 10 || cond != 0 {
		t.Errorf("a compare-and-set at the resource cannot double book: %d without, %d with", plain, cond)
	}
}

func TestConditionalWriteOffChangesNothing(t *testing.T) {
	a, _ := fencingTotals(600, nil, nil, false)
	b, _ := fencingTotals(600, nil, Params{"conditionalWrite": false}, false)
	if a != b {
		t.Errorf("off must equal the default: %d vs %d", a, b)
	}
}
