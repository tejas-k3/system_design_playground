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

type Scenario struct {
	ID          string                           `json:"id"`
	Title       string                           `json:"title"`
	Difficulty  string                           `json:"difficulty"`
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
