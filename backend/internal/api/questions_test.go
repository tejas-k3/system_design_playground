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

func TestReviewQuestionsServeTheDiagramAndGradeMarks(t *testing.T) {
	ts := newTestServer(t)
	var q scenarios.Question
	if code := get(t, ts.URL+"/api/questions/review-web-shop", &q); code != 200 || q.Diagram == nil || len(q.Diagram.Nodes) < 5 {
		t.Fatalf("status %d diagram %+v", code, q.Diagram)
	}
	resp, _ := http.Get(ts.URL + "/api/questions/review-web-shop")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, banned := range []string{`"defects"`, `"explain"`, `"rules"`, "retry-amplification"} {
		if strings.Contains(string(body), banned) {
			t.Fatalf("the served question leaks %s: %s", banned, body)
		}
	}
	var res scenarios.ReviewResult
	if code := post(t, ts.URL+"/api/questions/review-web-shop/review", map[string]any{"marked": []string{"users", "app1", "cache", "db"}}, &res); code != 200 || res.Stars != 3 || len(res.Found) != 4 || len(res.FalseMarks) != 0 {
		t.Fatalf("marking the four flawed components: status %d %+v", code, res)
	}
	for _, d := range res.Found {
		if d.Explain == "" {
			t.Errorf("a found defect must come with its explanation: %+v", d)
		}
	}
	if code := post(t, ts.URL+"/api/questions/review-web-shop/review", map[string]any{"marked": []string{"lb", "rep"}}, &res); code != 200 || res.Score != 0 || len(res.Missed) != 4 || len(res.FalseMarks) != 2 {
		t.Fatalf("marking only fine components: status %d %+v", code, res)
	}
	for name, body := range map[string]map[string]any{"unknown id": {"marked": []string{"nope"}}, "no marks field": {}} {
		c := post(t, ts.URL+"/api/questions/review-web-shop/review", body, nil)
		if name == "unknown id" && c != 400 {
			t.Errorf("%s: status %d, want 400", name, c)
		}
		if name == "no marks field" && c != 200 {
			t.Errorf("an empty review is a valid answer (nothing found): %d", c)
		}
	}
	if c := post(t, ts.URL+"/api/questions/url-shortener-numbers/review", map[string]any{"marked": []string{}}, nil); c != 400 {
		t.Errorf("an estimate question cannot be reviewed: %d", c)
	}
	if c := post(t, ts.URL+"/api/questions/nope/review", map[string]any{"marked": []string{}}, nil); c != 404 {
		t.Errorf("unknown question: %d", c)
	}
}

func TestOpenChallengeHidesItsLoadAndHasNoSolutionToServe(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/scenarios/ten-thousand-students")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	for _, banned := range []string{`"traffic"`, `"chaos"`, `"solution"`, "900"} {
		if strings.Contains(string(body), banned) {
			t.Errorf("an open challenge must not reveal %s: %s", banned, body)
		}
	}
	if !strings.Contains(string(body), `"open":true`) {
		t.Errorf("the client needs to know it is open: %s", body)
	}
	if code := get(t, ts.URL+"/api/scenarios/ten-thousand-students/solution", nil); code != 404 {
		t.Errorf("an open challenge has no solution to serve: status %d", code)
	}
	if code := get(t, ts.URL+"/api/scenarios/hello-world/solution", nil); code != 200 {
		t.Errorf("fix-mode solutions are still served: %d", code)
	}
	var list []struct {
		ID   string `json:"id"`
		Open bool   `json:"open"`
	}
	get(t, ts.URL+"/api/scenarios", &list)
	open := 0
	for _, s := range list {
		if s.Open {
			open++
		}
	}
	if open != 3 {
		t.Errorf("the list should flag the three open challenges, flagged %d", open)
	}
	// and it still runs with its hidden profile: a session scores it
	var run runResp
	sc := scenarios.Get("ten-thousand-students")
	if code := post(t, ts.URL+"/api/run", map[string]any{"diagram": sc.Solution, "scenarioId": sc.ID}, &run); code != 200 || run.Score == nil || !run.Score.Pass {
		t.Fatalf("the witness should pass when scored server-side: status %d %+v", code, run.Score)
	}
}

func TestBuildAndTestEndpoint(t *testing.T) {
	ts := newTestServer(t)
	var q scenarios.Question
	get(t, ts.URL+"/api/questions/url-shortener-numbers", &q)
	if q.BuildHint == "" {
		t.Fatal("the question should say it can be built and tested")
	}
	var r scenarios.BuildResult
	body := map[string]any{"answers": map[string]float64{"writeQps": 40, "readQps": 400, "peakReadQps": 200, "storageTB": 3}}
	if code := post(t, ts.URL+"/api/questions/url-shortener-numbers/build", body, &r); code != 200 || r.Verdict != scenarios.BuildUnder || r.Servers != 1 || r.RightServers != 3 {
		t.Fatalf("a 200 rps estimate should build one server and fail the load: status %d %+v", code, r)
	}
	if c := post(t, ts.URL+"/api/questions/url-shortener-numbers/build", map[string]any{"answers": map[string]float64{}}, nil); c != 400 {
		t.Errorf("no sizing number: %d", c)
	}
	if c := post(t, ts.URL+"/api/questions/chat-numbers/build", body, nil); c != 400 {
		t.Errorf("a question with no build step: %d", c)
	}
	if c := post(t, ts.URL+"/api/questions/nope/build", body, nil); c != 404 {
		t.Errorf("unknown question: %d", c)
	}
}

