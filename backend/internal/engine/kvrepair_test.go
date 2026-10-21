package engine

import "testing"

// A node is down for 20 s while keys are written, then comes back holding old data. Reads that reach it are stale
// until something brings it up to date: hints the other nodes kept for it, or read repair, or neither.
func staleAfterReturn(kv Params) (stale, fresh int, s *Sim) {
	chaos := []ChaosEvent{{Type: "kvNodeDown", NodeID: "kv", AtSec: 5, DurationSec: 20, Factor: 1}}
	base := Params{"readQuorum": 1.0, "writeQuorum": 2.0, "writeMs": 30.0}
	for k, v := range kv {
		base[k] = v
	}
	s = quorumRunWith(Params{"readPct": 70.0, "rps": 200.0}, base, chaos) // 30 s run: back up at 25 s
	r := s.RunSnapshot()
	return r.Stale, r.Fresh, s
}

func TestWithoutHintsOrRepairAReturnedNodeServesOldData(t *testing.T) {
	stale, fresh, _ := staleAfterReturn(nil)
	t.Logf("no handoff, no repair: %d of %d fresh reads stale", stale, fresh)
	if stale == 0 {
		t.Errorf("a node that missed writes while down must serve old data when R=1")
	}
}

func TestHintedHandoffBringsTheNodeUpToDate(t *testing.T) {
	plain, _, _ := staleAfterReturn(nil)
	hinted, _, s := staleAfterReturn(Params{"hintedHandoff": true})
	t.Logf("stale reads: %d without hints, %d with them; %.0f hints replayed", plain, hinted, s.Snapshot().Nodes["kv"].Extra["hintsReplayed"])
	if hinted >= plain/2 {
		t.Errorf("replaying the hints should remove most of the staleness: %d vs %d", hinted, plain)
	}
	if s.Snapshot().Nodes["kv"].Extra["hintsReplayed"] == 0 {
		t.Errorf("hints should have been replayed")
	}
}

func TestReadRepairFixesWhatReadsTouch(t *testing.T) {
	plain, _, _ := staleAfterReturn(nil)
	repaired, _, s := staleAfterReturn(Params{"readRepair": true})
	t.Logf("stale reads: %d without repair, %d with it; %.0f repairs", plain, repaired, s.Snapshot().Nodes["kv"].Extra["repairs"])
	if repaired >= plain {
		t.Errorf("repairing on read should reduce staleness: %d vs %d", repaired, plain)
	}
	if s.Snapshot().Nodes["kv"].Extra["repairs"] == 0 {
		t.Errorf("repairs should have happened")
	}
}

// A sloppy quorum lets stand-in nodes take the writes for nodes that are down, so writes keep succeeding.
func TestSloppyQuorumKeepsWritesAvailable(t *testing.T) {
	success := func(sloppy bool) float64 {
		chaos := []ChaosEvent{{Type: "kvNodeDown", NodeID: "kv", AtSec: 5, DurationSec: 30, Factor: 3}}
		s := quorumRunWith(Params{"readPct": 0.0}, Params{"readQuorum": 2.0, "writeQuorum": 2.0, "sloppyQuorum": sloppy}, chaos)
		r := s.RunSnapshot()
		return float64(r.OK) / float64(r.Resolved+r.Pending)
	}
	strict, sloppy := success(false), success(true)
	t.Logf("write success with 3 of 5 nodes down: %.3f strict, %.3f sloppy", strict, sloppy)
	if strict > 0.9 || sloppy < 0.95 {
		t.Errorf("strict loses quorum for some keys, sloppy does not: %.3f vs %.3f", strict, sloppy)
	}
}

func TestDefaultsAreUnchangedByTheNewSwitches(t *testing.T) {
	a, fa, _ := staleAfterReturn(nil)
	b, fb, _ := staleAfterReturn(Params{"hintedHandoff": false, "readRepair": false, "sloppyQuorum": false})
	if a != b || fa != fb {
		t.Errorf("switches off must equal the default: %d/%d vs %d/%d", a, fa, b, fb)
	}
}
