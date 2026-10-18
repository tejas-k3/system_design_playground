package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestDiagnoseQuestionsServeSymptomsNotTheAnswer(t *testing.T) {
	ts := newTestServer(t)
	var q scenarios.Question
	if code := get(t, ts.URL+"/api/questions/cut-off-threads", &q); code != 200 {
		t.Fatalf("status %d", code)
	}
	if q.Mode != scenarios.ModeDiagnose || len(q.Options) < 3 || q.Symptoms == nil || len(q.Symptoms.Series) == 0 {
		t.Fatalf("a diagnose question needs options and symptoms: %+v", q)
	}
	resp, _ := http.Get(ts.URL + "/api/questions/cut-off-threads")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), `"answer"`) || strings.Contains(string(body), `"explain"`) || strings.Contains(string(body), "no call timeout") {
		t.Fatalf("the answer or its explanation leaked before answering: %s", body)
	}
}

func TestAnsweringADiagnosisRevealsTheKeyOnlyAfterwards(t *testing.T) {
	ts := newTestServer(t)
	var wrong, right scenarios.ChoiceResult
	if code := post(t, ts.URL+"/api/questions/cut-off-threads/answer", map[string]any{"choice": 0}, &wrong); code != 200 || wrong.Correct || wrong.Explain == "" || wrong.Answer != 1 {
		t.Fatalf("wrong choice: status %d %+v", code, wrong)
	}
	if code := post(t, ts.URL+"/api/questions/cut-off-threads/answer", map[string]any{"choice": 1}, &right); code != 200 || !right.Correct || right.Stars != 3 {
		t.Fatalf("right choice: status %d %+v", code, right)
	}
	for name, body := range map[string]map[string]any{"out of range": {"choice": 9}, "negative": {"choice": -1}, "none given": {}, "numbers to a choice question": {"answers": map[string]float64{"a": 1}}} {
		if c := post(t, ts.URL+"/api/questions/cut-off-threads/answer", body, nil); c != 400 {
			t.Errorf("%s: status %d, want 400", name, c)
		}
	}
	if c := post(t, ts.URL+"/api/questions/url-shortener-numbers/answer", map[string]any{"choice": 1}, nil); c != 400 {
		t.Errorf("a choice sent to an estimate question: %d", c)
	}
}

func TestRunningATradeoffCandidateReturnsNumbersAndNeverTheKey(t *testing.T) {
	ts := newTestServer(t)
	var q scenarios.Question
	if code := get(t, ts.URL+"/api/questions/payments-replication", &q); code != 200 || len(q.Candidates) != 2 {
		t.Fatalf("status %d candidates %d", code, len(q.Candidates))
	}
	var async, sync scenarios.RunSummary
	if code := post(t, ts.URL+"/api/questions/payments-replication/run", map[string]any{"candidate": 0}, &async); code != 200 || async.Resolved == 0 || async.LostWrites == 0 {
		t.Fatalf("async: status %d %+v", code, async)
	}
	if code := post(t, ts.URL+"/api/questions/payments-replication/run", map[string]any{"candidate": 1}, &sync); code != 200 || sync.LostWrites != 0 || sync.P95 <= async.P95 {
		t.Fatalf("sync: status %d %+v", code, sync)
	}
	for name, body := range map[string]map[string]any{"negative": {"candidate": -1}, "out of range": {"candidate": 9}, "none given": {}} {
		if c := post(t, ts.URL+"/api/questions/payments-replication/run", body, nil); c != 400 {
			t.Errorf("%s: status %d, want 400", name, c)
		}
	}
	if c := post(t, ts.URL+"/api/questions/url-shortener-numbers/run", map[string]any{"candidate": 0}, nil); c != 400 {
		t.Errorf("an estimate question cannot be run: %d", c)
	}
	var res scenarios.ChoiceResult
	if code := post(t, ts.URL+"/api/questions/payments-replication/answer", map[string]any{"choice": 1}, &res); code != 200 || !res.Correct {
		t.Fatalf("answering: status %d %+v", code, res)
	}
}

func TestQuestionRunsShareTheRunSlots(t *testing.T) {
	srv := New(Config{MaxRuns: 1})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	srv.runSlots <- struct{}{}
	if c := post(t, ts.URL+"/api/questions/payments-replication/run", map[string]any{"candidate": 0}, nil); c != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503 while a run holds the only slot", c)
	}
	<-srv.runSlots
	if c := post(t, ts.URL+"/api/questions/payments-replication/run", map[string]any{"candidate": 0}, nil); c != 200 {
		t.Fatalf("status %d after the slot freed", c)
	}
	waitSlotsFree(t, srv, "the slot was not released after a question run")
}
