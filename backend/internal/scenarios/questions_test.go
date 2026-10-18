package scenarios

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestEstimateToleranceBoundaries(t *testing.T) {
	q := &Question{ID: "t", Mode: ModeEstimate, Fields: []EstimateField{{Key: "qps", Label: "QPS", Expected: 1000}}}
	cases := []struct {
		given float64
		want  string
	}{
		{1000, VerdictGood}, {1300, VerdictGood}, {700, VerdictGood}, // exactly +/-30 %
		{1301, VerdictClose}, {699, VerdictClose}, {2000, VerdictClose}, {500, VerdictClose}, // up to 2x either way
		{2001, VerdictOff}, {499, VerdictOff}, {0, VerdictOff}, {-5, VerdictOff}, {math.NaN(), VerdictOff}, {math.Inf(1), VerdictOff},
	}
	for _, c := range cases {
		r, err := q.GradeEstimate(map[string]float64{"qps": c.given})
		if err != nil {
			t.Fatal(err)
		}
		if got := r.Fields[0].Verdict; got != c.want {
			t.Errorf("answer %v: verdict %s, want %s", c.given, got, c.want)
		}
	}
}

func TestEstimateScoreAndStars(t *testing.T) {
	q := &Question{ID: "t", Mode: ModeEstimate, Fields: []EstimateField{{Key: "a", Expected: 100}, {Key: "b", Expected: 100}, {Key: "c", Expected: 100}, {Key: "d", Expected: 100}}}
	score := func(a, b, c, d float64) (float64, int) {
		r, err := q.GradeEstimate(map[string]float64{"a": a, "b": b, "c": c, "d": d})
		if err != nil {
			t.Fatal(err)
		}
		return r.Score, r.Stars
	}
	if s, st := score(100, 100, 100, 100); s != 1 || st != 3 {
		t.Errorf("all good: score %v stars %d", s, st)
	}
	if s, st := score(100, 100, 100, 180); s != 0.875 || st != 2 { // 3 good + 1 close (half) = 3.5 / 4
		t.Errorf("one close: score %v stars %d", s, st)
	}
	if s, st := score(100, 100, 150, 150); s != 0.75 || st != 2 { // 2 good + 2 close = 3 / 4
		t.Errorf("two close: score %v stars %d", s, st)
	}
	if s, st := score(100, 190, 190, 5000); s != 0.5 || st != 1 {
		t.Errorf("mixed: score %v stars %d", s, st)
	}
	if s, st := score(1, 1, 1, 1); s != 0 || st != 0 {
		t.Errorf("all off: score %v stars %d", s, st)
	}
}

func TestEstimateRejectsMissingAndUnknownFields(t *testing.T) {
	q := &Question{ID: "t", Mode: ModeEstimate, Fields: []EstimateField{{Key: "a", Expected: 1}, {Key: "b", Expected: 1}}}
	if _, err := q.GradeEstimate(map[string]float64{"a": 1}); err == nil {
		t.Error("a missing field must be refused, not graded as zero")
	}
	if _, err := q.GradeEstimate(map[string]float64{"a": 1, "b": 1, "zzz": 1}); err == nil {
		t.Error("an unknown field must be refused")
	}
}

func TestEstimateResultExplainsEveryNumber(t *testing.T) {
	q := Q("url-shortener-numbers")
	if q == nil {
		t.Fatal("missing question")
	}
	r, err := q.GradeEstimate(map[string]float64{"writeQps": 40, "readQps": 400, "peakReadQps": 1200, "storageTB": 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Fields {
		if f.Explain == "" || f.Expected <= 0 {
			t.Errorf("field %s must show the expected value and how it is derived: %+v", f.Key, f)
		}
	}
}

// The expected values are arithmetic on the facts in the prompt; these were worked out by hand.
func TestShippedEstimatesMatchHandCalculation(t *testing.T) {
	want := map[string]map[string]float64{
		// 100 M new URLs a month, 10 reads per write, 3x peak, 500 B a record, kept 5 years
		"url-shortener-numbers": {"writeQps": 100e6 / (30 * 86400), "readQps": 10 * 100e6 / (30 * 86400), "peakReadQps": 3 * 10 * 100e6 / (30 * 86400), "storageTB": 100e6 * 12 * 5 * 500 / 1e12},
		// 10 M daily users, 50 photos viewed each, 200 KB each, 90 % served by the CDN
		"photo-feed-numbers": {"readQps": 10e6 * 50 / 86400, "egressGbps": 10e6 * 50 / 86400 * 200e3 * 8 / 1e9, "originGbps": 0.1 * 10e6 * 50 / 86400 * 200e3 * 8 / 1e9},
		// 50 M daily users send 40 messages of 100 B each; keep 3 years
		"chat-numbers": {"msgPerSec": 50e6 * 40 / 86400, "peakMsgPerSec": 3 * 50e6 * 40 / 86400, "storageTB": 50e6 * 40 * 365 * 3 * 100 / 1e12},
	}
	for id, fields := range want {
		q := Q(id)
		if q == nil {
			t.Fatalf("missing %s", id)
		}
		if len(q.Fields) != len(fields) {
			t.Errorf("%s: %d fields, want %d", id, len(q.Fields), len(fields))
		}
		for _, f := range q.Fields {
			exp, ok := fields[f.Key]
			if !ok {
				t.Errorf("%s: unexpected field %s", id, f.Key)
				continue
			}
			if math.Abs(f.Expected-exp)/exp > 1e-9 {
				t.Errorf("%s.%s = %v, hand calculation says %v", id, f.Key, f.Expected, exp)
			}
		}
	}
}

func TestQuestionJSONNeverCarriesTheAnswerKey(t *testing.T) {
	for _, q := range AllQuestions() {
		b, err := json.Marshal(q)
		if err != nil {
			t.Fatal(err)
		}
		var tree any
		if err := json.Unmarshal(b, &tree); err != nil {
			t.Fatal(err)
		}
		for _, k := range jsonKeys(tree) {
			switch strings.ToLower(k) {
			case "expected", "explain", "answer", "defects", "derivation", "verdict":
				t.Errorf("%s: served JSON has the key %q: %s", q.ID, k, b)
			}
		}
		if q.Prompt == "" || q.Title == "" || q.Summary == "" {
			t.Errorf("%s: needs a title, summary and prompt", q.ID)
		}
		seen := map[string]bool{}
		for _, f := range q.Fields {
			if seen[f.Key] {
				t.Errorf("%s: duplicate field key %s", q.ID, f.Key)
			}
			seen[f.Key] = true
			if f.Explain == "" || f.Label == "" {
				t.Errorf("%s.%s: needs a label and an explanation", q.ID, f.Key)
			}
		}
	}
}

func jsonKeys(v any) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			out = append(out, k)
			out = append(out, jsonKeys(c)...)
		}
	case []any:
		for _, c := range x {
			out = append(out, jsonKeys(c)...)
		}
	}
	return out
}
