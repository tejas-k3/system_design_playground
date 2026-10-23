package engine

import "testing"

// A server that pushes to every connected user spends CPU on each push whether or not anyone asked for anything:
// 2,000 connected users, 1.75 pushes a second, 2 ms each, is 7 busy threads out of 8 before a single request arrives.
func pushRun(push float64) (*Sim, RunStats) {
	s := sessionSim(Params{"connections": 2000.0, "msgsPerMin": 6.0},
		Params{"threads": 8.0, "cpuMs": 4.0, "pushPerSec": push, "pushCpuMs": 2.0})
	s.Advance(30000)
	return s, s.RunSnapshot()
}

func TestPushesTakeThreadsFromRequests(t *testing.T) {
	s0, off := pushRun(0)
	s1, on := pushRun(1.75)
	load := s1.Snapshot().Nodes["a"].Extra["pushThreads"]
	t.Logf("no push: p95 %.0f ms; 1.75 push/s to %v users: p95 %.0f ms, %.1f threads spent pushing", off.P95, openSessions(s1), on.P95, load)
	if load < 6.5 || load > 7.5 {
		t.Errorf("2,000 users x 1.75/s x 2 ms = 7 threads, got %.2f", load)
	}
	if on.P95 < off.P95*3 {
		t.Errorf("seven of eight threads gone should slow ordinary requests down: %.0f vs %.0f", on.P95, off.P95)
	}
	if _, ok := s0.Snapshot().Nodes["a"].Extra["pushThreads"]; ok {
		t.Errorf("no push, no stat")
	}
}

func TestPushOffChangesNothing(t *testing.T) {
	_, a := pushRun(0)
	s := sessionSim(Params{"connections": 2000.0, "msgsPerMin": 6.0}, Params{"threads": 8.0, "cpuMs": 4.0})
	s.Advance(30000)
	b := s.RunSnapshot()
	if a.OK != b.OK || a.P95 != b.P95 {
		t.Errorf("push 0 must equal the default: %d/%.1f vs %d/%.1f", a.OK, a.P95, b.OK, b.P95)
	}
}
