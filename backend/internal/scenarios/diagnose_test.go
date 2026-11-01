package scenarios

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the diagnose fixtures from fresh seeded runs")

// The fixtures are not typed in: they are what a seeded run of a real challenge produces. This test re-runs
// each one and fails if the stored file differs, so a hand edit (or a change in the engine) cannot go unnoticed.
// After an intended engine change, regenerate with: go test ./internal/scenarios -run TestDiagnoseFixtures -update
func TestDiagnoseFixturesAreGeneratedFromARealRun(t *testing.T) {
	n := 0
	for _, q := range AllQuestions() {
		if q.Mode != ModeDiagnose {
			continue
		}
		n++
		fresh, err := generateSymptoms(q.source)
		if err != nil {
			t.Fatalf("%s: %v", q.ID, err)
		}
		path := filepath.Join("fixtures", "diagnose-"+q.ID+".json")
		if *update {
			b, _ := json.MarshalIndent(fresh, "", " ")
			if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: no fixture %s (generate it with -update)", q.ID, path)
		}
		var stored Symptoms
		if err := json.Unmarshal(b, &stored); err != nil {
			t.Fatal(err)
		}
		if why := symptomsDiffer(stored, *fresh); why != "" {
			t.Errorf("%s: the stored fixture differs from a fresh seeded run of %s (seed %d): %s; regenerate with -update, never edit it", q.ID, q.source.ScenarioID, q.source.Seed, why)
		}
		if q.Symptoms == nil || !reflect.DeepEqual(*q.Symptoms, stored) {
			t.Errorf("%s: the served symptoms are not the stored fixture", q.ID)
		}
	}
	if n < 3 {
		t.Fatalf("%d diagnose questions, want at least 3", n)
	}
}

func TestDiagnoseQuestionsAreWellFormed(t *testing.T) {
	for _, q := range AllQuestions() {
		if q.Mode != ModeDiagnose {
			continue
		}
		if len(q.Options) < 3 || len(q.Options) > 5 {
			t.Errorf("%s: %d options", q.ID, len(q.Options))
		}
		if q.answer < 0 || q.answer >= len(q.Options) {
			t.Errorf("%s: answer index %d out of range", q.ID, q.answer)
		}
		if q.explanation == "" {
			t.Errorf("%s: needs an explanation", q.ID)
		}
		if q.Symptoms == nil || len(q.Symptoms.Series) < 10 || q.Symptoms.Trace == nil {
			t.Errorf("%s: symptoms need a series and a trace", q.ID)
		}
		seen := map[string]bool{}
		for _, o := range q.Options {
			if seen[o] {
				t.Errorf("%s: duplicate option %q", q.ID, o)
			}
			seen[o] = true
		}
	}
}

func TestGradeChoice(t *testing.T) {
	q := &Question{ID: "t", Mode: ModeDiagnose, Options: []string{"a", "b", "c"}, answer: 1, explanation: "because"}
	r, err := q.GradeChoice(1)
	if err != nil || !r.Correct || r.Answer != 1 || r.Explain != "because" || r.Stars != 3 {
		t.Fatalf("correct choice: %+v %v", r, err)
	}
	r, err = q.GradeChoice(0)
	if err != nil || r.Correct || r.Answer != 1 || r.Stars != 0 {
		t.Fatalf("wrong choice must still reveal the answer: %+v %v", r, err)
	}
	for _, bad := range []int{-1, 3, 99} {
		if _, err := q.GradeChoice(bad); err == nil {
			t.Errorf("choice %d must be refused", bad)
		}
	}
	if _, err := (&Question{Mode: ModeEstimate}).GradeChoice(0); err == nil {
		t.Error("an estimate question is not graded by a choice")
	}
}

// within reports whether two measurements agree within a relative and an absolute allowance.
func close(a, b, rel, abs float64) bool {
	return math.Abs(a-b) <= rel*max(math.Abs(a), math.Abs(b))+abs
}

