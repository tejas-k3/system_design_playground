package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"sysdesign/internal/engine"
)

// A component counts as saturated at this share of busy.
const (
	saturatedAt = 0.95
	ladderSecs  = 20.0
)

var bottleneckLoads = []float64{1, 1.25, 1.5, 1.75, 2}

type bottleneckReq struct {
	Diagram engine.Diagram `json:"diagram"`
	Seed    *uint32        `json:"seed"`
	Guess   *string        `json:"guess"` // a component id, or "" for "nothing runs out"
}

type bottleneckPoint struct {
	Load        float64 `json:"load"`
	Rps         float64 `json:"rps"`
	SuccessRate float64 `json:"successRate"`
	P95         float64 `json:"p95"`
	Hottest     string  `json:"hottest"` // the busiest component at this load
	HottestUtil float64 `json:"hottestUtil"`
}

type bottleneckResp struct {
	Correct  bool              `json:"correct"`
	Culprits []string          `json:"culprits"` // empty when nothing saturates by twice today's load
	At       int               `json:"at"`       // index of the first load where something saturates, -1 for none
	Points   []bottleneckPoint `json:"points"`
	Explain  string            `json:"explain"`
}

// reaches lists every component reachable from id by following the calls.
func reaches(calls map[string][]string, id string) map[string]bool {
	seen := map[string]bool{}
	stack := []string{id}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, next := range calls[cur] {
			if !seen[next] {
				seen[next] = true
				stack = append(stack, next)
			}
		}
	}
	return seen
}

// firstToFail finds the lightest load at which some component is saturated, and which components are. utils
// holds each component's busy share at each load, lightest first; calls says who calls whom. A component whose
// own callee is saturated is only waiting on it (a server's threads sit busy behind a slow database), so it is
// not named: the culprits are the deepest saturated components.
func firstToFail(utils []map[string]float64, calls map[string][]string) (at int, who []string) {
	for i, u := range utils {
		saturated := map[string]bool{}
		for id, v := range u {
			if v >= saturatedAt {
				saturated[id] = true
			}
		}
		for id := range saturated {
			waiting := false
			for below := range reaches(calls, id) {
				if saturated[below] && below != id && !reaches(calls, below)[id] {
					waiting = true
				}
			}
			if !waiting {
				who = append(who, id)
			}
		}
		if len(who) > 0 {
			sort.Strings(who)
			return i, who
		}
	}
	return -1, nil
}

// subjectOf names the culprits as one phrase and gives the verb that agrees with it: "App Server ×3 reach".
func subjectOf(labels []string) (subject, verb string) {
	count := map[string]int{}
	var order []string
	for _, l := range labels {
		if count[l] == 0 {
			order = append(order, l)
		}
		count[l]++
	}
	for i, l := range order {
		if count[l] > 1 {
			order[i] = fmt.Sprintf("%s ×%d", l, count[l])
		}
	}
	verb = "reach"
	if len(labels) == 1 {
		verb = "reaches"
	}
	return strings.Join(order, " and "), verb
}

// bottleneck asks which part of a design runs out first when traffic doubles, then answers by running it.
// The answer leaves the server only with a guess, like a prediction's.
func (s *Server) bottleneck(w http.ResponseWriter, r *http.Request) {
	var req bottleneckReq
	if !decode(w, r, &req) {
		return
	}
	s.metrics.runs.Add(1)
	if tooBig(req.Diagram) {
		writeErr(w, http.StatusRequestEntityTooLarge, "diagram too large")
		return
	}
	d, _ := engine.Sanitize(req.Diagram)
	label := map[string]string{}
	for _, n := range d.Nodes {
		if n.Kind != engine.KClient {
			label[n.ID] = n.Label
		}
	}
	if len(label) == 0 || offered(d) <= 0 {
		writeErr(w, 400, "add some components and a source of traffic first")
		return
	}
	if req.Guess == nil {
		writeErr(w, 400, "say which component you think runs out first, or none")
		return
	}
	if _, known := label[*req.Guess]; *req.Guess != "" && !known {
		writeErr(w, 400, "that is not a component of this design")
		return
	}
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
	resp := bottleneckResp{Points: []bottleneckPoint{}}
	var utils []map[string]float64
	for _, m := range bottleneckLoads {
		pd, _ := engine.Sanitize(scaleLoad(d, m))
		opts := engine.Options{Seed: seed, StopTrafficAtMs: ladderSecs * 1000}
		sim := engine.New(pd, opts)
		// read how busy everything is while traffic is still flowing, not after it has stopped
		for sim.Now() < 0.8*opts.StopTrafficAtMs && !sim.Finished() {
			sim.Advance(100)
		}
		util := map[string]float64{}
		pt := bottleneckPoint{Load: m, Rps: offered(pd)}
		for id, ns := range sim.Snapshot().Nodes {
			if _, ok := label[id]; !ok {
				continue
			}
			util[id] = ns.Util
			if ns.Util > pt.HottestUtil || (ns.Util == pt.HottestUtil && id < pt.Hottest) {
				pt.Hottest, pt.HottestUtil = id, ns.Util
			}
		}
		for !sim.Finished() && sim.Now() < opts.StopTrafficAtMs+60000 {
			sim.Advance(100)
			if time.Now().After(deadline) {
				writeErr(w, http.StatusServiceUnavailable, "the check took too long; try a smaller design")
				return
			}
		}
		if run := sim.RunSnapshot(); run.Resolved > 0 {
			pt.SuccessRate, pt.P95 = float64(run.OK)/float64(run.Resolved), run.P95
		}
		utils = append(utils, util)
		resp.Points = append(resp.Points, pt)
	}
	calls := map[string][]string{}
	for _, e := range d.Edges {
		calls[e.From] = append(calls[e.From], e.To)
	}
	resp.At, resp.Culprits = firstToFail(utils, calls)
	switch {
	case resp.At < 0:
		resp.Correct = *req.Guess == ""
		resp.Explain = "No component is more than 95% busy up to twice today's traffic, so the design has room to spare at that load."
	default:
		resp.Correct = false
		for _, id := range resp.Culprits {
			resp.Correct = resp.Correct || id == *req.Guess
		}
		names := make([]string, len(resp.Culprits))
		for i, id := range resp.Culprits {
			names[i] = label[id]
		}
		subject, verb := subjectOf(names)
		resp.Explain = fmt.Sprintf("%s %s 95%% busy at %g times today's traffic (about %.0f requests a second): the first to run out.", subject, verb, resp.Points[resp.At].Load, resp.Points[resp.At].Rps)
	}
	writeJSON(w, 200, resp)
}
