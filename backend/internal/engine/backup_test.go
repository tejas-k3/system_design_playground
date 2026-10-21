package engine

import (
	"math"
	"testing"
)

// A database holding 10 GB, backed up every 10 s. At 25 s a bad migration silently corrupts it; at 28 s someone
// notices and restores the last backup taken before the damage (20 s). The data written since is gone (RPO) and the
// database is unavailable while the backup streams back (RTO).
func backupRun(db Params, chaos []ChaosEvent) *Sim {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 60.0, "readPct": 0.0, "keySpace": 1000.0, "timeoutMs": 2000.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0})
	p := Params{"connections": 64.0, "dataGB": 10.0}
	for k, v := range db {
		p[k] = v
	}
	b.node("db", KDatabase, p)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 8, Chaos: chaos})
	s.Advance(100000)
	return s
}

func corruptAndRestore() []ChaosEvent {
	return []ChaosEvent{{Type: "corrupt", NodeID: "db", AtSec: 25}, {Type: "restore", NodeID: "db", AtSec: 28}}
}

func TestRestoreLosesTheDataSinceTheLastGoodBackup(t *testing.T) {
	s := backupRun(Params{"backupEverySec": 10.0, "restoreMBps": 200.0}, corruptAndRestore())
	r := s.RunSnapshot()
	t.Logf("RPO %.1f s, RTO %.1f s, %d writes lost", r.RpoSec, r.RtoSec, r.LostWrites)
	if math.Abs(r.RpoSec-8) > 0.5 {
		t.Errorf("last good backup is at 20 s, restore at 28 s: RPO should be 8 s, got %.1f", r.RpoSec)
	}
	if want := 10.0 * 1024 / 200; math.Abs(r.RtoSec-want) > 0.5 {
		t.Errorf("10 GB at 200 MB/s takes %.1f s, got %.1f", want, r.RtoSec)
	}
	if r.LostWrites < 60*7 || r.LostWrites > 60*9 {
		t.Errorf("about 8 s of 60 writes a second are lost, got %d", r.LostWrites)
	}
}

func TestMoreFrequentBackupsShortenTheRPOAndFasterRestoreShortensTheRTO(t *testing.T) {
	slow := backupRun(Params{"backupEverySec": 20.0, "restoreMBps": 100.0}, corruptAndRestore()).RunSnapshot()
	fast := backupRun(Params{"backupEverySec": 5.0, "restoreMBps": 400.0}, corruptAndRestore()).RunSnapshot()
	t.Logf("RPO %.1f -> %.1f s, RTO %.1f -> %.1f s", slow.RpoSec, fast.RpoSec, slow.RtoSec, fast.RtoSec)
	if fast.RpoSec >= slow.RpoSec || fast.RtoSec >= slow.RtoSec || fast.LostWrites >= slow.LostWrites {
		t.Errorf("frequent backups and a fast restore should both help: %+v vs %+v", fast, slow)
	}
}

func TestNoBackupsMeansLosingEverythingSinceTheStart(t *testing.T) {
	r := backupRun(nil, corruptAndRestore()).RunSnapshot()
	t.Logf("RPO %.1f s, %d writes lost", r.RpoSec, r.LostWrites)
	if math.Abs(r.RpoSec-28) > 0.5 {
		t.Errorf("with no backup the database goes back to its start: RPO 28 s, got %.1f", r.RpoSec)
	}
}

func TestTheDatabaseAnswersNothingWhileItRestores(t *testing.T) {
	s := backupRun(Params{"backupEverySec": 10.0, "restoreMBps": 200.0}, corruptAndRestore())
	var during, after float64
	for _, p := range s.Snapshot().Series {
		if p.T > 30 && p.T < 75 { // restoring from 28 s to about 79 s
			during += p.OkRps
		}
		if p.T > 85 {
			after += p.OkRps
		}
	}
	if during > 1 || after < 100 {
		t.Errorf("no writes should succeed during the restore and they should resume after: %.0f during, %.0f after", during, after)
	}
}

func TestBackupsOffChangeNothing(t *testing.T) {
	a := backupRun(nil, nil).RunSnapshot()
	b := backupRun(Params{"backupEverySec": 0.0}, nil).RunSnapshot()
	if a.OK != b.OK || a.RpoSec != 0 || b.RtoSec != 0 {
		t.Errorf("no backups, no change: %+v vs %+v", a.OK, b.OK)
	}
}
