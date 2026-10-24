// Package scenarios holds the guided challenges: a broken starting design, goals, a reference
// solution, and the teaching copy around them.
package scenarios

import (
	"fmt"
	"sort"

	"sysdesign/internal/engine"
)

type Prediction struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Answer   int      `json:"answer"`
	Explain  string   `json:"explain"`
}

// PublicPrediction is a prediction question as the page first sees it: no answer, no explanation.
type PublicPrediction struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

// PublicScenario is a challenge as the page first loads it. What would give the challenge away is left out and fetched
// only when the player asks: each hint when it is revealed, a prediction's answer when a choice is made, the debrief
// when the result is shown (so a look at the network traffic of a page load shows none of them).
type PublicScenario struct {
	ID          string                           `json:"id"`
	Title       string                           `json:"title"`
	Difficulty  string                           `json:"difficulty"`
	Level       string                           `json:"level"`
	Summary     string                           `json:"summary"`
	Story       string                           `json:"story"`
	Diagram     engine.Diagram                   `json:"diagram"`
	Goals       engine.Goals                     `json:"goals"`
	Traffic     map[string][]engine.TrafficPoint `json:"traffic,omitempty"`
	Chaos       []engine.ChaosEvent              `json:"chaos,omitempty"`
	DurationSec float64                          `json:"durationSec"`
	WarmupSec   float64                          `json:"warmupSec"`
	Speed       float64                          `json:"speed"`
	ConfigOnly  bool                             `json:"configOnly,omitempty"`
	Open        bool                             `json:"open,omitempty"`
	HintCount   int                              `json:"hintCount"`
	Predictions []PublicPrediction               `json:"predictions,omitempty"`
	Concepts    []string                         `json:"concepts,omitempty"`
}

// Public is the first-load view of the challenge.
func (s *Scenario) Public() PublicScenario {
	p := PublicScenario{
		ID: s.ID, Title: s.Title, Difficulty: s.Difficulty, Level: s.Level, Summary: s.Summary, Story: s.Story, Diagram: s.Diagram, Goals: s.Goals,
		Traffic: s.Traffic, Chaos: s.Chaos, DurationSec: s.DurationSec, WarmupSec: s.WarmupSec, Speed: s.Speed,
		ConfigOnly: s.ConfigOnly, Open: s.Open, HintCount: len(s.Hints), Concepts: s.Concepts,
	}
	for i := range s.Predictions {
		q := &s.Predictions[i]
		p.Predictions = append(p.Predictions, PublicPrediction{ID: q.ID, Question: q.Question, Options: s.ShownOptions(q)})
	}
	return p
}

// Hint is the text of the nth hint (1 to 3), "" when there is none.
func (s *Scenario) Hint(n int) string {
	if n < 1 || n > len(s.Hints) {
		return ""
	}
	return s.Hints[n-1]
}

// AnswerHint is the plain statement of what solves the challenge: the step after hint 3, shown only when the player
// asks for it.
func (s *Scenario) AnswerHint() string { return s.Answer }

// Prediction finds a prediction question by id.
func (s *Scenario) Prediction(id string) *Prediction {
	for i := range s.Predictions {
		if s.Predictions[i].ID == id {
			return &s.Predictions[i]
		}
	}
	return nil
}

