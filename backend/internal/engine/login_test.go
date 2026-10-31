package engine

import "testing"

// A logged-in user's session lives in the memory of one app server (local), in a shared store (shared), or nowhere
// (the default). A request that reaches a server without the user's session finds the user signed out.
func loginRun(app Params, lb Params, withRedis bool, chaos []ChaosEvent, skew float64) (*Sim, RunStats) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0, "identities": 200.0, "identitySkew": skew})
	b.node("lb", KLoadBalancer, lb)
	ap := Params{"threads": 32.0, "queueCap": 500.0, "sessionState": "local"}
	for k, v := range app {
		ap[k] = v
	}
	b.node("a1", KAppServer, ap)
	b.node("a2", KAppServer, ap)
	b.node("db", KDatabase, Params{"connections": 32.0, "readMs": 2.0})
	b.edge("u", "lb").edge("lb", "a1").edge("lb", "a2").edge("a1", "db").edge("a2", "db")
	if withRedis {
		b.node("r", KRedis, Params{})
		b.edge("a1", "r").edge("a2", "r")
	}
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 9, Chaos: chaos})
	s.Advance(30000)
	return s, s.RunSnapshot()
}

var crashA1 = []ChaosEvent{{Type: "crash", NodeID: "a1", AtSec: 15, DownSec: 3}}

func TestWithLocalSessionsAndRoundRobinAUserIsSignedOutOnTheOtherServer(t *testing.T) {
	_, rr := loginRun(nil, Params{}, false, nil, 0)
	_, sticky := loginRun(nil, Params{"sticky": true}, false, nil, 0)
	t.Logf("round robin: %d signed out; sticky: %d", rr.Reauth, sticky.Reauth)
	if rr.Reauth < 60 || rr.Reauth > 200 {
		t.Errorf("each of 200 users meets the server without their session about once: %d", rr.Reauth)
	}
	if sticky.Reauth != 0 {
		t.Errorf("a sticky balancer keeps a user on the server that has their session: %d", sticky.Reauth)
	}
}

func TestStickyRoutingDoesNotSaveSessionsFromACrashButASharedStoreDoes(t *testing.T) {
	_, sticky := loginRun(nil, Params{"sticky": true}, false, crashA1, 0)
	_, shared := loginRun(Params{"sessionState": "shared"}, Params{}, true, crashA1, 0)
	t.Logf("sticky local: %d sessions lost, %d signed out; shared: %d lost, %d signed out", sticky.SessionsLost, sticky.Reauth, shared.SessionsLost, shared.Reauth)
	if sticky.SessionsLost < 50 || sticky.Reauth < 50 {
		t.Errorf("the sessions on the server that died are gone and its users must sign in again: %d lost, %d signed out", sticky.SessionsLost, sticky.Reauth)
	}
	if shared.SessionsLost != 0 || shared.Reauth != 0 {
		t.Errorf("the store outlives the server: %d lost, %d signed out", shared.SessionsLost, shared.Reauth)
	}
}

func TestASharedStoreThatLosesItsDataSignsEveryoneOut(t *testing.T) {
	ch := []ChaosEvent{{Type: "crash", NodeID: "r", AtSec: 15, DownSec: 2}}
	_, r := loginRun(Params{"sessionState": "shared"}, Params{}, true, ch, 0)
	if r.SessionsLost < 150 || r.Reauth < 150 {
		t.Errorf("a store without persistence forgets all 200 sessions: %d lost, %d signed out", r.SessionsLost, r.Reauth)
	}
}

func TestSoftStateExpiresWhenUnused(t *testing.T) {
	_, forever := loginRun(Params{"sessionState": "shared"}, Params{}, true, nil, 0)
	_, soft := loginRun(Params{"sessionState": "shared", "sessionTtlSec": 1.0}, Params{}, true, nil, 0)
	t.Logf("kept: %d lost; 1 s of idleness: %d lost, %d signed out", forever.SessionsLost, soft.SessionsLost, soft.Reauth)
	if forever.SessionsLost != 0 || soft.SessionsLost < 50 || soft.Reauth < 50 {
		t.Errorf("a user idle for over a second (a request every 2 s on average) finds the session gone: %d lost", soft.SessionsLost)
	}
}

func TestNoSessionStateMeansNoSessions(t *testing.T) {
	_, r := loginRun(Params{"sessionState": "none"}, Params{}, false, crashA1, 0)
	if r.SessionsLost != 0 || r.Reauth != 0 {
		t.Errorf("stateless servers lose nothing: %d, %d", r.SessionsLost, r.Reauth)
	}
}

func TestStickyRoutingLoadsServersUnevenlyWhenSomeUsersAreBusy(t *testing.T) {
	s, _ := loginRun(nil, Params{"sticky": true}, false, nil, 1.5)
	a1, a2 := s.Snapshot().Nodes["a1"].Arr, s.Snapshot().Nodes["a2"].Arr
	s2, _ := loginRun(nil, Params{}, false, nil, 1.5)
	b1, b2 := s2.Snapshot().Nodes["a1"].Arr, s2.Snapshot().Nodes["a2"].Arr
	t.Logf("sticky %.0f / %.0f; round robin %.0f / %.0f", a1, a2, b1, b2)
	ratio := func(x, y float64) float64 {
		if x < y {
			x, y = y, x
		}
		return x / y
	}
	if ratio(a1, a2) < 1.3 || ratio(b1, b2) > 1.1 {
		t.Errorf("a few busy users pin their servers: sticky %.2f, round robin %.2f", ratio(a1, a2), ratio(b1, b2))
	}
}
