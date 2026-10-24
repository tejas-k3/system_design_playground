// Package api exposes the simulator over HTTP (registry, scenarios, headless runs) and a
// WebSocket that drives live simulations.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"sysdesign/internal/designs"
	"sysdesign/internal/engine"
	"sysdesign/internal/scenarios"
)

const (
	maxBody  = 1 << 20
	maxNodes = 120
	maxEdges = 300
)

// DefaultMaxSessions bounds concurrent live simulations; each one burns CPU while running.
const DefaultMaxSessions = 64

// defaultMaxRuns bounds concurrent headless runs (0 in Config means this many per CPU core, at least 2): each
// holds a whole simulation in memory (about 12 MB for a heavy design), and the CPU is the real limit anyway.
func defaultMaxRuns() int { return max(2, runtime.GOMAXPROCS(0)) }

// DefaultSaveLimit is how many designs one client may share per minute.
const DefaultSaveLimit = 20

// maxDesignBody bounds a shared design; a full 120-node diagram is far smaller.
const maxDesignBody = 256 << 10

// DefaultIdleTimeout is how long a stopped or paused session may sit untouched before it is closed.
const DefaultIdleTimeout = 10 * time.Minute

// Config configures a Server.
type Config struct {
	StaticDir      string         // built frontend to serve with an index.html fallback (optional)
	OriginPatterns []string       // allowed WebSocket origins
	MaxSessions    int            // concurrent live simulations; 0 means DefaultMaxSessions
	MaxRuns        int            // concurrent headless /api/run simulations; 0 means one per CPU core (at least 2)
	IdleTimeout    time.Duration  // close a session that is not running and has been silent this long; 0 means DefaultIdleTimeout
	Designs        *designs.Store // where shared designs live; nil means an in-memory store
	SaveLimit      int            // designs one client may save per minute; 0 means DefaultSaveLimit
}

// Server wires the routes together.
type Server struct {
	mux       *http.ServeMux
	staticDir string
	origins   []string
	slots     chan struct{} // one token per live simulation session
	runSlots  chan struct{} // one token per headless run in progress
	idle      time.Duration
	designs   *designs.Store
	saves     *limiter
	metrics   metrics
}

// New builds a server from cfg.
func New(cfg Config) *Server {
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = DefaultMaxSessions
	}
	if cfg.MaxRuns <= 0 {
		cfg.MaxRuns = defaultMaxRuns()
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if cfg.SaveLimit <= 0 {
		cfg.SaveLimit = DefaultSaveLimit
	}
	if cfg.Designs == nil {
		cfg.Designs, _ = designs.New("", 0) // in-memory never fails
	}
	s := &Server{designs: cfg.Designs, saves: newLimiter(cfg.SaveLimit, time.Minute), mux: http.NewServeMux(), staticDir: cfg.StaticDir, origins: cfg.OriginPatterns, slots: make(chan struct{}, cfg.MaxSessions), runSlots: make(chan struct{}, cfg.MaxRuns), idle: cfg.IdleTimeout}
	staticDir := cfg.StaticDir
	s.mux.HandleFunc("GET /api/metrics", s.metrics.serve(cfg.Designs.Len))
	s.mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	s.mux.HandleFunc("GET /api/registry", s.registry)
	s.mux.HandleFunc("GET /api/scenarios", s.listScenarios)
	s.mux.HandleFunc("GET /api/scenarios/{id}", s.getScenario)
	s.mux.HandleFunc("GET /api/scenarios/{id}/solution", s.getSolution)
	s.mux.HandleFunc("GET /api/scenarios/{id}/alternatives", s.getAlternatives)
	s.mux.HandleFunc("GET /api/scenarios/{id}/hints/{n}", s.getHint)
	s.mux.HandleFunc("GET /api/scenarios/{id}/answer-hint", s.getAnswerHint)
	s.mux.HandleFunc("POST /api/scenarios/{id}/predictions/{pid}", s.answerPrediction)
	s.mux.HandleFunc("GET /api/scenarios/{id}/debrief", s.getDebrief)
	s.mux.HandleFunc("GET /api/questions", s.listQuestions)
	s.mux.HandleFunc("GET /api/questions/{id}", s.getQuestion)
	s.mux.HandleFunc("POST /api/questions/{id}/answer", s.answerQuestion)
	s.mux.HandleFunc("POST /api/questions/{id}/run", s.runCandidate)
	s.mux.HandleFunc("POST /api/questions/{id}/review", s.reviewQuestion)
	s.mux.HandleFunc("POST /api/questions/{id}/build", s.buildQuestion)
	s.mux.HandleFunc("POST /api/validate", s.validate)
	s.mux.HandleFunc("POST /api/run", s.run)
	s.mux.HandleFunc("POST /api/sweep", s.sweep)
	s.mux.HandleFunc("POST /api/designs", s.saveDesign)
	s.mux.HandleFunc("GET /api/designs/{id}", s.loadDesign)
	s.mux.HandleFunc("GET /api/sim", s.sim)
	if staticDir != "" {
		s.mux.HandleFunc("GET /", s.static)
	}
	return s
}

