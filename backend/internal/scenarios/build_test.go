package scenarios

import "testing"

func TestBuildAndTestShowsUnderAndOverProvisioning(t *testing.T) {
	q := Q("url-shortener-numbers")
	if q == nil || q.build == nil {
		t.Fatal("the URL shortener question should offer to build and test the estimate")
	}
	right := map[string]float64{"writeQps": 39, "readQps": 386, "peakReadQps": 1157, "storageTB": 3}
	cases := []struct {
		name    string
		peak    float64
		servers int
		verdict string
	}{
		{"spot on", 1157, 3, BuildRight},
		{"a tenth of the load", 120, 1, BuildUnder},
		{"half", 580, 2, BuildUnder},
		{"ten times too much", 11570, 29, BuildOver},
	}
	for _, c := range cases {
		a := map[string]float64{}
		for k, v := range right {
			a[k] = v
		}
		a["peakReadQps"] = c.peak
		r, err := q.BuildAndTest(a, 42)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r.Servers != c.servers || r.Verdict != c.verdict {
			t.Errorf("%s: %d servers, verdict %s; want %d, %s (yours %+v)", c.name, r.Servers, r.Verdict, c.servers, c.verdict, r.Yours)
		}
		if r.RightServers != 3 || r.RightSized.Success < 0.99 {
			t.Errorf("%s: the right-sized comparison should be 3 servers meeting the load: %+v", c.name, r)
		}
	}
}

func TestBuildAndTestRefusesWhatItCannotSize(t *testing.T) {
	q := Q("url-shortener-numbers")
	if _, err := q.BuildAndTest(map[string]float64{"writeQps": 1}, 1); err == nil {
		t.Error("a missing sizing number must be refused")
	}
	if _, err := q.BuildAndTest(map[string]float64{"writeQps": 1, "readQps": 1, "peakReadQps": 0, "storageTB": 1}, 1); err == nil {
		t.Error("a zero estimate cannot size a fleet")
	}
	if _, err := Q("chat-numbers").BuildAndTest(map[string]float64{}, 1); err == nil {
		t.Error("a question with no build step must say so")
	}
}

func TestBuildStepIsNotServedAsAnswerKey(t *testing.T) {
	q := Q("url-shortener-numbers")
	if q.BuildHint == "" {
		t.Error("the dialog needs a sentence saying how the numbers are turned into servers")
	}
}
