package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	var meta struct {
		Latency struct {
			Rules []engine.LatencyRule `json:"rules"`
		} `json:"latency"`
		Speeds  []float64           `json:"speeds"`
		Reasons []engine.ReasonInfo `json:"reasons"`
	}
	resp, _ = http.Get(ts.URL + "/api/registry")
	_ = json.NewDecoder(resp.Body).Decode(&meta)
	if len(meta.Speeds) != len(Speeds) || len(meta.Reasons) != int(engine.NumReasons) || len(meta.Latency.Rules) == 0 {
		t.Fatalf("registry is missing shared constants: %+v", meta)
	}
	var list []scenarioSummary
	resp, _ = http.Get(ts.URL + "/api/scenarios")
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if len(list) != len(scenarios.All()) || len(list) == 0 {
		t.Fatalf("%d scenarios listed, %d registered", len(list), len(scenarios.All()))
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

func TestWebSocketReturnsRequestTraces(t *testing.T) {
	ts := newTestServer(t)
	c, ctx := dial(t, ts)
	sc := scenarios.Get("opening-night")
	readType(t, c, ctx, "frame")

	// before any run there is nothing to show, but the answer must still be well-formed arrays
	sendMsg(t, c, ctx, clientMsg{Type: "traces"})
	var empty tracesMsg
	if err := json.Unmarshal(readType(t, c, ctx, "traces"), &empty); err != nil || empty.OK == nil || empty.Failed == nil {
		t.Fatalf("empty traces must be [] not null: %+v err=%v", empty, err)
	}

	sendMsg(t, c, ctx, clientMsg{Type: "init", Diagram: &sc.Solution})
	for i := 0; i < 100; i++ {
		sendMsg(t, c, ctx, clientMsg{Type: "step"})
	}
	sendMsg(t, c, ctx, clientMsg{Type: "traces"})
	var got tracesMsg
	if err := json.Unmarshal(readType(t, c, ctx, "traces"), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.OK) == 0 || len(got.OK[0].Spans) < 3 {
		t.Fatalf("expected successful traces with several spans, got %+v", got.OK)
	}
}

func TestNodeSeriesAnswersWithOneSecondPointsAndNeverNull(t *testing.T) {
	ts := newTestServer(t)
	c, ctx := dial(t, ts)
	sc := scenarios.Get("opening-night")
	readType(t, c, ctx, "frame")

	sendMsg(t, c, ctx, clientMsg{Type: "nodeSeries", NodeID: "app1"})
	var empty nodeSeriesMsg
	if err := json.Unmarshal(readType(t, c, ctx, "nodeSeries"), &empty); err != nil || empty.Points == nil || empty.NodeID != "app1" {
		t.Fatalf("before any run the answer is an empty list, not null, naming the node: %+v err=%v", empty, err)
	}

	sendMsg(t, c, ctx, clientMsg{Type: "init", Diagram: &sc.Solution})
	for i := 0; i < 50; i++ { // 5 simulated seconds
		sendMsg(t, c, ctx, clientMsg{Type: "step"})
	}
	var id string
	for _, n := range sc.Solution.Nodes {
		if n.Kind == engine.KAppServer {
			id = n.ID
		}
	}
	sendMsg(t, c, ctx, clientMsg{Type: "nodeSeries", NodeID: id})
	var got nodeSeriesMsg
	if err := json.Unmarshal(readType(t, c, ctx, "nodeSeries"), &got); err != nil {
		t.Fatal(err)
	}
	if got.NodeID != id || len(got.Points) < 4 || len(got.Points) > 5 {
		t.Fatalf("5 simulated seconds should give 4-5 points for %s, got %d (%+v)", id, len(got.Points), got.NodeID)
	}
}

func TestSharedDesignsRoundTripAreSanitizedAndGuarded(t *testing.T) {
	ts := newTestServer(t)
	sc := scenarios.Get("opening-night")

	var created struct {
		ID string `json:"id"`
	}
	dirty := sc.Solution
	dirty.Nodes = append(append([]engine.Node(nil), dirty.Nodes...), engine.Node{ID: "evil", Kind: "nope"}) // must not survive
	if code := post(t, ts.URL+"/api/designs", map[string]any{"diagram": dirty}, &created); code != http.StatusCreated || len(created.ID) != 13 {
		t.Fatalf("save: status %d id %q", code, created.ID)
	}
	resp, err := http.Get(ts.URL + "/api/designs/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Diagram engine.Diagram `json:"diagram"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if resp.StatusCode != 200 || len(got.Diagram.Nodes) != len(sc.Solution.Nodes) {
		t.Fatalf("load: status %d, %d nodes (want %d, the invalid node dropped)", resp.StatusCode, len(got.Diagram.Nodes), len(sc.Solution.Nodes))
	}

	for path, want := range map[string]int{"/api/designs/aaaaaaaaaaaaa": 404, "/api/designs/..%2f..%2fetc": 404, "/api/designs/short": 404} {
		r, _ := http.Get(ts.URL + path)
		r.Body.Close()
		if r.StatusCode != want {
			t.Errorf("GET %s = %d, want %d", path, r.StatusCode, want)
		}
	}
	if code := post(t, ts.URL+"/api/designs", map[string]any{"diagram": engine.Diagram{}}, nil); code != http.StatusBadRequest {
		t.Fatalf("an empty design should be refused, got %d", code)
	}
	huge := engine.Diagram{}
	for i := 0; i < 130; i++ {
		huge.Nodes = append(huge.Nodes, engine.Node{ID: fmt.Sprint("n", i), Kind: engine.KCache})
	}
	if code := post(t, ts.URL+"/api/designs", map[string]any{"diagram": huge}, nil); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("an oversized design should be refused, got %d", code)
	}
}