func TestBuildRunsShareTheRunSlots(t *testing.T) {
	srv := New(Config{MaxRuns: 1})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	body := map[string]any{"answers": map[string]float64{"writeQps": 40, "readQps": 400, "peakReadQps": 1200, "storageTB": 3}}
	srv.runSlots <- struct{}{}
	if c := post(t, ts.URL+"/api/questions/url-shortener-numbers/build", body, nil); c != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503 while a run holds the slot", c)
	}
	<-srv.runSlots
	if c := post(t, ts.URL+"/api/questions/url-shortener-numbers/build", body, nil); c != 200 {
		t.Fatalf("status %d after the slot freed", c)
	}
	waitSlotsFree(t, srv, "the slot was not released after a build")
}

func TestAnsweringAMatchQuestionGradesEveryItemAndRevealsTheKeyOnlyAfterwards(t *testing.T) {
	ts := newTestServer(t)
	const path = "/api/questions/what-does-each-box-do"
	var served struct {
		Items []map[string]any `json:"items"`
	}
	if code := get(t, ts.URL+path, &served); code != 200 || len(served.Items) != 12 {
		t.Fatalf("served: status %d, %d items", code, len(served.Items))
	}
	for _, it := range served.Items {
		if _, leaks := it["answer"]; leaks {
			t.Fatal("an item carries its answer before the learner has chosen")
		}
	}
	zeros := make([]int, 12)
	var res scenarios.MatchResult
	if code := post(t, ts.URL+path+"/answer", map[string]any{"choices": zeros}, &res); code != 200 || len(res.Items) != 12 || res.Items[0].Explain == "" {
		t.Fatalf("graded: status %d %+v", code, res)
	}
	right := make([]int, 12)
	for i, it := range res.Items {
		right[i] = it.Answer
	}
	var full scenarios.MatchResult
	if code := post(t, ts.URL+path+"/answer", map[string]any{"choices": right}, &full); code != 200 || full.Correct != 12 || full.Stars != 3 {
		t.Fatalf("all right: status %d %+v", code, full)
	}
	for name, body := range map[string]map[string]any{
		"too few":           {"choices": []int{0, 1}},
		"out of range":      {"choices": []int{0, 0, 0, 9, 0, 0, 0, 0, 0, 0, 0, 0}},
		"none given":        {},
		"a single choice":   {"choice": 1},
		"numbers to a menu": {"answers": map[string]float64{"a": 1}},
	} {
		if c := post(t, ts.URL+path+"/answer", body, nil); c != 400 {
			t.Errorf("%s: status %d, want 400", name, c)
		}
	}
	if c := post(t, ts.URL+"/api/questions/cut-off-threads/answer", map[string]any{"choices": zeros}, nil); c != 400 {
		t.Errorf("a list of choices sent to a diagnosis: %d", c)
	}
}

func TestGuidedBriefServesItsChecklistButNotTheHiddenLoadOrTheAnswer(t *testing.T) {
	ts := newTestServer(t)
	var sc map[string]any
	if code := get(t, ts.URL+"/api/scenarios/first-design-links", &sc); code != 200 {
		t.Fatalf("status %d", code)
	}
	steps, _ := sc["starter"].([]any)
	if len(steps) != 3 {
		t.Fatalf("served %d starter steps, want 3", len(steps))
	}
	for _, hidden := range []string{"traffic", "chaos", "answer", "solution"} {
		if _, leaks := sc[hidden]; leaks {
			t.Errorf("an open brief serves %q", hidden)
		}
	}
	var list []map[string]any
	get(t, ts.URL+"/api/scenarios", &list)
	for _, s := range list {
		if _, ok := s["starter"]; ok {
			t.Error("the checklist belongs to the challenge view, not the first-load list")
		}
	}
}

func TestAnsweringARequirementsPromptGradesThePicksAndRevealsTheKeyAfterwards(t *testing.T) {
	ts := newTestServer(t)
	const path = "/api/questions/ask-photo-feed"
	var served struct {
		Asks  []map[string]any `json:"asks"`
		Picks int              `json:"picks"`
	}
	if code := get(t, ts.URL+path, &served); code != 200 || len(served.Asks) != 8 || served.Picks != 3 {
		t.Fatalf("served: status %d, %d asks, %d picks", code, len(served.Asks), served.Picks)
	}
	for _, a := range served.Asks {
		if len(a) != 1 {
			t.Fatalf("an ask is served with more than its text: %v", a)
		}
	}
	var res scenarios.AskResult
	if code := post(t, ts.URL+path+"/answer", map[string]any{"picks": []int{0, 1}}, &res); code != 200 || res.Wasted != 2 || len(res.Items) != 8 || res.Stars != 0 {
		t.Fatalf("graded: status %d %+v", code, res)
	}
	var best scenarios.AskResult
	if code := post(t, ts.URL+path+"/answer", map[string]any{"picks": []int{2, 5}}, &best); code != 200 || best.Found != 2 || best.Stars != 3 || best.Items[2].Flip == "" {
		t.Fatalf("best picks: status %d %+v", code, best)
	}
	for name, body := range map[string]map[string]any{
		"none":              {},
		"too many":          {"picks": []int{0, 1, 2, 3}},
		"a repeat":          {"picks": []int{2, 2}},
		"out of range":      {"picks": []int{9}},
		"a single choice":   {"choice": 1},
		"numbers to a menu": {"answers": map[string]float64{"a": 1}},
	} {
		if c := post(t, ts.URL+path+"/answer", body, nil); c != 400 {
			t.Errorf("%s: status %d, want 400", name, c)
		}
	}
	if c := post(t, ts.URL+"/api/questions/cut-off-threads/answer", map[string]any{"picks": []int{0}}, nil); c != 400 {
		t.Errorf("picks sent to a diagnosis: %d", c)
	}
}
