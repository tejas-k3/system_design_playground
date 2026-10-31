package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sysdesign/internal/engine"
	"sysdesign/internal/scenarios"
)

func TestDefendRunsAlternativeOnLearnerDesign(t *testing.T) {
	sc := scenarios.Get("gc-storm")
	alt := scenarios.Alternatives(sc.ID)[0]
	var got defendResponse
	code := post(t, newTestServer(t).URL+"/api/scenarios/"+sc.ID+"/defend", map[string]any{"diagram": sc.Diagram, "alternative": 0}, &got)
	if code != http.StatusOK || !got.Applicable {
		t.Fatalf("status %d: %+v", code, got)
	}
	if got.Alternative.Label != alt.Label || got.Alternative.Why != alt.Why || got.WithChange.SuccessPct != alt.SuccessPct || got.WithChange.P95Ms != alt.P95Ms || got.WithChange.CostPerHour != alt.CostPerHour || got.WithChange.Pass != alt.Pass {
		t.Errorf("stored alternative mismatch: got %+v, stored %+v", got, alt)
	}
	if got.WithChange.SuccessPct <= got.Yours.SuccessPct || got.WithChange.P95Ms <= got.Yours.P95Ms {
		t.Errorf("retry alternative should raise success and expose its longer tail: yours %+v, with change %+v", *got.Yours, *got.WithChange)
	}
}

func TestDefendValidationAndNonApplicableCases(t *testing.T) {
	ts := newTestServer(t)
	d := engine.Diagram{Nodes: []engine.Node{{ID: "only", Kind: engine.KClient, Params: engine.Params{}}}}
	var got defendResponse
	if code := post(t, ts.URL+"/api/scenarios/retry-storm/defend", map[string]any{"diagram": d, "alternative": 0}, &got); code != 200 || got.Applicable || got.Reason == "" {
		t.Errorf("missing alternative node: status=%d result=%+v", code, got)
	}
	if code := post(t, ts.URL+"/api/scenarios/ten-thousand-students/defend", map[string]any{"diagram": d, "alternative": 0}, &got); code != 200 || got.Applicable || got.Reason == "" {
		t.Errorf("open challenge: status=%d result=%+v", code, got)
	}
	if code := post(t, ts.URL+"/api/scenarios/retry-storm/defend", map[string]any{"diagram": d, "alternative": -1}, nil); code != 400 {
		t.Errorf("bad index: status %d", code)
	}
	if code := post(t, ts.URL+"/api/scenarios/retry-storm/defend", map[string]any{"diagram": d, "alternative": 100}, nil); code != 400 {
		t.Errorf("out-of-range index: status %d", code)
	}
	if code := post(t, ts.URL+"/api/scenarios/nope/defend", map[string]any{"diagram": d, "alternative": 0}, nil); code != 404 {
		t.Errorf("unknown challenge: status %d", code)
	}
	if code := post(t, ts.URL+"/api/scenarios/retry-storm/defend", "bad body", nil); code != 400 {
		t.Errorf("bad body: status %d", code)
	}
	large := d
	large.Nodes = make([]engine.Node, maxNodes+1)
	if code := post(t, ts.URL+"/api/scenarios/retry-storm/defend", map[string]any{"diagram": large, "alternative": 0}, nil); code != 413 {
		t.Errorf("oversized design: status %d", code)
	}
}

func TestDefendDoesNotLeakInFirstLoadAndReleasesSlot(t *testing.T) {
	s := New(Config{MaxRuns: 1})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	var raw map[string]json.RawMessage
	if code := get(t, ts.URL+"/api/scenarios/retry-storm", &raw); code != 200 {
		t.Fatal(code)
	}
	if strings.Contains(string(raw["story"]), scenarios.Alternatives("retry-storm")[0].Why) || strings.Contains(string(raw["debrief"]), scenarios.Alternatives("retry-storm")[0].Why) {
		t.Fatal("counter-answer leaked on first load")
	}
	sc := scenarios.Get("retry-storm")
	var got defendResponse
	if code := post(t, ts.URL+"/api/scenarios/retry-storm/defend", map[string]any{"diagram": sc.Diagram, "alternative": 0}, &got); code != 200 {
		t.Fatal(code)
	}
	if len(s.runSlots) != 0 {
		t.Fatal("defend did not release the run slot")
	}
}

func TestDefendMatchesEveryStoredAlternative(t *testing.T) {
	for _, sc := range scenarios.All() {
		for i, alt := range scenarios.Alternatives(sc.ID) {
			changed := scenarios.ApplyAlternative(sc.Diagram, alt.Changes)
			got, done := runDefendDesign(changed, sc, 42, time.Now().Add(20*time.Second))
			if !done || got.Pass != alt.Pass || got.SuccessPct != alt.SuccessPct || got.P95Ms != alt.P95Ms || got.CostPerHour != alt.CostPerHour {
				t.Errorf("%s alternative %d %q: got %+v done=%v, stored pass=%v success=%.1f p95=%.0f cost=%.2f", sc.ID, i, alt.Label, got, done, alt.Pass, alt.SuccessPct, alt.P95Ms, alt.CostPerHour)
			}
		}
	}
}
