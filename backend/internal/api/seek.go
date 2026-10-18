package api

import (
	"errors"

	"sysdesign/internal/engine"
)

// maxEditLog bounds the history kept for replay. Past it the oldest edits are dropped and replay
// is refused for the rest of the run: an approximation would not match what was shown.
const maxEditLog = 500

// edit is something the learner did to a running simulation, at the simulated time they did it.
type edit struct {
	t       float64 // ms of simulated time
	kind    string  // diagram | crash | restart
	diagram engine.Diagram
	node    string
}

func (ss *session) logEdit(e edit) {
	if ss.sim == nil {
		return
	}
	e.t = ss.sim.Now()
	ss.edits = append(ss.edits, e)
	if len(ss.edits) > maxEditLog {
		ss.edits = ss.edits[len(ss.edits)-maxEditLog:]
		ss.replayLost = true
	}
}

// applyDiagram makes d the live design, syncing a running simulation and recording the edit.
func (ss *session) applyDiagram(d engine.Diagram) {
	ss.diagram = d
	if ss.sim != nil {
		ss.sim.Sync(d)
		ss.logEdit(edit{kind: "diagram", diagram: d})
	}
}

func (ss *session) applyCrash(id string) {
	if ss.sim != nil {
		ss.sim.CrashNode(id, 0)
		ss.logEdit(edit{kind: "crash", node: id})
	}
}

func (ss *session) applyRestart(id string) {
	if ss.sim != nil {
		ss.sim.RestartNode(id)
		ss.logEdit(edit{kind: "restart", node: id})
	}
}

// seek rebuilds the simulation from the design it started with and replays every logged edit at the
// simulated time it happened, stopping exactly at tMs. The engine is deterministic, so the result is
// identical to what the live run showed at that moment. Edits after tMs are forgotten.
func (ss *session) seek(tMs float64) error {
	switch {
	case ss.sim == nil:
		return errors.New("nothing to rewind: start a run first")
	case ss.replayLost:
		return errors.New("this run has been edited too many times to replay")
	case tMs < 0 || tMs > ss.sim.Now():
		return errors.New("that moment has not happened yet")
	}
	opts := engine.Options{Seed: ss.seed}
	if ss.sc != nil {
		opts = ss.sc.Options(ss.seed)
	}
	sim := engine.New(ss.base, opts)
	sim.EnableParticles(true)
	sim.EnableTraces(true)
	keep := 0
	d := ss.base
	for _, e := range ss.edits {
		if e.t > tMs {
			break
		}
		sim.Advance(e.t - sim.Now())
		switch e.kind {
		case "diagram":
			sim.Sync(e.diagram)
			d = e.diagram
		case "crash":
			sim.CrashNode(e.node, 0)
		case "restart":
			sim.RestartNode(e.node)
		}
		keep++
	}
	sim.Advance(tMs - sim.Now())
	ss.sim, ss.diagram, ss.edits = sim, d, ss.edits[:keep]
	ss.finished = sim.Finished()
	ss.chaosSeen = len(sim.ChaosLog()) // their toasts were shown the first time
	ss.slowWarned = false
	ss.status = "paused"
	return nil
}
