package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sysdesign/internal/scenarios"
)

func TestLoadSweepOfOneServerShowsTheKnee(t *testing.T) {
	ts := newTestServer(t)
	d := scenarios.Get("opening-night").Diagram // one small server: it has a knee
	var r sweepResp
	code := post(t, ts.URL+"/api/sweep", map[string]any{"diagram": d, "durationSec": 20, "loads": []float64{0.25, 0.5, 1, 2, 4, 8}}, &r)
	if code != 200 || len(r.Points) != 6 {
		t.Fatalf("status %d points %d", code, len(r.Points))
	}
	for i := 1; i < len(r.Points); i++ {
		if r.Points[i].Rps <= r.Points[i-1].Rps {
			t.Fatalf("offered load must increase: %+v", r.Points)
		}
	}
	first, last := r.Points[0], r.Points[len(r.Points)-1]
	if first.SuccessRate < 0.99 {
		t.Fatalf("a quarter of the load should be easy: %+v", first)
	}
	if last.SuccessRate > first.SuccessRate-0.05 && last.P95 < 2*first.P95 {
		t.Fatalf("8x the load must hurt either success or latency: first %+v last %+v", first, last)
	}
	// measured on this design: fine at 80 rps (success 1.000, p95 212 ms), failing at 160 rps (success 0.591, p95 696 ms)
	if r.Points[2].SuccessRate < 0.99 || r.Points[3].SuccessRate > 0.8 {
		t.Fatalf("the knee for one small server should sit between 80 and 160 rps: %+v", r.Points)
	}
	if first.CostPerHour <= 0 {
		t.Fatal("cost missing")
	}
}

func TestSweepRefusesOverLimitRequests(t *testing.T) {
	ts := newTestServer(t)
	d := scenarios.Get("opening-night").Solution
	many := make([]float64, 21)
	for i := range many {
		many[i] = 1
	}
	cases := map[string]map[string]any{
		"21 points":        {"diagram": d, "loads": many},
		"61 s":             {"diagram": d, "durationSec": 61, "loads": []float64{1}},
		"no points":        {"diagram": d},
		"negative load":    {"diagram": d, "loads": []float64{-1}},
		"both kinds":       {"diagram": d, "loads": []float64{1}, "param": map[string]any{"nodeId": "app1", "key": "threads"}, "values": []float64{4}},
		"unknown node":     {"diagram": d, "param": map[string]any{"nodeId": "nope", "key": "threads"}, "values": []float64{4}},
		"unknown setting":  {"diagram": d, "param": map[string]any{"nodeId": "app1", "key": "nope"}, "values": []float64{4}},
		"values with load": {"diagram": d, "values": []float64{4}},
	}
	for name, body := range cases {
		if code := post(t, ts.URL+"/api/sweep", body, nil); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, code)
		}
	}
}

func TestSweepShareOneRunSlotAndReturns503WhenFull(t *testing.T) {
	srv := New(Config{MaxRuns: 1})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	d := scenarios.Get("opening-night").Solution
	srv.runSlots <- struct{}{}
	b, _ := json.Marshal(map[string]any{"diagram": d, "durationSec": 5, "loads": []float64{1}})
	resp, err := http.Post(ts.URL+"/api/sweep", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("status %d", resp.StatusCode)
	}
	<-srv.runSlots
	var r sweepResp
	if code := post(t, ts.URL+"/api/sweep", map[string]any{"diagram": d, "durationSec": 5, "loads": []float64{1, 2, 3}}, &r); code != 200 || len(r.Points) != 3 {
		t.Fatalf("status %d points %d", code, len(r.Points))
	}
	if len(srv.runSlots) != 0 {
		t.Fatal("slot not released: a sweep must hold exactly one slot for all its points")
	}
}

func TestWhatIfSweepClampsToTheSettingsRange(t *testing.T) {
	ts := newTestServer(t)
	d := scenarios.Get("opening-night").Solution
	var id string
	for _, n := range d.Nodes {
		if n.Kind == "appServer" {
			id = n.ID
		}
	}
	var r sweepResp
	code := post(t, ts.URL+"/api/sweep", map[string]any{"diagram": d, "durationSec": 10, "param": map[string]any{"nodeId": id, "key": "threads"}, "values": []float64{0, 4, 16, 500}}, &r)
	if code != 200 || len(r.Points) != 4 {
		t.Fatalf("status %d points %d", code, len(r.Points))
	}
	if r.Points[0].Value != 1 || r.Points[3].Value != 64 || r.Points[1].Value != 4 {
		t.Fatalf("values should be clamped to 1..64 and reported as used: %+v", r.Points)
	}
	if r.Points[3].CostPerHour <= r.Points[1].CostPerHour {
		t.Fatalf("more threads should cost more: %+v", r.Points)
	}
}
