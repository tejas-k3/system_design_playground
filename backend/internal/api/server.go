// Package api exposes the simulator over HTTP (registry, scenarios, headless runs) and a
// WebSocket that drives live simulations.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// DefaultIdleTimeout is how long a stopped or paused session may sit untouched before it is closed.
const DefaultIdleTimeout = 10 * time.Minute

// Config configures a Server.
type Config struct {
	StaticDir      string        // built frontend to serve with an index.html fallback (optional)
	OriginPatterns []string      // allowed WebSocket origins
	MaxSessions    int           // concurrent live simulations; 0 means DefaultMaxSessions
	IdleTimeout    time.Duration // close a session that is not running and has been silent this long; 0 means DefaultIdleTimeout
}

// Server wires the routes together.
type Server struct {
	mux       *http.ServeMux
	staticDir string
	origins   []string
	slots     chan struct{} // one token per live simulation session
	idle      time.Duration
}

// New builds a server from cfg.
func New(cfg Config) *Server {
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = DefaultMaxSessions
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	s := &Server{mux: http.NewServeMux(), staticDir: cfg.StaticDir, origins: cfg.OriginPatterns, slots: make(chan struct{}, cfg.MaxSessions), idle: cfg.IdleTimeout}
	staticDir := cfg.StaticDir
	s.mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	s.mux.HandleFunc("GET /api/registry", s.registry)
	s.mux.HandleFunc("GET /api/scenarios", s.listScenarios)
	s.mux.HandleFunc("GET /api/scenarios/{id}", s.getScenario)
	s.mux.HandleFunc("GET /api/scenarios/{id}/solution", s.getSolution)
	s.mux.HandleFunc("POST /api/validate", s.validate)
	s.mux.HandleFunc("POST /api/run", s.run)
	s.mux.HandleFunc("GET /api/sim", s.sim)
	if staticDir != "" {
		s.mux.HandleFunc("GET /", s.static)
	}
	return s
}

func (s *Server) Handler() http.Handler { return logRequests(s.mux) }

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
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Millisecond))
		}
	})
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
	writeJSON(w, 200, map[string]any{"nodes": engine.Specs(), "latency": map[string]float64{"min": engine.MinLatencyMs, "max": engine.MaxLatencyMs}})
}

type scenarioSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Difficulty string `json:"difficulty"`
	Summary    string `json:"summary"`
	ConfigOnly bool   `json:"configOnly,omitempty"`
}

func (s *Server) listScenarios(w http.ResponseWriter, r *http.Request) {
	out := []scenarioSummary{}
	for _, sc := range scenarios.All() {
		out = append(out, scenarioSummary{sc.ID, sc.Title, sc.Difficulty, sc.Summary, sc.ConfigOnly})
	}
	writeJSON(w, 200, out)
}

func (s *Server) getScenario(w http.ResponseWriter, r *http.Request) {
	sc := scenarios.Get(r.PathValue("id"))
	if sc == nil {
		writeErr(w, 404, "no such scenario")
		return
	}
	writeJSON(w, 200, sc)
}

func (s *Server) getSolution(w http.ResponseWriter, r *http.Request) {
	sc := scenarios.Get(r.PathValue("id"))
	if sc == nil {
		writeErr(w, 404, "no such scenario")
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
	writeJSON(w, 200, map[string]any{"diagram": d, "adjusted": adjusted, "costPerHour": engine.DiagramCost(d)})
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
	resp := runResp{Stats: sim.Snapshot(), CostPerHour: engine.DiagramCost(d), Truncated: truncated}
	if sc != nil {
		sc := engine.Evaluate(sc.Goals, d, sim.RunSnapshot(), resp.CostPerHour)
		resp.Score = &sc
	}
	writeJSON(w, 200, resp)
}

// static serves the built frontend, falling back to index.html for client-side routes.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	p := filepath.Join(s.staticDir, filepath.Clean("/"+r.URL.Path))
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		http.ServeFile(w, r, p)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.staticDir, "index.html"))
}
