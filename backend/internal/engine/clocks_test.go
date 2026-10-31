package engine

import "testing"

// Last-writer-wins compares the time each leader stamped on its write. A leader whose clock is ahead stamps its writes
// with a later time, so a write made afterwards at another leader can lose to an older one it never conflicted with.
func clockRun(db Params) RunStats {
	p := Params{"topology": "multiLeader", "leaders": 2.0, "syncLagMs": 100.0, "conflictPolicy": "lastWriterWins"}
	for k, v := range db {
		p[k] = v
	}
	return multiLeaderRun(p).RunSnapshot()
}

func TestAClockThatIsAheadMakesAnOlderWriteBeatANewerOne(t *testing.T) {
	none := clockRun(nil)
	skew := clockRun(Params{"clockSpreadMs": 3000.0})
	t.Logf("no offset: %d newer writes lost to the clock; 3 s offset: %d", none.ClockLost, skew.ClockLost)
	if none.ClockLost != 0 {
		t.Errorf("clocks that agree order writes by when they happened: %d", none.ClockLost)
	}
	// 400 writes/s over 200 keys: a key is written every 0.5 s; a write at the leader that is 3 s behind loses to the
	// previous write of its key when that one was at the other leader, which is half the time
	if skew.ClockLost < 3000 {
		t.Errorf("a 3 s offset between leaders drops many newer writes: %d", skew.ClockLost)
	}
}

func TestClocksDriftApartUntilTheyAreSynchronised(t *testing.T) {
	drift := clockRun(Params{"clockDriftMsPerMin": 3000.0})
	synced := clockRun(Params{"clockDriftMsPerMin": 3000.0, "clockSyncSec": 5.0})
	t.Logf("drifting 3 s a minute: %d lost; synchronised every 5 s: %d", drift.ClockLost, synced.ClockLost)
	if drift.ClockLost <= synced.ClockLost*3 || synced.ClockLost == 0 {
		t.Errorf("synchronising keeps the offset below 250 ms, so far fewer writes are lost, but not none: %d against %d", synced.ClockLost, drift.ClockLost)
	}
}

func TestLogicalVersionsOrderWritesWithoutTheClock(t *testing.T) {
	lww := clockRun(Params{"clockSpreadMs": 3000.0})
	logical := clockRun(Params{"clockSpreadMs": 3000.0, "conflictPolicy": "logicalVersion"})
	t.Logf("clock stamps: %d lost to the clock; logical versions: %d (and %d true conflicts)", lww.ClockLost, logical.ClockLost, logical.LostUpdates)
	if logical.ClockLost != 0 {
		t.Errorf("a write that saw the previous one is ordered after it whatever the clocks say: %d", logical.ClockLost)
	}
	if logical.LostUpdates == 0 {
		t.Errorf("writes that really were concurrent still conflict")
	}
}

func TestMergingKeepsEveryWriteWhateverTheClocksSay(t *testing.T) {
	if r := clockRun(Params{"clockSpreadMs": 3000.0, "conflictPolicy": "merge"}); r.ClockLost != 0 {
		t.Errorf("nothing is dropped: %d", r.ClockLost)
	}
}
