package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"sysdesign/internal/engine"
	"sysdesign/internal/scenarios"
)

const (
	frameEvery  = 40 * time.Millisecond
	statsEvery  = 100 * time.Millisecond
	maxStepWall = 12 * time.Millisecond
	sliceMs     = 10.0
	maxDtMs     = 100.0
)

// Speeds are the simulation speed multipliers a session accepts; clients offer exactly these.
var Speeds = []float64{0.02, 0.05, 0.1, 0.25, 0.5, 1, 2, 4}

type clientMsg struct {
	Type       string          `json:"type"`
	Diagram    *engine.Diagram `json:"diagram,omitempty"`
	ScenarioID string          `json:"scenarioId,omitempty"`
	Seed       *uint32         `json:"seed,omitempty"`
	Speed      float64         `json:"speed,omitempty"`
	NodeID     string          `json:"nodeId,omitempty"`
}

type notice struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

type frameMsg struct {
	Type      string            `json:"type"`
	Status    string            `json:"status"`
	Speed     float64           `json:"speed"`
	Stats     *engine.Stats     `json:"stats,omitempty"`
	Particles []engine.Particle `json:"particles,omitempty"`
	Notices   []notice          `json:"notices,omitempty"`
}

type resultMsg struct {
	Type       string       `json:"type"`
	ScenarioID string       `json:"scenarioId"`
	Score      engine.Score `json:"score"`
}

// tracesMsg answers a "traces" request with the recent failed and successful request traces.
type tracesMsg struct {
	Type string `json:"type"`
	engine.TraceSet
}

type errMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type session struct {
	conn       *websocket.Conn
	diagram    engine.Diagram
	sc         *scenarios.Scenario
	seed       uint32
	speed      float64
	status     string // stopped | running | paused
	sim        *engine.Sim
	finished   bool
	chaosSeen  int
	slowWarned bool
	pending    []notice
	lastWall   time.Time
	lastStat   time.Time
	active     time.Time // last client message, or the last moment the simulation was running
	idle       time.Duration
}

func emptyStats() *engine.Stats {
	return &engine.Stats{Nodes: map[string]engine.NodeStats{}, Edges: map[string]engine.EdgeStats{}, Series: []engine.SeriesPoint{}}
}

func (s *Server) sim(w http.ResponseWriter, r *http.Request) {
	select {
	case s.slots <- struct{}{}:
		s.metrics.sessionsActive.Add(1)
		s.metrics.sessionsTotal.Add(1)
		defer func() {
			<-s.slots
			s.metrics.sessionsActive.Add(-1)
		}()
	default:
		s.metrics.sessionsRefused.Add(1)
		w.Header().Set("Retry-After", "5")
		writeErr(w, http.StatusServiceUnavailable, "too many active simulations, try again shortly")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.origins})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxBody)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	in := make(chan clientMsg, 64)
	go func() {
		defer cancel()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var m clientMsg
			if json.Unmarshal(data, &m) != nil {
				continue
			}
			select {
			case in <- m:
			case <-ctx.Done():
				return
			}
		}
	}()

	ss := &session{conn: conn, seed: 42, speed: 1, status: "stopped", active: time.Now(), idle: s.idle}
	ticker := time.NewTicker(frameEvery)
	defer ticker.Stop()
	ss.send(ctx, frameMsg{Type: "frame", Status: ss.status, Speed: ss.speed, Stats: emptyStats()})
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-in:
			ss.active = time.Now()
			if !ss.handle(ctx, m) {
				return
			}
		case <-ticker.C:
			if !ss.tick(ctx) {
				return
			}
		}
	}
}

func (ss *session) send(ctx context.Context, v any) bool {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("marshal: %v", err)
		return true
	}
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return ss.conn.Write(wctx, websocket.MessageText, data) == nil
}

func (ss *session) fail(ctx context.Context, msg string) bool {
	return ss.send(ctx, errMsg{Type: "error", Message: msg})
}

func (ss *session) frame(withStats bool) frameMsg {
	f := frameMsg{Type: "frame", Status: ss.status, Speed: ss.speed, Notices: ss.pending}
	ss.pending = nil
	if ss.sim != nil {
		f.Particles = ss.sim.TakeParticles()
		if withStats {
			st := ss.sim.Snapshot()
			f.Stats = &st
		}
	} else if withStats {
		f.Stats = emptyStats()
	}
	return f
}

func sameTopology(a, b engine.Diagram) bool {
	if len(a.Nodes) != len(b.Nodes) || len(a.Edges) != len(b.Edges) {
		return false
	}
	nodes := map[string]string{}
	for _, n := range a.Nodes {
		nodes[n.ID] = n.Kind
	}
	for _, n := range b.Nodes {
		if nodes[n.ID] != n.Kind {
			return false
		}
	}
	edges := map[string]string{}
	for _, e := range a.Edges {
		edges[e.ID] = e.From + ">" + e.To
	}
	for _, e := range b.Edges {
		if edges[e.ID] != e.From+">"+e.To {
			return false
		}
	}
	return true
}

func (ss *session) build() {
	opts := engine.Options{Seed: ss.seed}
	if ss.sc != nil {
		opts = ss.sc.Options(ss.seed)
	}
	ss.sim = engine.New(ss.diagram, opts)
	ss.sim.EnableParticles(true)
	ss.sim.EnableTraces(true)
	ss.finished = false
	ss.chaosSeen = 0
	ss.slowWarned = false
}

