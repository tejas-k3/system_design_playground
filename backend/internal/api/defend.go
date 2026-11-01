package api

import (
	"math"
	"net/http"
	"time"

	"sysdesign/internal/engine"
	"sysdesign/internal/scenarios"
)

type defendRequest struct {
	Diagram     engine.Diagram `json:"diagram"`
	Alternative int            `json:"alternative"`
	Seed        *uint32        `json:"seed"`
}

type defendScore struct {
	Pass        bool    `json:"pass"`
	SuccessPct  float64 `json:"successPct"`
	P95Ms       float64 `json:"p95Ms"`
	CostPerHour float64 `json:"costPerHour"`
}

type defendResponse struct {
	Applicable  bool               `json:"applicable"`
	Reason      string             `json:"reason,omitempty"`
	Alternative *defendAlternative `json:"alternative,omitempty"`
	Yours       *defendScore       `json:"yours,omitempty"`
	WithChange  *defendScore       `json:"withChange,omitempty"`
}

type defendAlternative struct {
	Label string `json:"label"`
	Why   string `json:"why"`
}

func (s *Server) defend(w http.ResponseWriter, r *http.Request) {
	sc := scenarios.Get(r.PathValue("id"))
	if sc == nil {
		writeErr(w, http.StatusNotFound, "no such scenario")
		return
	}
	var req defendRequest
	if !decode(w, r, &req) {
		return
	}
	s.metrics.runs.Add(1)
	if tooBig(req.Diagram) {
		writeErr(w, http.StatusRequestEntityTooLarge, "diagram too large")
		return
	}
	alts := scenarios.Alternatives(sc.ID)
	if len(alts) == 0 {
		writeJSON(w, http.StatusOK, defendResponse{Reason: "There is nothing to compare with for this challenge."})
		return
	}
	if req.Alternative < 0 || req.Alternative >= len(alts) {
		writeErr(w, http.StatusBadRequest, "choose a stored alternative")
		return
	}
	d, _ := engine.Sanitize(req.Diagram)
	nodes := make(map[string]bool, len(d.Nodes))
	for _, n := range d.Nodes {
		nodes[n.ID] = true
	}
	alt := alts[req.Alternative]
	for _, change := range alt.Changes {
		if !nodes[change.Node] {
			writeJSON(w, http.StatusOK, defendResponse{Reason: "This change needs a component that is not in your design."})
			return
		}
	}
	changed := scenarios.ApplyAlternative(d, alt.Changes)
	release, ok := s.acquireRunSlot(w)
	if !ok {
		return
	}
	defer release()
	seed := uint32(42)
	if req.Seed != nil {
		seed = *req.Seed
	}
	deadline := time.Now().Add(sweepDeadline)
	yours, done := runDefendDesign(d, sc, seed, deadline)
	if !done {
		writeErr(w, http.StatusServiceUnavailable, "the comparison took too long; try a smaller design")
		return
	}
	withChange, done := runDefendDesign(changed, sc, seed, deadline)
	if !done {
		writeErr(w, http.StatusServiceUnavailable, "the comparison took too long; try a smaller design")
		return
	}
	writeJSON(w, http.StatusOK, defendResponse{
		Applicable:  true,
		Alternative: &defendAlternative{Label: alt.Label, Why: alt.Why},
		Yours:       &yours, WithChange: &withChange,
	})
}

func runDefendDesign(d engine.Diagram, sc *scenarios.Scenario, seed uint32, deadline time.Time) (defendScore, bool) {
	d, _ = engine.Sanitize(d)
	opts := sc.Options(seed)
	sim := engine.New(d, opts)
	limit := opts.StopTrafficAtMs + 60000
	for !sim.Finished() && sim.Now() < limit {
		sim.Advance(100)
		if time.Now().After(deadline) {
			return defendScore{}, false
		}
	}
	run := sim.RunSnapshot()
	success := math.Floor(100*float64(run.OK)/max(1, float64(run.Resolved+run.Pending))*10) / 10
	return defendScore{
		Pass:        engine.Evaluate(sc.Goals, d, run, sim.CostPerHour()).Pass,
		SuccessPct:  success,
		P95Ms:       math.Round(run.P95),
		CostPerHour: math.Round(sim.CostPerHour()*100) / 100,
	}, true
}
