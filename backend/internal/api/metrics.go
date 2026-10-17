package api

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// metrics are cheap process-wide counters exposed in the Prometheus text format, so a deployment
// can watch load without extra dependencies.
type metrics struct {
	sessionsActive   atomic.Int64
	sessionsTotal    atomic.Int64
	sessionsRefused  atomic.Int64
	designsSaved     atomic.Int64
	designsRateLimit atomic.Int64
	runs             atomic.Int64
	runsRefused      atomic.Int64
}

func (m *metrics) serve(storedDesigns func() int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		line := func(name, help, kind string, v int64) {
			fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %d\n", name, help, name, kind, name, v)
		}
		line("sdp_sessions_active", "Live simulation sessions right now.", "gauge", m.sessionsActive.Load())
		line("sdp_sessions_total", "Simulation sessions opened since start.", "counter", m.sessionsTotal.Load())
		line("sdp_sessions_refused_total", "Sessions refused because the server was full.", "counter", m.sessionsRefused.Load())
		line("sdp_runs_total", "Headless runs requested through /api/run.", "counter", m.runs.Load())
		line("sdp_runs_refused_total", "Headless runs refused because too many were already running.", "counter", m.runsRefused.Load())
		line("sdp_designs_saved_total", "Designs shared since start.", "counter", m.designsSaved.Load())
		line("sdp_designs_rate_limited_total", "Design saves refused by the rate limit.", "counter", m.designsRateLimit.Load())
		line("sdp_designs_stored", "Designs currently kept.", "gauge", int64(storedDesigns()))
	}
}
