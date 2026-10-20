package engine

import "testing"

func sessionSim(client, app Params) *Sim {
	b := &builder{}
	cp := Params{"arrival": "sessions", "connections": 400.0, "sessionSec": 600.0, "msgsPerMin": 6.0, "timeoutMs": 3000.0, "readPct": 100.0}
	for k, v := range client {
		cp[k] = v
	}
	b.node("u", KClient, cp)
	ap := Params{"threads": 64.0, "queueCap": 2000.0, "cpuMs": 2.0}
	for k, v := range app {
		ap[k] = v
	}
	b.node("a", KAppServer, ap)
	b.node("db", KDatabase, Params{"connections": 32.0, "readMs": 2.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return New(d, Options{Seed: 4})
}

func openSessions(s *Sim) float64 {
	return s.Snapshot().Nodes["a"].Extra["sessions"]
}

// Connections are held in memory, independent of threads: when connections x memory per connection exceeds
// what the server has, new connections are refused and the accepted count equals the capacity.
func TestConnectionsAreLimitedByMemoryNotThreads(t *testing.T) {
	// 180 MB less 64 MB base and 16 MB for sixteen busy threads = 100 MB for connections at 100 KB each (1 MB = 1024 KB); 400 users fit, 1500 do not
	app := Params{"memLimitMB": 180.0, "memBaseMB": 64.0, "memPerConnKB": 100.0, "threads": 16.0}
	s := sessionSim(Params{"connections": 400.0}, app)
	s.Advance(20000)
	if got := openSessions(s); got != 400 {
		t.Fatalf("400 users fit in the memory, open sessions %v", got)
	}
	s = sessionSim(Params{"connections": 1500.0}, app)
	s.Advance(20000)
	got := openSessions(s)
	refused := s.Snapshot().Nodes["a"].Extra["sessionCap"]
	t.Logf("1500 users, capacity %v: open %v", refused, got)
	if got != 1024 {
		t.Errorf("accepted connections should equal the capacity (1024), got %v", got)
	}
	if refused != 1024 {
		t.Errorf("the published capacity should be 1024, got %v", refused)
	}
	var refusals float64
	for i := 0; i < 5; i++ { // the rest keep being refused, with a reason of their own
		s.Advance(1000)
		refusals += s.Snapshot().Nodes["a"].Reasons["connRefused"]
	}
	if refusals == 0 {
		t.Errorf("the rest should be refused with the connRefused reason")
	}
}

// When a server restarts, every client it held reconnects. Without jitter they all come at once.
func TestReconnectStormNeedsJitter(t *testing.T) {
	peak := func(jitter float64) float64 {
		s := sessionSim(Params{"connections": 600.0, "reconnectJitterSec": jitter}, nil)
		s.Advance(30000)
		s.CrashNode("a", 1)
		p := 0.0
		for i := 0; i < 40; i++ {
			s.Advance(500)
			if r := s.Snapshot().Nodes["a"].Extra["connectRps"]; r > p {
				p = r
			}
		}
		return p
	}
	storm, spread := peak(0), peak(10)
	t.Logf("peak connection setups/s: %.0f with no jitter, %.0f with 10 s jitter", storm, spread)
	if storm < 5*spread {
		t.Errorf("no jitter should peak >= 5x higher: %.0f vs %.0f", storm, spread)
	}
}

func TestOtherArrivalModesAreUnchangedBySessions(t *testing.T) {
	s := sessionSim(Params{"arrival": "open", "rps": 100.0}, nil)
	s.Advance(5000)
	if _, ok := s.Snapshot().Nodes["a"].Extra["sessions"]; ok {
		t.Errorf("no session stats without sessions")
	}
}
