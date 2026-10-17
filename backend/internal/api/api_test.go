package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"sysdesign/internal/engine"
	"sysdesign/internal/scenarios"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newTestServerWith(t, Config{OriginPatterns: []string{"*"}})
}

func newTestServerWith(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(New(cfg).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func post(t *testing.T, url string, body any, out any) int {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestRegistryAndScenarios(t *testing.T) {
	ts := newTestServer(t)
	var reg struct {
		Nodes []engine.NodeSpec `json:"nodes"`
	}
	resp, _ := http.Get(ts.URL + "/api/registry")
	_ = json.NewDecoder(resp.Body).Decode(&reg)
	if len(reg.Nodes) != 13 {
		t.Fatalf("registry has %d kinds", len(reg.Nodes))
	}
	var list []scenarioSummary
	resp, _ = http.Get(ts.URL + "/api/scenarios")
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if len(list) != 13 {
		t.Fatalf("%d scenarios", len(list))
	}
	resp, _ = http.Get(ts.URL + "/api/scenarios/nope")
	if resp.StatusCode != 404 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	resp, _ = http.Get(ts.URL + "/api/scenarios/hello-world")
	var raw map[string]json.RawMessage
	_ = json.NewDecoder(resp.Body).Decode(&raw)
	if _, leaked := raw["solution"]; leaked {
		t.Fatal("scenario payload leaks the solution")
	}
}

func TestValidateAndRun(t *testing.T) {
	ts := newTestServer(t)
	sc := scenarios.Get("hello-world")
	var v struct {
		Adjusted bool `json:"adjusted"`
	}
	bad := sc.Diagram
	bad.Edges = append(append([]engine.Edge(nil), bad.Edges...), engine.Edge{ID: "x", From: "db", To: "app"})
	if post(t, ts.URL+"/api/validate", map[string]any{"diagram": bad}, &v) != 200 || !v.Adjusted {
		t.Fatal("validate did not flag the illegal edge")
	}
	var run runResp
	if code := post(t, ts.URL+"/api/run", map[string]any{"diagram": sc.Solution, "scenarioId": "hello-world"}, &run); code != 200 {
		t.Fatalf("run status %d", code)
	}
	if run.Score == nil || !run.Score.Pass {
		t.Fatalf("solution should pass: %+v", run.Score)
	}
	var free runResp
	post(t, ts.URL+"/api/run", map[string]any{"diagram": sc.Solution, "durationSec": 10}, &free)
	if free.Score != nil || free.Stats.Global.Run.Resolved == 0 {
		t.Fatalf("free run: %+v", free.Stats.Global.Run)
	}
	if code := post(t, ts.URL+"/api/run", map[string]any{"scenarioId": "nope"}, nil); code != 404 {
		t.Fatalf("status %d", code)
	}
}

func dial(t *testing.T, ts *httptest.Server) (*websocket.Conn, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/api/sim", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c, ctx
}

func sendMsg(t *testing.T, c *websocket.Conn, ctx context.Context, m clientMsg) {
	t.Helper()
	b, _ := json.Marshal(m)
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func readType(t *testing.T, c *websocket.Conn, ctx context.Context, want string) json.RawMessage {
	t.Helper()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %q: %v", want, err)
		}
		var h struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(data, &h)
		if h.Type == want {
			return data
		}
	}
}

func TestWebSocketStepsToScoredResult(t *testing.T) {
	ts := newTestServer(t)
	c, ctx := dial(t, ts)
	sc := scenarios.Get("hello-world")
	readType(t, c, ctx, "frame")
	sendMsg(t, c, ctx, clientMsg{Type: "init", ScenarioID: sc.ID, Diagram: &sc.Solution})
	readType(t, c, ctx, "frame")
	for i := 0; i < 400; i++ {
		sendMsg(t, c, ctx, clientMsg{Type: "step"})
	}
	var res resultMsg
	if err := json.Unmarshal(readType(t, c, ctx, "result"), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Score.Pass || res.ScenarioID != sc.ID {
		t.Fatalf("expected a passing score, got %+v", res.Score)
	}
}

func TestWebSocketLivePlay(t *testing.T) {
	ts := newTestServer(t)
	c, ctx := dial(t, ts)
	sc := scenarios.Get("opening-night")
	readType(t, c, ctx, "frame")
	sendMsg(t, c, ctx, clientMsg{Type: "init", Diagram: &sc.Solution, Speed: 4})
	sendMsg(t, c, ctx, clientMsg{Type: "play"})
	deadline := time.Now().Add(8 * time.Second)
	sawParticles, sawStats := false, false
	for time.Now().Before(deadline) && !(sawParticles && sawStats) {
		var f frameMsg
		_ = json.Unmarshal(readType(t, c, ctx, "frame"), &f)
		sawParticles = sawParticles || len(f.Particles) > 0
		sawStats = sawStats || (f.Stats != nil && f.Stats.Global.Now > 1 && f.Stats.Global.Run.OK > 0)
	}
	if !sawParticles || !sawStats {
		t.Fatalf("particles=%v stats=%v", sawParticles, sawStats)
	}
	sendMsg(t, c, ctx, clientMsg{Type: "pause"})
	sendMsg(t, c, ctx, clientMsg{Type: "reset"})
	for {
		var f frameMsg
		_ = json.Unmarshal(readType(t, c, ctx, "frame"), &f)
		if f.Status == "stopped" && f.Stats != nil && f.Stats.Global.Now == 0 {
			break
		}
	}
}

func TestConfigOnlyRejectsTopologyChanges(t *testing.T) {
	ts := newTestServer(t)
	c, ctx := dial(t, ts)
	sc := scenarios.Get("connection-crunch")
	readType(t, c, ctx, "frame")
	sendMsg(t, c, ctx, clientMsg{Type: "init", ScenarioID: sc.ID, Diagram: &sc.Diagram})
	readType(t, c, ctx, "frame")
	changed := sc.Diagram
	changed.Nodes = append(append([]engine.Node(nil), changed.Nodes...), engine.Node{ID: "extra", Kind: engine.KCache})
	sendMsg(t, c, ctx, clientMsg{Type: "diagram", Diagram: &changed})
	readType(t, c, ctx, "error")
	// parameter edits are fine
	edited := sc.Solution
	sendMsg(t, c, ctx, clientMsg{Type: "diagram", Diagram: &edited})
	sendMsg(t, c, ctx, clientMsg{Type: "speed", Speed: 2})
	var f frameMsg
	_ = json.Unmarshal(readType(t, c, ctx, "frame"), &f)
	if f.Speed != 2 {
		t.Fatalf("speed %v", f.Speed)
	}
}

// Go encodes nil slices as null, which crashes naive clients. Frames must always carry arrays.
func TestFramesNeverCarryNullArrays(t *testing.T) {
	ts := newTestServer(t)
	c, ctx := dial(t, ts)
	sc := scenarios.Get("hello-world")
	sendMsg(t, c, ctx, clientMsg{Type: "init", Diagram: &sc.Solution})
	sendMsg(t, c, ctx, clientMsg{Type: "play"})
	for i := 0; i < 6; i++ {
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(`"series":null`)) || bytes.Contains(data, []byte(`"nodes":null`)) || bytes.Contains(data, []byte(`"edges":null`)) {
			t.Fatalf("frame contains a null collection: %s", data)
		}
	}
}

func TestIdleSessionsAreClosedButRunningOnesAreNot(t *testing.T) {
	ts := newTestServerWith(t, Config{OriginPatterns: []string{"*"}, IdleTimeout: 300 * time.Millisecond})
	sc := scenarios.Get("hello-world")

	idle, ctx := dial(t, ts)
	readType(t, idle, ctx, "frame")
	_, _, err := idle.Read(ctx) // nothing more is sent, so the server must hang up
	if websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Fatalf("idle session should close with going-away, got %v", err)
	}

	busy, ctx2 := dial(t, ts)
	sendMsg(t, busy, ctx2, clientMsg{Type: "init", Diagram: &sc.Solution})
	sendMsg(t, busy, ctx2, clientMsg{Type: "play"})
	deadline := time.Now().Add(900 * time.Millisecond) // three idle timeouts
	for time.Now().Before(deadline) {
		readType(t, busy, ctx2, "frame") // still streaming: a running simulation is not idle
	}
}

func TestSessionCapReturns503AndFreesSlots(t *testing.T) {
	ts := newTestServerWith(t, Config{OriginPatterns: []string{"*"}, MaxSessions: 1})
	first, ctx := dial(t, ts)
	readType(t, first, ctx, "frame")

	resp, err := http.Get(ts.URL + "/api/sim")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("expected 503 with Retry-After while full, got %d", resp.StatusCode)
	}
	if _, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/api/sim", nil); err == nil {
		t.Fatal("second WebSocket should have been refused")
	}

	first.Close(websocket.StatusNormalClosure, "bye")
	deadline := time.Now().Add(3 * time.Second)
	for {
		c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/api/sim", nil)
		if err == nil {
			c.CloseNow()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("slot was never released after the first session closed: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
