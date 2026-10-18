package scenarios

import (
	"encoding/json"
	"flag"
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
		fresh, err := GenerateSymptoms(q.source)
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
		if !reflect.DeepEqual(stored, *fresh) {
			t.Errorf("%s: the stored fixture differs from a fresh seeded run of %s (seed %d); regenerate with -update, never edit it", q.ID, q.source.ScenarioID, q.source.Seed)
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
