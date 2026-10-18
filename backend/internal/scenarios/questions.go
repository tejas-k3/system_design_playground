package scenarios

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Question modes beyond fixing a broken design. Each question is graded on the server and its answer key is
// never serialised, so nothing the client can fetch reveals it before the learner commits.
const (
	ModeEstimate = "estimate"
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