type Scenario struct {
	ID          string                           `json:"id"`
	Title       string                           `json:"title"`
	Difficulty  string                           `json:"difficulty"`
	Level       string                           `json:"level"` // interview band: sde1, sde2 or sde3 (levels.go)
	Summary     string                           `json:"summary"`
	Story       string                           `json:"story"`
	Diagram     engine.Diagram                   `json:"diagram"`
	Solution    engine.Diagram                   `json:"-"`
	Goals       engine.Goals                     `json:"goals"`
	Traffic     map[string][]engine.TrafficPoint `json:"traffic,omitempty"`
	Chaos       []engine.ChaosEvent              `json:"chaos,omitempty"`
	DurationSec float64                          `json:"durationSec"`
	WarmupSec   float64                          `json:"warmupSec"`
	Speed       float64                          `json:"speed"`
	ConfigOnly  bool                             `json:"configOnly,omitempty"`
	Open        bool                             `json:"open,omitempty"` // build from a blank canvas; the traffic and failures are hidden and there is no reference solution to show
	Hints       [3]string                        `json:"hints"`
	Answer      string                           `json:"-"` // what hint 3 used to say outright; served only by the answer-hint endpoint, behind an extra click
	Predictions []Prediction                     `json:"predictions,omitempty"`
	Debrief     string                           `json:"debrief"`
	Concepts    []string                         `json:"concepts,omitempty"` // ideas it practises, from the fixed Concepts list
}

// Options returns the engine options for a run of this scenario.
func (s *Scenario) Options(seed uint32) engine.Options {
	return engine.Options{
		Seed: seed, Traffic: s.Traffic, Chaos: s.Chaos,
		StopTrafficAtMs: s.DurationSec * 1000, WarmupMs: s.WarmupSec * 1000,
	}
}

var (
	all   []*Scenario
	byID  = map[string]*Scenario{}
	order = map[string]int{"Tutorial": 0, "Easy": 1, "Medium": 2, "Hard": 3}
)

func register(s *Scenario) {
	if s.Speed == 0 {
		s.Speed = 1
	}
	s.Goals = s.Goals.WithStarTargets()
	s.Concepts = conceptTags[s.ID]
	s.Level = levelOf[s.ID]
	all = append(all, s)
	byID[s.ID] = s
}

// All returns scenarios ordered by difficulty then registration order.
func All() []*Scenario {
	out := append([]*Scenario(nil), all...)
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].Difficulty] < order[out[j].Difficulty] })
	return out
}

func Get(id string) *Scenario { return byID[id] }

// ---- tiny diagram builder ------------------------------------------------------------

type bld struct{ d engine.Diagram }

func (b *bld) n(id, kind string, x, y float64, p engine.Params) *bld {
	label := engine.Spec(kind).Name
	b.d.Nodes = append(b.d.Nodes, engine.Node{ID: id, Kind: kind, X: x, Y: y, Label: label, Params: p, Locked: kind == engine.KClient})
	return b
}

// servers adds n app servers (app1..appN) in a column. Each is fed by the "lb" node and wired to
// "db" plus any extra targets, which is the shape most challenges share.
func (b *bld) servers(n int, x, y0, dy float64, p engine.Params, extra ...string) *bld {
	for k := 0; k < n; k++ {
		id := fmt.Sprintf("app%d", k+1)
		b.n(id, engine.KAppServer, x, y0+float64(k)*dy, p).e("lb", id).e(id, "db")
		for _, t := range extra {
			b.e(id, t)
		}
	}
	return b
}

func (b *bld) label(id, l string) *bld {
	for i := range b.d.Nodes {
		if b.d.Nodes[i].ID == id {
			b.d.Nodes[i].Label = l
		}
	}
	return b
}

func (b *bld) e(from, to string) *bld {
	var fk, tk string
	for _, n := range b.d.Nodes {
		if n.ID == from {
			fk = n.Kind
		}
		if n.ID == to {
			tk = n.Kind
		}
	}
	b.d.Edges = append(b.d.Edges, engine.Edge{ID: from + ">" + to, From: from, To: to, LatencyMs: engine.DefaultLatency(fk, tk)})
	return b
}

func (b *bld) done() engine.Diagram {
	d, _ := engine.Sanitize(b.d)
	return d
}

func f(v float64) *float64 { return &v }
func i(v int) *int         { return &v }

var cols = []float64{0, 240, 480, 720, 960, 1200}

const (
	rowTop, rowMid, rowBot = 40.0, 164.0, 288.0
)

type P = engine.Params