func (s *Server) Handler() http.Handler { return secureHeaders(logRequests(s.mux)) }

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) { w.code = c; w.ResponseWriter.WriteHeader(c) }

// Hijack/Flush pass through so WebSocket upgrades keep working.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/healthz" {
			log.Printf("%s %s %d %s", r.Method, routeLabel(r), sw.code, time.Since(start).Round(time.Millisecond))
		}
	})
}

// routeLabel is the mux pattern that served the request without its method ("/api/designs/{id}"),
// never the raw path, which can carry a design id. Unmatched requests and 405s have no pattern.
func routeLabel(r *http.Request) string {
	if _, route, ok := strings.Cut(r.Pattern, " "); ok {
		return route
	}
	if r.Pattern != "" {
		return r.Pattern
	}
	return "(unmatched)"
}

// pathFreeErr drops the file path from an *fs.PathError (it holds the design id) and keeps the
// operation and the underlying cause.
func pathFreeErr(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Op + ": " + pe.Err.Error()
	}
	return err.Error()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func tooBig(d engine.Diagram) bool { return len(d.Nodes) > maxNodes || len(d.Edges) > maxEdges }

// ---- handlers -------------------------------------------------------------------------

func (s *Server) registry(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"nodes":   engine.Specs(),
		"latency": map[string]any{"min": engine.MinLatencyMs, "max": engine.MaxLatencyMs, "rules": engine.LatencyRules()},
		"speeds":  Speeds,
		"reasons": engine.ReasonInfos(),
	})
}

type scenarioSummary struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Difficulty string   `json:"difficulty"`
	Level      string   `json:"level"`
	Summary    string   `json:"summary"`
	ConfigOnly bool     `json:"configOnly,omitempty"`
	Open       bool     `json:"open,omitempty"`
	Concepts   []string `json:"concepts,omitempty"`
}

func (s *Server) listScenarios(w http.ResponseWriter, r *http.Request) {
	out := []scenarioSummary{}
	for _, sc := range scenarios.All() {
		out = append(out, scenarioSummary{sc.ID, sc.Title, sc.Difficulty, sc.Level, sc.Summary, sc.ConfigOnly, sc.Open, sc.Concepts})
	}
	writeJSON(w, 200, out)
}

func (s *Server) getScenario(w http.ResponseWriter, r *http.Request) {
	sc := scenarios.Get(r.PathValue("id"))
	if sc == nil {
		writeErr(w, 404, "no such scenario")
		return
	}
	pub := sc.Public()
	if sc.Open { // the load and the failures are part of the test: the server runs them, the client never sees them
		pub.Traffic, pub.Chaos = nil, nil
	}
	writeJSON(w, 200, pub)
}

