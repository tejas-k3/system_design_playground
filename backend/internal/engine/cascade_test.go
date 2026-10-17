package engine

import "testing"

// successBetween advances the sim to t1 then t2 (seconds) and returns the success rate of requests that
// finished in between, so a recovery after a transient trigger can be told from a failure that persists.
func successBetween(s *Sim, t1, t2 float64) float64 {
	s.Advance(t1*1000 - s.now)
	a := s.RunSnapshot()
	s.Advance(t2*1000 - s.now)
	b := s.RunSnapshot()
	return float64(b.OK-a.OK) / float64(b.Resolved-a.Resolved)
}

// A server at ~75% load is slowed 8x for five seconds, then fully healthy again. Whether the system
// recovers depends on how it queues and retries, the textbook metastable-failure setup: with immediate
// retries and a deep FIFO queue, clients give up and retry while the server keeps working through
// requests nobody is waiting for, so the queue never drains even though the trigger is long gone.
func TestMetastableFailureNeedsRetriesAndADeepQueue(t *testing.T) {
	trigger := []ChaosEvent{{Type: "slow", NodeID: "a0", AtSec: 5, Factor: 8, DurationSec: 5}}
	imm := Params{"rps": 55.0, "retries": 3.0, "backoff": "none", "timeoutMs": 800.0}
	for _, c := range []struct {
		name        string
		client, app Params
		recovers    bool
	}{
		{"immediate retries + deep queue", imm, Params{"queueCap": 500.0}, false},
		{"no retries + deep queue", Params{"rps": 55.0, "retries": 0.0, "timeoutMs": 800.0}, Params{"queueCap": 500.0}, true},
		{"backoff retries + deep queue", Params{"rps": 55.0, "retries": 3.0, "backoff": "exponential", "timeoutMs": 800.0}, Params{"queueCap": 500.0}, true},
		{"immediate retries + short queue", imm, Params{}, true},
		{"immediate retries + deep LIFO queue with a wait limit", imm, Params{"queueCap": 500.0, "queueOrder": "lifo", "queueTimeoutMs": 400.0}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			d, _ := Sanitize(stack(1, c.client, c.app, nil, nil).d)
			s := New(d, Options{Seed: 7, Chaos: trigger})
			if before := successBetween(s, 1, 5); before < 0.99 {
				t.Fatalf("not healthy before the trigger: %.2f", before)
			}
			if later := successBetween(s, 40, 60); c.recovers && later < 0.95 {
				t.Errorf("should recover, success 35 s after the trigger ended was %.2f", later)
			} else if !c.recovers && later > 0.2 {
				t.Errorf("should still be collapsed 35 s after the trigger ended, success was %.2f", later)
			}
		})
	}
}
