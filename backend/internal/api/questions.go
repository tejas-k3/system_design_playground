package api

import (
	"net/http"

	"sysdesign/internal/scenarios"
)

type questionSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Mode       string `json:"mode"`
	Difficulty string `json:"difficulty"`
	Summary    string `json:"summary"`
}

func (s *Server) listQuestions(w http.ResponseWriter, r *http.Request) {
	out := []questionSummary{}
	for _, q := range scenarios.AllQuestions() {
		out = append(out, questionSummary{q.ID, q.Title, q.Mode, q.Difficulty, q.Summary})
	}
	writeJSON(w, 200, out)
}

// getQuestion serves a question without its answer key (the key fields are not serialised at all).
func (s *Server) getQuestion(w http.ResponseWriter, r *http.Request) {
	q := scenarios.Q(r.PathValue("id"))
	if q == nil {
		writeErr(w, 404, "no such question")
		return
	}
	writeJSON(w, 200, q)
}

type runCandidateReq struct {
	Candidate *int `json:"candidate"`
}

// runCandidate simulates one candidate design of a trade-off question (fixed seed, so everyone sees the same
// numbers) under the question's own traffic and failures. It shares the headless run slots.
func (s *Server) runCandidate(w http.ResponseWriter, r *http.Request) {
	q := scenarios.Q(r.PathValue("id"))
	if q == nil {
		writeErr(w, 404, "no such question")
		return
	}
	var req runCandidateReq
	if !decode(w, r, &req) {
		return
	}
	if q.Mode != scenarios.ModeTradeoff || req.Candidate == nil {
		writeErr(w, 400, "name one of the designs to run")
		return
	}
	s.metrics.runs.Add(1)
	select {
	case s.runSlots <- struct{}{}:
		defer func() { <-s.runSlots }()
	default:
		s.metrics.runsRefused.Add(1)
		w.Header().Set("Retry-After", "2")
		writeErr(w, http.StatusServiceUnavailable, "too many simulations running, try again shortly")
		return
	}
	res, err := q.RunCandidate(*req.Candidate, 42)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

type answerReq struct {
	Answers map[string]float64 `json:"answers"` // estimate questions
	Choice  *int               `json:"choice"`  // multiple-choice questions
}

// answerQuestion grades a submission. Only now do the expected values and workings leave the server.
func (s *Server) answerQuestion(w http.ResponseWriter, r *http.Request) {
	q := scenarios.Q(r.PathValue("id"))
	if q == nil {
		writeErr(w, 404, "no such question")
		return
	}
	var req answerReq
	if !decode(w, r, &req) {
		return
	}
	if q.Mode == scenarios.ModeDiagnose || q.Mode == scenarios.ModeTradeoff {
		if req.Choice == nil {
			writeErr(w, 400, "choose one of the options")
			return
		}
		res, err := q.GradeChoice(*req.Choice)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, res)
		return
	}
	if q.Mode != scenarios.ModeEstimate || req.Choice != nil {
		writeErr(w, 400, "this kind of question is not answered that way")
		return
	}
	res, err := q.GradeEstimate(req.Answers)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, res)
}
