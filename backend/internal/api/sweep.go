package api

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"sysdesign/internal/engine"
)

const (
	maxSweepPoints   = 20
	maxSweepDuration = 60.0
	sweepDeadline    = 20 * time.Second
)

type sweepParam struct {
	NodeID string `json:"nodeId"`
	Key    string `json:"key"`
}

type sweepReq struct {
	Diagram     engine.Diagram `json:"diagram"`
	Seed        *uint32        `json:"seed"`
	DurationSec float64        `json:"durationSec"`
	Loads       []float64      `json:"loads"` // multiples of every client's own rate
	Param       *sweepParam    `json:"param"`
	Values      []float64      `json:"values"`
}

type sweepPoint struct {
	Value       float64 `json:"value"` // the load multiple, or the setting's value as used (after clamping)
	Rps         float64 `json:"rps"`   // offered rate from open and bursty clients at their base or peak rate
	SuccessRate float64 `json:"successRate"`
	P50         float64 `json:"p50"`
	P95         float64 `json:"p95"`
	P99         float64 `json:"p99"`
	CostPerHour float64 `json:"costPerHour"`
	Truncated   bool    `json:"truncated,omitempty"`
}

type sweepResp struct {
	Points    []sweepPoint `json:"points"`
	Truncated bool         `json:"truncated,omitempty"`
}

// offered is the request rate the open and bursty clients generate (their peak under a pattern).
func offered(d engine.Diagram) float64 {
	total := 0.0
	for _, n := range d.Nodes {
		if n.Kind != engine.KClient {
			continue
		}
		p := engine.Resolve(n.Kind, n.Params)
		if a, _ := p["arrival"].(string); a == "closed" {
			continue
		}
		r, _ := p["rps"].(float64)
		if pat, _ := p["pattern"].(string); pat != "constant" && pat != "" {
			r, _ = p["peakRps"].(float64)
		}
		total += r
	}
	return total
}

func scaleLoad(d engine.Diagram, m float64) engine.Diagram {
	out := d
	out.Nodes = append([]engine.Node(nil), d.Nodes...)
	for i, n := range out.Nodes {
		if n.Kind != engine.KClient {
			continue
		}
		p := engine.Resolve(n.Kind, n.Params)
		np := engine.Params{}
		for k, v := range n.Params {
			np[k] = v
		}
		for _, k := range []string{"rps", "peakRps", "users"} {
			if v, ok := p[k].(float64); ok {
				np[k] = v * m
			}
		}
		out.Nodes[i].Params = np
	}
	return out
}

func withParam(d engine.Diagram, node, key string, v float64) engine.Diagram {
	out := d
	out.Nodes = append([]engine.Node(nil), d.Nodes...)
	for i, n := range out.Nodes {
		if n.ID != node {
			continue
		}
		np := engine.Params{}
		for k, x := range n.Params {
			np[k] = x
		}
		np[key] = v
		out.Nodes[i].Params = np
	}
	return out
}

// sweep runs the same design at several loads (or one setting at several values) and returns one
// summary per point. It holds a single run slot for all points and stops at a wall-clock deadline.
func (s *Server) sweep(w http.ResponseWriter, r *http.Request) {
	var req sweepReq
	if !decode(w, r, &req) {
		return
	}
	s.metrics.runs.Add(1)
	if tooBig(req.Diagram) {
		writeErr(w, http.StatusRequestEntityTooLarge, "diagram too large")
		return
	}
	d, _ := engine.Sanitize(req.Diagram)
	dur := req.DurationSec
	if dur == 0 {
		dur = 30
	}
	byParam := req.Param != nil
	n := len(req.Loads)
	if byParam {
		n = len(req.Values)
	}
	switch {
	case dur < 0 || dur > maxSweepDuration:
		writeErr(w, 400, fmt.Sprintf("durationSec must be between 1 and %.0f", maxSweepDuration))
		return
	case n == 0 || n > maxSweepPoints:
		writeErr(w, 400, fmt.Sprintf("give between 1 and %d points", maxSweepPoints))
		return
	case byParam && len(req.Loads) > 0, !byParam && len(req.Values) > 0:
		writeErr(w, 400, "sweep either loads or one setting's values, not both")
		return
	}
	var spec *engine.ParamSpec
	if byParam {
		var kind string
		for _, nd := range d.Nodes {
			if nd.ID == req.Param.NodeID {
				kind = nd.Kind
			}
		}
		if kind == "" {
			writeErr(w, 400, "no such component")
			return
		}
		for _, ps := range engine.Spec(kind).Params {
			if ps.Key == req.Param.Key && ps.Type == "number" {
				ps := ps
				spec = &ps
			}
		}
		if spec == nil {
			writeErr(w, 400, "that setting is not a number this component has")
			return
		}
	}
	xs := append([]float64(nil), req.Loads...)
	if byParam {
		xs = append([]float64(nil), req.Values...)
	}
	for _, x := range xs {
		if x < 0 || (!byParam && x == 0) {
			writeErr(w, 400, "loads must be positive and values must not be negative")
			return
		}
	}
	sort.Float64s(xs)

	select {
	case s.runSlots <- struct{}{}:
		defer func() { <-s.runSlots }()
	default:
		s.metrics.runsRefused.Add(1)
		w.Header().Set("Retry-After", "2")
		writeErr(w, http.StatusServiceUnavailable, "too many simulations running, try again shortly")
		return
	}
	seed := uint32(42)
	if req.Seed != nil {
		seed = *req.Seed
	}
	deadline := time.Now().Add(sweepDeadline)
	resp := sweepResp{Points: []sweepPoint{}}
	for _, x := range xs {
		if time.Now().After(deadline) {
			resp.Truncated = true
			break
		}
		var pd engine.Diagram
		used := x
		if byParam {
			clamped := x
			if x < spec.Min {
				clamped = spec.Min
			}
			if x > spec.Max {
				clamped = spec.Max
			}
			used = clamped
			pd = withParam(d, req.Param.NodeID, req.Param.Key, clamped)
		} else {
			pd = scaleLoad(d, x)
		}
		pd, _ = engine.Sanitize(pd)
		opts := engine.Options{Seed: seed, StopTrafficAtMs: dur * 1000}
		sim := engine.New(pd, opts)
		pt := sweepPoint{Value: used, Rps: offered(pd)}
		limit := opts.StopTrafficAtMs + 60000
		for !sim.Finished() && sim.Now() < limit {
			sim.Advance(100)
			if time.Now().After(deadline) {
				pt.Truncated, resp.Truncated = true, true
				break
			}
		}
		run := sim.RunSnapshot()
		if run.Resolved > 0 {
			pt.SuccessRate = float64(run.OK) / float64(run.Resolved)
		}
		pt.P50, pt.P95, pt.P99, pt.CostPerHour = run.P50, run.P95, run.P99, sim.CostPerHour()
		resp.Points = append(resp.Points, pt)
	}
	writeJSON(w, 200, resp)
}
