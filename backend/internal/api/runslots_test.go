package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sysdesign/internal/scenarios"
)

// Every route that runs a simulation goes through the same run-slot admission: with the only slot
// taken it answers 503 + Retry-After 2 + the same body and counts one refusal; the slot is handed
// back after a success and after an error return.
func TestEveryRunRouteSharesTheSameSlotRefusal(t *testing.T) {
	d := scenarios.Get("opening-night").Solution
	build := map[string]any{"answers": map[string]float64{"writeQps": 40, "readQps": 400, "peakReadQps": 1200, "storageTB": 3}}
	cases := []struct {
		name     string
		path     string
		ok       any
		failure  any // a request that fails after the slot is taken (nil: the route has none)
		failWant int
	}{
		{"run", "/api/run", map[string]any{"scenarioId": "hello-world"}, nil, 0},
		{"sweep", "/api/sweep", map[string]any{"diagram": d, "durationSec": 5, "loads": []float64{1}}, nil, 0},
		{"question run", "/api/questions/payments-replication/run", map[string]any{"candidate": 0}, map[string]any{"candidate": 99}, 400},
		{"question build", "/api/questions/url-shortener-numbers/build", build, map[string]any{"answers": map[string]float64{}}, 400},
	}
	const wantBody = `{"error":"too many simulations running, try again shortly"}`
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := New(Config{MaxRuns: 1})
			ts := httptest.NewServer(srv.Handler())
			t.Cleanup(ts.Close)
			send := func(body any) (int, http.Header, string) {
				b, _ := json.Marshal(body)
				resp, err := http.Post(ts.URL+c.path, "application/json", bytes.NewReader(b))
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				out, _ := io.ReadAll(resp.Body)
				return resp.StatusCode, resp.Header, strings.TrimSpace(string(out))
			}

			srv.runSlots <- struct{}{}
			code, h, body := send(c.ok)
			if code != http.StatusServiceUnavailable || h.Get("Retry-After") != "2" || body != wantBody {
				t.Fatalf("busy: %d Retry-After=%q body=%s", code, h.Get("Retry-After"), body)
			}
			if got := srv.metrics.runsRefused.Load(); got != 1 {
				t.Fatalf("runs refused = %d, want 1", got)
			}
			if got := srv.metrics.runs.Load(); got != 1 {
				t.Fatalf("runs counted = %d, want 1 (the refused request still counts as a run request)", got)
			}
			<-srv.runSlots

			if code, _, body := send(c.ok); code != 200 {
				t.Fatalf("after the slot freed: %d %s", code, body)
			}
			waitSlotsFree(t, srv, "slot not released after a success")
			if c.failure != nil {
				if code, _, body := send(c.failure); code != c.failWant {
					t.Fatalf("error case: %d %s, want %d", code, body, c.failWant)
				}
				waitSlotsFree(t, srv, "slot not released after an error return")
				if code, _, body := send(c.ok); code != 200 {
					t.Fatalf("after an error return: %d %s", code, body)
				}
			}
			if got := srv.metrics.runsRefused.Load(); got != 1 {
				t.Fatalf("runs refused = %d after the free requests, want still 1", got)
			}
		})
	}
}
