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

type answerReq struct {
	Answers map[string]float64 `json:"answers"`
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
	if q.Mode != scenarios.ModeEstimate {
		writeErr(w, 400, "this kind of question is not answered with numbers")
		return
	}
	res, err := q.GradeEstimate(req.Answers)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, res)
}