func TestSavingDesignsIsRateLimitedPerClient(t *testing.T) {
	ts := newTestServerWith(t, Config{OriginPatterns: []string{"*"}, SaveLimit: 3})
	d := scenarios.Get("hello-world").Solution
	for i := 0; i < 3; i++ {
		if code := post(t, ts.URL+"/api/designs", map[string]any{"diagram": d}, nil); code != http.StatusCreated {
			t.Fatalf("save %d: %d", i, code)
		}
	}
	b, _ := json.Marshal(map[string]any{"diagram": d})
	resp, err := http.Post(ts.URL+"/api/designs", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("expected 429 with Retry-After, got %d", resp.StatusCode)
	}
}

func TestLimiterWindowResets(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLimiter(2, time.Minute)
	l.now = func() time.Time { return now }
	for i := 1; i <= 2; i++ {
		if !l.allow("a") {
			t.Fatalf("request %d of 2 should be allowed", i)
		}
	}
	if l.allow("a") {
		t.Fatal("the third request in the window should be blocked")
	}
	if !l.allow("b") {
		t.Fatal("a different client has its own allowance")
	}
	now = now.Add(61 * time.Second)
	if !l.allow("a") {
		t.Fatal("the allowance should return after the window")
	}
}

func TestSecurityHeadersAndStaticCaching(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "assets", "app-abc123.js"), []byte("console.log(1)"), 0o644)
	ts := newTestServerWith(t, Config{OriginPatterns: []string{"*"}, StaticDir: dir})

	get := func(path string) *http.Response {
		r, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r
	}
	for _, path := range []string{"/", "/assets/app-abc123.js", "/api/healthz", "/some/client/route"} {
		r := get(path)
		csp := r.Header.Get("Content-Security-Policy")
		if !strings.Contains(csp, "script-src 'self';") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") || !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("%s: weak or missing CSP %q", path, csp)
		}
		if r.Header.Get("X-Content-Type-Options") != "nosniff" || r.Header.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: missing hardening headers", path)
		}
	}
	if cc := get("/assets/app-abc123.js").Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("hashed assets should be cached forever, got %q", cc)
	}
	for _, path := range []string{"/", "/some/client/route"} {
		if cc := get(path).Header.Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s must be revalidated so deploys are picked up, got %q", path, cc)
		}
	}
	if got := get("/some/client/route"); got.StatusCode != 200 {
		t.Errorf("unknown routes fall back to the app, got %d", got.StatusCode)
	}
}