// symptomsDiffer compares a stored fixture with a fresh run. The engine is deterministic on one machine, but
// floating-point results are not bit-identical across CPU architectures (a fused multiply-add on arm64 and not
// on amd64 changes the last digit, and a last-digit difference can reorder two events a nanosecond apart), so
// the comparison allows a few percent. That still catches any edit that changes what the dashboard shows
// (an outage that moves, a metric that is invented) while tolerating what the architecture does.
func symptomsDiffer(stored, fresh Symptoms) string {
	if stored.DurationSec != fresh.DurationSec || len(stored.Series) != len(fresh.Series) {
		return fmt.Sprintf("shape differs (%v s / %d points vs %v s / %d points)", stored.DurationSec, len(stored.Series), fresh.DurationSec, len(fresh.Series))
	}
	worst, where := 0.0, ""
	for i, a := range stored.Series {
		b := fresh.Series[i]
		for name, pair := range map[string][2]float64{"t": {a.T, b.T}, "okRps": {a.OkRps, b.OkRps}, "errRps": {a.ErrRps, b.ErrRps}, "p50": {a.P50, b.P50}, "p95": {a.P95, b.P95}, "success": {a.Success, b.Success}, "inFlight": {float64(a.InFly), float64(b.InFly)}} {
			rel, abs := 0.05, 5.0
			if name == "t" {
				rel, abs = 0, 1e-9
			}
			if name == "success" {
				rel, abs = 0, 0.05
			}
			if name == "inFlight" {
				rel, abs = 0.10, 10
			}
			if d := math.Abs(pair[0] - pair[1]); !close(pair[0], pair[1], rel, abs) && d > worst {
				worst, where = d, fmt.Sprintf("point %d (t=%.0f) %s: %.3f vs %.3f", i, a.T, name, pair[0], pair[1])
			}
		}
	}
	if where != "" {
		return "worst difference at " + where
	}
	if (stored.Trace == nil) != (fresh.Trace == nil) || (stored.Trace != nil && stored.Trace.Reason != fresh.Trace.Reason) {
		return "the failed trace differs in its outcome"
	}
	for k, a := range stored.Errors {
		if !close(float64(a), float64(fresh.Errors[k]), 0.05, 10) {
			return fmt.Sprintf("error count %s: %d vs %d", k, a, fresh.Errors[k])
		}
	}
	for k := range fresh.Errors {
		if _, ok := stored.Errors[k]; !ok && fresh.Errors[k] > 10 {
			return "a failure reason is missing from the fixture: " + k
		}
	}
	return ""
}

// The explanations quote what the fixture shows. If the engine changes, these fail before a learner is told
// something the dashboard no longer says.
func TestWatchQuestionsQuoteWhatTheirFixturesShow(t *testing.T) {
	down := Q("watch-server-down").Symptoms
	if q, n := down.Errors["queueFull"], down.Errors["nodeDown"]; q < 5*n || n == 0 {
		t.Errorf("server-down: %d 'Queue full' against %d 'Node down'; the explanation says almost all are queue full", q, n)
	}
	var okDuring, arrivals float64
	for _, p := range down.Series {
		if p.T > 24 && p.T < 44 {
			okDuring = max(okDuring, p.OkRps)
		}
		if p.T > 6 && p.T < 18 {
			arrivals = max(arrivals, p.OkRps)
		}
	}
	if okDuring < 150 || okDuring > 230 || arrivals < 300 || arrivals > 400 {
		t.Errorf("server-down: successes during the outage peak at %.0f (say about 190), normal load is %.0f (say about 350)", okDuring, arrivals)
	}
	slow := Q("watch-slow-query").Symptoms
	first, last := slow.Series[0], slow.Series[len(slow.Series)/3]
	if last.P50 < 3*first.P50 || last.InFly < 2*first.InFly {
		t.Errorf("slow-query: p50 %.0f to %.0f ms, in flight %v to %v; the explanation says latency and requests in flight climb sharply", first.P50, last.P50, first.InFly, last.InFly)
	}
	if slow.Trace == nil || slow.Trace.Spans[0].Node != "app" || slow.Trace.Reason != "queueFull" {
		t.Error("slow-query: the failed request should be refused at the app server, which is the point of the question")
	}
}
