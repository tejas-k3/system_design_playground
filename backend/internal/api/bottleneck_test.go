package api

import (
	"reflect"
	"strings"
	"testing"

	"sysdesign/internal/scenarios"
)

func TestFirstToFailIsTheComponentThatSaturatesAtTheLightestLoad(t *testing.T) {
	cases := []struct {
		name  string
		utils []map[string]float64
		calls map[string][]string
		at    int
		who   []string
	}{
		{"database first", []map[string]float64{{"db": .5, "app": .4}, {"db": .8, "app": .6}, {"db": .96, "app": .7}}, nil, 2, []string{"db"}},
		{"already saturated at today's load", []map[string]float64{{"db": 1, "app": .9}, {"db": 1, "app": 1}}, nil, 0, []string{"db"}},
		{"servers saturate together", []map[string]float64{{"app1": 1, "app2": .97, "db": .5}}, nil, 0, []string{"app1", "app2"}},
		{"just under the line is not named", []map[string]float64{{"app": .96, "db": .94}}, nil, 0, []string{"app"}},
		{"nothing saturates", []map[string]float64{{"db": .5}, {"db": .9}}, nil, -1, nil},
		{"no data", nil, nil, -1, nil},
		{"a server waiting on a saturated database is not the culprit", []map[string]float64{{"app": 1, "db": 1}}, map[string][]string{"app": {"db"}}, 0, []string{"db"}},
		{"the waiting is followed through a cache to the database", []map[string]float64{{"app": 1, "cache": 1, "db": 1}}, map[string][]string{"app": {"cache", "db"}, "cache": {"db"}}, 0, []string{"db"}},
		{"a saturated server over a healthy database is the culprit", []map[string]float64{{"app": 1, "db": .4}}, map[string][]string{"app": {"db"}}, 0, []string{"app"}},
		{"a loop in the connections does not hang it", []map[string]float64{{"a": 1, "b": 1}}, map[string][]string{"a": {"b"}, "b": {"a"}}, 0, []string{"a", "b"}},
	}
	for _, c := range cases {
		at, who := firstToFail(c.utils, c.calls)
		if at != c.at || !reflect.DeepEqual(who, c.who) {
			t.Errorf("%s: at %d, %v; want %d, %v", c.name, at, who, c.at, c.who)
		}
	}
}

func TestBottleneckGradesAGuessAgainstARealLoadSweep(t *testing.T) {
	ts := newTestServer(t)
	slow := scenarios.Get("slow-query").Diagram
	var wrong, right bottleneckResp
	if code := post(t, ts.URL+"/api/bottleneck", map[string]any{"diagram": slow, "guess": "app"}, &wrong); code != 200 || wrong.Correct || len(wrong.Points) != 5 {
		t.Fatalf("a wrong guess: status %d %+v", code, wrong)
	}
	if !strings.Contains(wrong.Explain, "at today's traffic") || strings.Contains(wrong.Explain, "1 times") {
		t.Errorf("a part that is already full today should say so plainly: %q", wrong.Explain)
	}
	if wrong.Points[0].Hottest != "db" {
		t.Errorf("the busiest part is the one doing the work, not the server waiting on it: %q", wrong.Points[0].Hottest)
	}
	if !reflect.DeepEqual(wrong.Culprits, []string{"db"}) || wrong.At < 0 || wrong.Explain == "" {
		t.Fatalf("the answer should name the database and where it saturates: %+v", wrong)
	}
	if code := post(t, ts.URL+"/api/bottleneck", map[string]any{"diagram": slow, "guess": "db"}, &right); code != 200 || !right.Correct {
		t.Fatalf("the right guess: status %d %+v", code, right)
	}
	for i, p := range right.Points {
		if i > 0 && p.Load <= right.Points[i-1].Load {
			t.Errorf("loads must rise: %+v", right.Points)
		}
		if p.Rps <= 0 || p.Hottest == "" {
			t.Errorf("point %d is incomplete: %+v", i, p)
		}
	}
}

func TestBottleneckAcceptsNothingAsAnAnswerWhenNothingSaturates(t *testing.T) {
	ts := newTestServer(t)
	roomy := scenarios.Get("hello-world").Solution
	var res bottleneckResp
	if code := post(t, ts.URL+"/api/bottleneck", map[string]any{"diagram": roomy, "guess": ""}, &res); code != 200 {
		t.Fatalf("status %d", code)
	}
	if res.At >= 0 {
		t.Skipf("the roomy design saturates at %d: %v; pick another design for this test", res.At, res.Culprits)
	}
	if !res.Correct || len(res.Culprits) != 0 {
		t.Errorf("nothing saturating is the answer, and guessing nothing is right: %+v", res)
	}
}

func TestBottleneckRefusesWhatItCannotGrade(t *testing.T) {
	ts := newTestServer(t)
	d := scenarios.Get("slow-query").Diagram
	for name, body := range map[string]map[string]any{
		"no guess":        {"diagram": d},
		"an unknown part": {"diagram": d, "guess": "nope"},
		"the users":       {"diagram": d, "guess": "users"},
		"no components":   {"diagram": map[string]any{"nodes": []any{}, "edges": []any{}, "notes": []any{}}, "guess": ""},
	} {
		if c := post(t, ts.URL+"/api/bottleneck", body, nil); c != 400 {
			t.Errorf("%s: status %d, want 400", name, c)
		}
	}
}

func TestTheCulpritsAreNamedAsOneReadableSubject(t *testing.T) {
	for _, c := range []struct {
		labels        []string
		subject, verb string
	}{
		{[]string{"Database"}, "Database", "reaches"},
		{[]string{"App Server", "App Server", "App Server"}, "App Server ×3", "reach"},
		{[]string{"App Server", "Database"}, "App Server and Database", "reach"},
		{[]string{"App Server", "App Server", "Database"}, "App Server ×2 and Database", "reach"},
	} {
		if s, v := subjectOf(c.labels); s != c.subject || v != c.verb {
			t.Errorf("%v: %q %q; want %q %q", c.labels, s, v, c.subject, c.verb)
		}
	}
}