func TestMetricsReflectActivity(t *testing.T) {
	ts := newTestServerWith(t, Config{OriginPatterns: []string{"*"}, MaxSessions: 1, SaveLimit: 1})
	read := func() map[string]int {
		resp, err := http.Get(ts.URL + "/api/metrics")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
			t.Fatalf("metrics content type %q", ct)
		}
		out := map[string]int{}
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			var name string
			var v int
			if n, _ := fmt.Sscanf(sc.Text(), "%s %d", &name, &v); n == 2 && !strings.HasPrefix(name, "#") {
				out[name] = v
			}
		}
		return out
	}
	if m := read(); m["sdp_sessions_active"] != 0 || m["sdp_designs_stored"] != 0 {
		t.Fatalf("a fresh server should be idle: %v", m)
	}

	c, ctx := dial(t, ts)
	readType(t, c, ctx, "frame")
	if r, err := http.Get(ts.URL + "/api/sim"); err == nil { // refused: the only slot is taken
		r.Body.Close()
	}
	d := scenarios.Get("hello-world").Solution
	post(t, ts.URL+"/api/designs", map[string]any{"diagram": d}, nil)
	post(t, ts.URL+"/api/designs", map[string]any{"diagram": d}, nil) // over the limit of 1
	post(t, ts.URL+"/api/run", map[string]any{"diagram": d, "durationSec": 2}, nil)

	m := read()
	want := map[string]int{"sdp_sessions_active": 1, "sdp_sessions_total": 1, "sdp_sessions_refused_total": 1, "sdp_designs_saved_total": 1, "sdp_designs_rate_limited_total": 1, "sdp_designs_stored": 1, "sdp_runs_total": 1}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %d, want %d (all: %v)", k, m[k], v, m)
		}
	}
	c.Close(websocket.StatusNormalClosure, "bye")
	deadline := time.Now().Add(3 * time.Second)
	for read()["sdp_sessions_active"] != 0 {
		if time.Now().After(deadline) {
			t.Fatal("active sessions gauge never returned to zero")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunCapReturns503WhileAllSlotsAreBusy(t *testing.T) {
	srv := New(Config{MaxRuns: 1})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	body := map[string]any{"scenarioId": "hello-world"}

	srv.runSlots <- struct{}{} // another headless run is in progress
	b, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+"/api/run", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("expected 503 with Retry-After while the slot is taken, got %d", resp.StatusCode)
	}
	if got := srv.metrics.runsRefused.Load(); got != 1 {
		t.Fatalf("runs refused = %d, want 1", got)
	}

	<-srv.runSlots // the other run finished
	sc := scenarios.Get("hello-world")
	if code := post(t, ts.URL+"/api/run", map[string]any{"diagram": sc.Solution, "scenarioId": "hello-world"}, nil); code != 200 {
		t.Fatalf("run after the slot freed up: %d", code)
	}
	if len(srv.runSlots) != 0 {
		t.Fatal("the slot was not released after the run")
	}
}

func TestEventLogIsSentOnlyWhenItChanges(t *testing.T) {
	ts := newTestServer(t)
	c, ctx := dial(t, ts)
	sc := scenarios.Get("primary-dies")
	readType(t, c, ctx, "frame")
	sendMsg(t, c, ctx, clientMsg{Type: "init", Diagram: &sc.Solution, ScenarioID: sc.ID, Speed: 8})
	sendMsg(t, c, ctx, clientMsg{Type: "play"})
	deadline := time.Now().Add(15 * time.Second)
	withEvents, quietAfter := 0, 0
	for time.Now().Before(deadline) && quietAfter < 3 {
		var f frameMsg
		_ = json.Unmarshal(readType(t, c, ctx, "frame"), &f)
		if f.Stats == nil {
			continue
		}
		if len(f.Stats.Events) > 0 {
			withEvents++
		} else if withEvents > 0 {
			quietAfter++
		}
	}
	if withEvents == 0 || quietAfter < 3 {
		t.Fatalf("events should appear when logged and be left out of later unchanged frames: with=%d quiet=%d", withEvents, quietAfter)
	}
}

func TestValidateReturnsLintFindingsAndNeverNull(t *testing.T) {
	ts := newTestServer(t)
	var v struct {
		Findings []engine.Finding `json:"findings"`
	}
	if post(t, ts.URL+"/api/validate", map[string]any{"diagram": scenarios.Get("connection-crunch").Diagram}, &v) != 200 {
		t.Fatal("validate failed")
	}
	found := false
	for _, f := range v.Findings {
		found = found || f.Rule == "pool-exceeds-db"
	}
	if !found {
		t.Fatalf("connection-crunch should be flagged for its pools: %+v", v.Findings)
	}
	body, _ := json.Marshal(map[string]any{"diagram": engine.Diagram{}})
	resp, err := http.Post(ts.URL+"/api/validate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if bytes.Contains(raw, []byte(`"findings":null`)) {
		t.Fatalf("an empty design must give [], not null: %s", raw)
	}
}
