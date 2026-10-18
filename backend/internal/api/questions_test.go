package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"sysdesign/internal/scenarios"
)

func TestQuestionsAreListedAndServedWithoutTheirAnswerKey(t *testing.T) {
	ts := newTestServer(t)
	for _, path := range []string{"/api/questions", "/api/questions/url-shortener-numbers"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s: status %d", path, resp.StatusCode)
		}
		for _, banned := range []string{`"expected"`, `"explain"`, `"verdict"`} {
			if strings.Contains(string(body), banned) {
				t.Errorf("%s leaks %s before an answer is submitted: %s", path, banned, body)
			}
		}
	}
	var list []struct {
		ID, Mode string
	}
	if code := get(t, ts.URL+"/api/questions", &list); code != 200 || len(list) != len(scenarios.AllQuestions()) {
		t.Fatalf("list: status %d, %d items", code, len(list))
	}
	if code := get(t, ts.URL+"/api/questions/nope", nil); code != 404 {
		t.Fatalf("unknown question: %d", code)
	}
}

func TestAnsweringAnEstimateReturnsTheGradeAndTheWorkings(t *testing.T) {
	ts := newTestServer(t)
	var r scenarios.EstimateResult
	code := post(t, ts.URL+"/api/questions/url-shortener-numbers/answer", map[string]any{"answers": map[string]float64{"writeQps": 40, "readQps": 400, "peakReadQps": 1200, "storageTB": 3}}, &r)
	if code != 200 || len(r.Fields) != 4 || r.Stars != 3 {
		t.Fatalf("status %d result %+v", code, r)
	}
	for _, f := range r.Fields {
		if f.Explain == "" || f.Expected == 0 {
			t.Errorf("the result should show the expected value and the workings: %+v", f)
		}
	}
	bad := map[string]map[string]any{
		"missing field": {"answers": map[string]float64{"writeQps": 40}},
		"unknown field": {"answers": map[string]float64{"writeQps": 1, "readQps": 1, "peakReadQps": 1, "storageTB": 1, "x": 1}},
		"no answers":    {},
	}
	for name, body := range bad {
		if c := post(t, ts.URL+"/api/questions/url-shortener-numbers/answer", body, nil); c != 400 {
			t.Errorf("%s: status %d, want 400", name, c)
		}
	}
	if c := post(t, ts.URL+"/api/questions/nope/answer", map[string]any{"answers": map[string]float64{}}, nil); c != 404 {
		t.Errorf("unknown question: %d", c)
	}
}

func get(t *testing.T, url string, out any) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}
