package scenarios

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"sysdesign/internal/engine"
)

// Question modes beyond fixing a broken design. Each question is graded on the server and its answer key is
// never serialised, so nothing the client can fetch reveals it before the learner commits.
const (
	ModeEstimate = "estimate"
	ModeDiagnose = "diagnose"
)

// Verdicts for one estimated number.
const (
	VerdictGood  = "good"  // within 30 % of the expected value
	VerdictClose = "close" // within a factor of two either way
	VerdictOff   = "off"
)

// EstimateField is one number the learner must produce. Label, Unit and Hint are shown with the question;
// Expected and Explain stay on the server and come back with the result.
type EstimateField struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Unit     string  `json:"unit"`
	Expected float64 `json:"-"`
	Explain  string  `json:"-"` // how the expected value follows from the facts
}

type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Question struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Mode       string          `json:"mode"`
	Difficulty string          `json:"difficulty"`
	Summary    string          `json:"summary"`
	Prompt     string          `json:"prompt"`
	Facts      []Fact          `json:"facts,omitempty"`
	Fields     []EstimateField `json:"fields,omitempty"`
	Options    []string        `json:"options,omitempty"`  // diagnose: the candidate explanations
	Symptoms   *Symptoms       `json:"symptoms,omitempty"` // diagnose: what the learner gets to look at

	source      diagSource // where the symptoms come from, so a test can regenerate them
	answer      int        // diagnose: index of the right option
	explanation string     // diagnose: why, shown only after answering
}

// Symptoms are what a monitoring page and one trace would show during an incident. They are generated from a
// seeded run of a real challenge's starting design (see GenerateSymptoms), never typed in.
type Symptoms struct {
	DurationSec float64              `json:"durationSec"`
	Series      []engine.SeriesPoint `json:"series"` // one point every 2 simulated seconds
	Errors      map[string]int       `json:"errors"` // failure counts by reason id
	Trace       *engine.TraceOut     `json:"trace"`  // the first failed request
}

type diagSource struct {
	ScenarioID  string
	Seed        uint32
	DurationSec float64
}

//go:embed fixtures/*.json
var fixtureFS embed.FS

func loadSymptoms(id string) *Symptoms {
	b, err := fixtureFS.ReadFile("fixtures/diagnose-" + id + ".json")
	if err != nil {
		return nil
	}
	var s Symptoms
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}

// GenerateSymptoms runs a challenge's starting design with a seed and records what an operator would see.
func GenerateSymptoms(src diagSource) (*Symptoms, error) {
	sc := Get(src.ScenarioID)
	if sc == nil {
		return nil, fmt.Errorf("no scenario %q", src.ScenarioID)
	}
	sim := engine.New(sc.Diagram, sc.Options(src.Seed))
	sim.EnableTraces(true)
	for sim.Now() < src.DurationSec*1000 && !sim.Finished() {
		sim.Advance(100)
	}
	st := sim.Snapshot()
	out := &Symptoms{DurationSec: src.DurationSec, Errors: st.Global.Run.Errors}
	for i, p := range st.Series {
		if i%4 == 0 {
			out.Series = append(out.Series, p)
		}
	}
	if f := sim.Traces().Failed; len(f) > 0 {
		out.Trace = &f[0]
	}
	if out.Errors == nil {
		out.Errors = map[string]int{}
	}
	return out, nil
}

type ChoiceResult struct {
	Correct bool   `json:"correct"`
	Answer  int    `json:"answer"`
	Explain string `json:"explain"`
	Stars   int    `json:"stars"`
}

// GradeChoice grades a multiple-choice diagnosis. The right answer is revealed with the result either way.
func (q *Question) GradeChoice(choice int) (ChoiceResult, error) {
	if q.Mode != ModeDiagnose {
		return ChoiceResult{}, errors.New("not a multiple-choice question")
	}
	if choice < 0 || choice >= len(q.Options) {
		return ChoiceResult{}, fmt.Errorf("choose one of the %d options", len(q.Options))
	}
	r := ChoiceResult{Correct: choice == q.answer, Answer: q.answer, Explain: q.explanation}
	if r.Correct {
		r.Stars = 3
	}
	return r, nil
}

type FieldResult struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Unit     string  `json:"unit"`
	Given    float64 `json:"given"`
	Expected float64 `json:"expected"`
	Verdict  string  `json:"verdict"`
	Explain  string  `json:"explain"`
}

type EstimateResult struct {
	Fields []FieldResult `json:"fields"`
	Score  float64       `json:"score"` // good = 1, close = 0.5, off = 0, averaged
	Stars  int           `json:"stars"` // 3: everything good; 2: score >= 0.75; 1: score >= 0.5
}

// verdictFor grades one number: good within +/-30 %, close within a factor of two, off beyond or not a
// positive finite number.
func verdictFor(given, expected float64) string {
	if math.IsNaN(given) || math.IsInf(given, 0) || given <= 0 || expected <= 0 {
		return VerdictOff
	}
	switch r := given / expected; {
	case r >= 0.7 && r <= 1.3:
		return VerdictGood
	case r >= 0.5 && r <= 2:
		return VerdictClose
	}
	return VerdictOff
}

// GradeEstimate grades a complete set of answers. Missing and unknown fields are refused rather than
// guessed at, so a typo cannot quietly score as zero.
func (q *Question) GradeEstimate(answers map[string]float64) (EstimateResult, error) {
	if q.Mode != ModeEstimate {
		return EstimateResult{}, errors.New("not an estimate question")
	}
	known := map[string]bool{}
	for _, f := range q.Fields {
		known[f.Key] = true
		if _, ok := answers[f.Key]; !ok {
			return EstimateResult{}, fmt.Errorf("missing an answer for %q", f.Key)
		}
	}
	var extra []string
	for k := range answers {
		if !known[k] {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return EstimateResult{}, fmt.Errorf("unknown field %q", extra[0])
	}
	res := EstimateResult{}
	total, allGood := 0.0, true
	for _, f := range q.Fields {
		v := verdictFor(answers[f.Key], f.Expected)
		switch v {
		case VerdictGood:
			total++
		case VerdictClose:
			total += 0.5
			allGood = false
		default:
			allGood = false
		}
		res.Fields = append(res.Fields, FieldResult{Key: f.Key, Label: f.Label, Unit: f.Unit, Given: answers[f.Key], Expected: f.Expected, Verdict: v, Explain: f.Explain})
	}
	if n := len(q.Fields); n > 0 {
		res.Score = total / float64(n)
	}
	switch {
	case allGood:
		res.Stars = 3
	case res.Score >= 0.75:
		res.Stars = 2
	case res.Score >= 0.5:
		res.Stars = 1
	}
	return res, nil
}

var (
	allQuestions []*Question
	questionByID = map[string]*Question{}
)

func registerQuestion(q *Question) {
	allQuestions = append(allQuestions, q)
	questionByID[q.ID] = q
}

// AllQuestions returns every question in registration order.
func AllQuestions() []*Question { return append([]*Question(nil), allQuestions...) }

// Q returns a question by id, or nil.
func Q(id string) *Question { return questionByID[id] }