// getHint reveals one hint, when the player asks for it.
func (s *Server) getHint(w http.ResponseWriter, r *http.Request) {
	sc := scenarios.Get(r.PathValue("id"))
	if sc == nil {
		writeErr(w, 404, "no such scenario")
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || sc.Hint(n) == "" {
		writeErr(w, 404, "no such hint")
		return
	}
	writeJSON(w, 200, map[string]any{"level": n, "hint": sc.Hint(n)})
}

// getAnswerHint is the plain statement of the solution, behind its own request after the three hints.
func (s *Server) getAnswerHint(w http.ResponseWriter, r *http.Request) {
	sc := scenarios.Get(r.PathValue("id"))
	if sc == nil {
		writeErr(w, 404, "no such scenario")
		return
	}
	if sc.AnswerHint() == "" {
		writeErr(w, 404, "no answer hint")
		return
	}
	writeJSON(w, 200, map[string]string{"answer": sc.AnswerHint()})
}

// answerPrediction takes the player's choice and only then says which answer was right and why.
func (s *Server) answerPrediction(w http.ResponseWriter, r *http.Request) {
	sc := scenarios.Get(r.PathValue("id"))
	if sc == nil {
		writeErr(w, 404, "no such scenario")
		return
	}
	q := sc.Prediction(r.PathValue("pid"))
	if q == nil {
		writeErr(w, 404, "no such question")
		return
	}
	var req struct {
		Choice int `json:"choice"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Choice < 0 || req.Choice >= len(q.Options) {
		writeErr(w, 400, "choose one of the options")
		return
	}
	// the player chose a position in the shuffled order (scenarios.ShownOptions); answer in the same positions
	writeJSON(w, 200, map[string]any{"choice": req.Choice, "correct": sc.StoredIndex(q, req.Choice) == q.Answer, "answer": sc.ShownAnswer(q), "explain": q.Explain})
}

// getDebrief is shown with the result, not before it.
func (s *Server) getDebrief(w http.ResponseWriter, r *http.Request) {
	sc := scenarios.Get(r.PathValue("id"))
	if sc == nil {
		writeErr(w, 404, "no such scenario")
		return
	}
	writeJSON(w, 200, map[string]string{"debrief": sc.Debrief, "deeper": sc.Deeper})
}

func (s *Server) getAlternatives(w http.ResponseWriter, r *http.Request) {
	sc := scenarios.Get(r.PathValue("id"))
	if sc == nil {
		writeErr(w, 404, "no such scenario")
		return
	}
	alts := scenarios.Alternatives(sc.ID)
	if alts == nil {
		alts = []scenarios.Alternative{}
	}
	writeJSON(w, 200, alts)
}

func (s *Server) getSolution(w http.ResponseWriter, r *http.Request) {
	sc := scenarios.Get(r.PathValue("id"))
	if sc == nil {
		writeErr(w, 404, "no such scenario")
		return
	}
	if sc.Open {
		writeErr(w, 404, "this challenge has no reference solution")
		return
	}
	writeJSON(w, 200, sc.Solution)
}

type validateReq struct {
	Diagram engine.Diagram `json:"diagram"`
}

func (s *Server) validate(w http.ResponseWriter, r *http.Request) {
	var req validateReq
	if !decode(w, r, &req) {
		return
	}
	if tooBig(req.Diagram) {
		writeErr(w, 413, "diagram too large")
		return
	}
	d, adjusted := engine.Sanitize(req.Diagram)
	findings := engine.Lint(d)
	if findings == nil {
		findings = []engine.Finding{}
	}
	writeJSON(w, 200, map[string]any{"diagram": d, "adjusted": adjusted, "costPerHour": engine.DiagramCost(d), "findings": findings})
}

type runReq struct {
	Diagram     engine.Diagram `json:"diagram"`
	ScenarioID  string         `json:"scenarioId"`
	Seed        *uint32        `json:"seed"`
	DurationSec float64        `json:"durationSec"`
}

type runResp struct {
	Score       *engine.Score `json:"score,omitempty"`
	Stats       engine.Stats  `json:"stats"`
	CostPerHour float64       `json:"costPerHour"`
	Truncated   bool          `json:"truncated,omitempty"`
}

// run simulates headlessly. With a scenarioId it runs the scenario to the end and scores it;
// otherwise it runs the diagram's own traffic for durationSec (default 30, max 300).
func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	var req runReq
	if !decode(w, r, &req) {
		return
	}
	s.metrics.runs.Add(1)
	if tooBig(req.Diagram) {
		writeErr(w, 413, "diagram too large")
		return
	}
	d, _ := engine.Sanitize(req.Diagram)
	seed := uint32(42)
	if req.Seed != nil {
		seed = *req.Seed
	}
	opts := engine.Options{Seed: seed}
	var sc *scenarios.Scenario
	if req.ScenarioID != "" {
		if sc = scenarios.Get(req.ScenarioID); sc == nil {
			writeErr(w, 404, "no such scenario")
			return
		}
		opts = sc.Options(seed)
	} else {
		dur := req.DurationSec
		if dur <= 0 {
			dur = 30
		}
		if dur > 300 {
			dur = 300
		}
		opts.StopTrafficAtMs = dur * 1000
	}
	release, ok := s.acquireRunSlot(w)
	if !ok {
		return
	}
	defer release()
	sim := engine.New(d, opts)
	deadline := time.Now().Add(10 * time.Second)
	limit := opts.StopTrafficAtMs + 60000
	truncated := false
	for !sim.Finished() && sim.Now() < limit {
		sim.Advance(100)
		if time.Now().After(deadline) {
			truncated = true
			break
		}
	}
	resp := runResp{Stats: sim.Snapshot(), CostPerHour: sim.CostPerHour(), Truncated: truncated}
	if sc != nil {
		sc := engine.Evaluate(sc.Goals, d, sim.RunSnapshot(), resp.CostPerHour)
		resp.Score = &sc
	}
	writeJSON(w, 200, resp)
}

// acquireRunSlot takes one of the headless-run slots. When all are busy it counts the refusal, writes
// the 503 (Retry-After 2) and returns ok=false, and the caller just returns. Otherwise the caller
// must call release when the run is over (defer it).
func (s *Server) acquireRunSlot(w http.ResponseWriter) (release func(), ok bool) {
	select {
	case s.runSlots <- struct{}{}:
		return func() { <-s.runSlots }, true
	default:
		s.metrics.runsRefused.Add(1)
		w.Header().Set("Retry-After", "2")
		writeErr(w, http.StatusServiceUnavailable, "too many simulations running, try again shortly")
		return nil, false
	}
}

// saveDesign stores a diagram (sanitized first) and returns the id that opens it.
func (s *Server) saveDesign(w http.ResponseWriter, r *http.Request) {
	if !s.saves.allow(clientAddr(r)) {
		s.metrics.designsRateLimit.Add(1)
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "too many designs shared, try again in a minute")
		return
	}
	var req validateReq
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDesignBody))
	if err := dec.Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid or oversized design")
		return
	}
	if tooBig(req.Diagram) {
		writeErr(w, http.StatusRequestEntityTooLarge, "diagram too large")
		return
	}
	d, _ := engine.Sanitize(req.Diagram)
	if len(d.Nodes) == 0 {
		writeErr(w, http.StatusBadRequest, "nothing to share: the design is empty")
		return
	}
	id, err := s.designs.Put(d)
	if err != nil {
		log.Printf("save design: %s", pathFreeErr(err))
		writeErr(w, http.StatusInternalServerError, "could not save the design")
		return
	}
	s.metrics.designsSaved.Add(1)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) loadDesign(w http.ResponseWriter, r *http.Request) {
	d, ok, err := s.designs.Get(r.PathValue("id"))
	switch {
	case errors.Is(err, designs.ErrBadID), err == nil && !ok:
		writeErr(w, http.StatusNotFound, "no such design")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "could not read the design")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"diagram": d})
	}
}

// static serves the built frontend, falling back to index.html for client-side routes.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	p := filepath.Join(s.staticDir, filepath.Clean("/"+r.URL.Path))
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		w.Header().Set("Cache-Control", cacheControl(r.URL.Path))
		http.ServeFile(w, r, p)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(s.staticDir, "index.html"))
}
