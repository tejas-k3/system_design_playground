package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// The messaging golden pins the queue, topic and log simulation: for representative designs on seeds 1 to 3 it
// records a hash of the full Snapshot and of its event list. Regenerate with UPDATE_GOLDEN=1 only when a behaviour
// change is intended.

func dedupeRepeatDesign(dedupe bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 0.0, "keySpace": 100000.0, "keyDist": "uniform", "timeoutMs": 3000.0})
	b.node("a", KAppServer, Params{"threads": 64.0, "queueCap": 2000.0, "dbPool": 100.0, "eventPublish": "outbox", "callTimeoutMs": 100.0, "outboxRetryMs": 100.0})
	b.node("db", KDatabase, Params{"connections": 64.0})
	b.node("q", KQueue, Params{"capacity": 100000.0, "dedupeIds": dedupe})
	b.node("w", KWorker, Params{"concurrency": 16.0})
	b.edge("u", "a").edge("a", "db").edge("a", "q").edge("q", "w").edge("w", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestMessagingGolden(t *testing.T) {
	cases := []struct {
		name  string
		d     Diagram
		chaos []ChaosEvent
		sec   float64
	}{
		{"queue-poison-dlq", poisonDesign(3, true, 10), nil, 45},
		{"queue-poison-lost", poisonDesign(5, false, 15), []ChaosEvent{{Type: "crash", NodeID: "w", AtSec: 15, DownSec: 8}}, 45},
		{"topic-two-subscribers", topicDesign([]float64{10, 200}), nil, 40},
		{"log-groups-replay", logDesign(4, "key", []float64{1, 1, 2, 2}), []ChaosEvent{{Type: "replay", NodeID: "l", AtSec: 20, Factor: 10}}, 45},
		{"log-worker-crash", logDesign(3, "key", nil), []ChaosEvent{{Type: "crash", NodeID: "w0", AtSec: 12, DownSec: 10}}, 45},
		{"queue-dedupe-repeat", dedupeRepeatDesign(true), []ChaosEvent{{Type: "linkDown", NodeID: "a", ToID: "q", AtSec: 10, DurationSec: 10, Mode: "replies"}}, 45},
		{"queue-plain-repeat", dedupeRepeatDesign(false), []ChaosEvent{{Type: "linkDown", NodeID: "a", ToID: "q", AtSec: 10, DurationSec: 10, Mode: "replies"}}, 45},
	}
	var lines []string
	for _, c := range cases {
		for seed := uint32(1); seed <= 3; seed++ {
			s := New(c.d, Options{Seed: seed, Chaos: c.chaos})
			s.Advance(c.sec * 1000)
			snap := s.Snapshot()
			sb, err := json.Marshal(snap)
			if err != nil {
				t.Fatal(err)
			}
			eb, err := json.Marshal(snap.Events)
			if err != nil {
				t.Fatal(err)
			}
			rb, err := json.Marshal(s.RunSnapshot())
			if err != nil {
				t.Fatal(err)
			}
			h := func(b []byte) string { x := sha256.Sum256(b); return hex.EncodeToString(x[:]) }
			lines = append(lines, fmt.Sprintf("%s seed=%d snapshot=%s events=%s run=%s", c.name, seed, h(sb), h(eb), h(rb)))
		}
	}
	got := strings.Join(lines, "\n") + "\n"
	const path = "testdata/messaging.golden"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := range gl {
			if i >= len(wl) || gl[i] != wl[i] {
				t.Errorf("line %d differs:\n got  %s", i+1, gl[i])
			}
		}
	}
}
