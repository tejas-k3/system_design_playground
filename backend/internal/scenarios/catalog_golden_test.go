package scenarios

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"sysdesign/internal/engine"
)

// The catalog golden pins the scenario catalog: the ids in All() order, a hash of everything a scenario carries
// (including the fields that are json:"-" on Scenario), and the engine outcome of the start and the solution on
// seed 42. Regenerate with UPDATE_GOLDEN=1 only when a catalog change is intended.

type goldenScenario struct {
	Scenario    Scenario
	Solution    engine.Diagram
	Answer      string
	Deeper      string
	Hints       [3]string
	Predictions []Prediction
	Debrief     string
	Goals       engine.Goals
}

func goldenOutcome(d engine.Diagram, sc *Scenario) string {
	r := engine.RunToEnd(d, sc.Options(42), sc.Goals)
	rate := 0.0
	if r.Run.Resolved > 0 {
		rate = float64(r.Run.OK) / float64(r.Run.Resolved)
	}
	return fmt.Sprintf("%v/%d/%.4f/%.1f", r.Pass, r.Stars, rate, r.Run.P95)
}

func catalogGoldenLines(t *testing.T) []string {
	var lines []string
	for _, sc := range All() {
		b, err := json.Marshal(goldenScenario{*sc, sc.Solution, sc.Answer, sc.Deeper, sc.Hints, sc.Predictions, sc.Debrief, sc.Goals})
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		lines = append(lines, fmt.Sprintf("%s %s start=%s solution=%s", sc.ID, hex.EncodeToString(sum[:]),
			goldenOutcome(sc.Diagram, sc), goldenOutcome(sc.Solution, sc)))
	}
	return lines
}

func TestCatalogGolden(t *testing.T) {
	got := strings.Join(catalogGoldenLines(t), "\n") + "\n"
	seen := map[string]bool{}
	for _, sc := range All() {
		if seen[sc.ID] {
			t.Errorf("scenario %s registered twice", sc.ID)
		}
		seen[sc.ID] = true
	}
	const path = "testdata/catalog.golden"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gl) || i < len(wl); i++ {
			var g, w string
			if i < len(gl) {
				g = gl[i]
			}
			if i < len(wl) {
				w = wl[i]
			}
			if g != w {
				t.Errorf("line %d differs:\n got  %s\n want %s", i+1, g, w)
			}
		}
	}
}