func nearestSpeed(v float64) float64 {
	best := Speeds[0]
	for _, s := range Speeds {
		if math.Abs(s-v) < math.Abs(best-v) {
			best = s
		}
	}
	return best
}

// handle applies one client message. It returns false when the connection should close.
func (ss *session) handle(ctx context.Context, m clientMsg) bool {
	switch m.Type {
	case "init":
		ss.sim, ss.status, ss.sc = nil, "stopped", nil
		if m.ScenarioID != "" {
			if ss.sc = scenarios.Get(m.ScenarioID); ss.sc == nil {
				return ss.fail(ctx, "no such scenario")
			}
		}
		if m.Diagram != nil {
			if tooBig(*m.Diagram) {
				return ss.fail(ctx, "diagram too large")
			}
			ss.diagram, _ = engine.Sanitize(*m.Diagram)
		}
		if m.Seed != nil {
			ss.seed = *m.Seed
		}
		if m.Speed > 0 {
			ss.speed = nearestSpeed(m.Speed)
		}
		return ss.send(ctx, ss.frame(true))
	case "diagram":
		if m.Diagram == nil {
			return true
		}
		if tooBig(*m.Diagram) {
			return ss.fail(ctx, "diagram too large")
		}
		d, _ := engine.Sanitize(*m.Diagram)
		if ss.sc != nil && ss.sc.ConfigOnly && !sameTopology(ss.diagram, d) {
			return ss.fail(ctx, "Incident mode: fix it with configuration, the architecture is fixed")
		}
		ss.diagram = d
		if ss.sim != nil {
			ss.sim.Sync(d)
		}
	case "play":
		if ss.sim != nil && ss.finished {
			ss.sim = nil
		}
		if ss.sim == nil {
			ss.build()
		}
		ss.status, ss.lastWall = "running", time.Now()
		return ss.send(ctx, ss.frame(true))
	case "pause":
		if ss.status == "running" {
			ss.status = "paused"
		}
		return ss.send(ctx, ss.frame(true))
	case "step":
		if ss.sim == nil || ss.finished {
			ss.build()
		}
		ss.status = "paused"
		ss.sim.Advance(100)
		ss.afterAdvance(ctx)
		return ss.send(ctx, ss.frame(true))
	case "reset":
		ss.sim, ss.status = nil, "stopped"
		return ss.send(ctx, ss.frame(true))
	case "speed":
		ss.speed = nearestSpeed(m.Speed)
		return ss.send(ctx, ss.frame(false))
	case "traces":
		set := engine.TraceSet{Failed: []engine.TraceOut{}, OK: []engine.TraceOut{}}
		if ss.sim != nil {
			set = ss.sim.Traces()
		}
		return ss.send(ctx, tracesMsg{Type: "traces", TraceSet: set})
	case "crash":
		if ss.sim != nil {
			ss.sim.CrashNode(m.NodeID, 0)
		}
	case "restart":
		if ss.sim != nil {
			ss.sim.RestartNode(m.NodeID)
		}
	}
	return true
}

// tick advances a running simulation by the wall time elapsed (scaled by speed) and sends a frame.
func (ss *session) tick(ctx context.Context) bool {
	if ss.status != "running" || ss.sim == nil {
		if time.Since(ss.active) > ss.idle {
			ss.conn.Close(websocket.StatusGoingAway, "idle")
			return false
		}
		return true
	}
	now := time.Now()
	ss.active = now
	dt := float64(now.Sub(ss.lastWall).Microseconds()) / 1000
	ss.lastWall = now
	if dt > maxDtMs {
		dt = maxDtMs
	}
	want := dt * ss.speed
	began := time.Now()
	done := 0.0
	for done < want {
		step := want - done
		if step > sliceMs {
			step = sliceMs
		}
		ss.sim.Advance(step)
		done += step
		if done < want && time.Since(began) > maxStepWall {
			if !ss.slowWarned {
				ss.slowWarned = true
				ss.pending = append(ss.pending, notice{"warn", "Heavy load: running in slow motion"})
			}
			break
		}
	}
	ss.afterAdvance(ctx)
	withStats := now.Sub(ss.lastStat) >= statsEvery || ss.finished
	if withStats {
		ss.lastStat = now
	}
	return ss.send(ctx, ss.frame(withStats))
}

func (ss *session) afterAdvance(ctx context.Context) {
	log := ss.sim.ChaosLog()
	for ; ss.chaosSeen < len(log); ss.chaosSeen++ {
		c := log[ss.chaosSeen]
		txt := "Chaos: " + c.Label + " got slower"
		switch c.Type {
		case "crash":
			txt = "Chaos: " + c.Label + " crashed"
		case "partition":
			txt = "Chaos: " + c.Label + " was cut off from the network"
		case "splitbrain":
			txt = fmt.Sprintf("Split brain: %s came back unfenced and took over again; %d writes made to the new primary were lost", c.Label, c.Lost)
		case "promotion":
			txt = fmt.Sprintf("Failover: %s is the primary now; %d acknowledged writes were lost", c.Label, c.Lost)
		}
		ss.pending = append(ss.pending, notice{"warn", txt})
	}
	if ss.sc != nil && !ss.finished && ss.sim.Finished() {
		ss.finished = true
		ss.status = "paused"
		score := engine.Evaluate(ss.sc.Goals, ss.diagram, ss.sim.RunSnapshot(), ss.sim.CostPerHour())
		ss.send(ctx, resultMsg{Type: "result", ScenarioID: ss.sc.ID, Score: score})
	}
}
