package api

import (
	"encoding/json"
	"reflect"
	"testing"

	"sysdesign/internal/engine"
	"sysdesign/internal/scenarios"
)

// bumpThreads returns the diagram with every app server's thread count changed.
func bumpThreads(d engine.Diagram, threads float64) engine.Diagram {
	out := d
	out.Nodes = append([]engine.Node(nil), d.Nodes...)
	for i, n := range out.Nodes {
		if n.Kind == engine.KAppServer {
			p := engine.Params{}
			for k, v := range n.Params {
				p[k] = v
			}
			p["threads"] = threads
			out.Nodes[i].Params = p
		}
	}
	return out
}

func appID(d engine.Diagram) string {
	for _, n := range d.Nodes {
		if n.Kind == engine.KAppServer {
			return n.ID
		}
	}
	return ""
}

// runLive plays a session in irregular slices, the way wall-clock ticks do, with edits in between.
func runLive(t *testing.T, ss *session, start engine.Diagram) {
	t.Helper()
	ss.diagram = start
	ss.build()
	steps := []struct {
		ms   float64
		edit func()
	}{
		{4130, nil},
		{777, func() { ss.applyDiagram(bumpThreads(start, 2)) }},
		{6003, nil},
		{250, func() { ss.applyCrash(appID(start)) }},
		{3100, nil},
		{41, func() { ss.applyRestart(appID(start)) }},
		{5000, func() { ss.applyDiagram(bumpThreads(start, 16)) }},
		{2500, nil},
	}
	for _, s := range steps {
		ss.sim.Advance(s.ms)
		if s.edit != nil {
			s.edit()
		}
	}
}

func TestSeekReplaysLiveEditsExactly(t *testing.T) {
	sc := scenarios.Get("opening-night")
	ss := &session{seed: 7, sc: sc, speed: 1}
	runLive(t, ss, sc.Diagram)
	end := ss.sim.Now()
	want := ss.sim.RunSnapshot()
	wantStats := ss.sim.Snapshot()

	for _, back := range []float64{0, 1234} { // to the very end, then a little way back and forward again
		target := end - back
		if err := ss.seek(target); err != nil {
			t.Fatalf("seek: %v", err)
		}
		if ss.sim.Now() != target {
			t.Fatalf("landed at %v, want %v", ss.sim.Now(), target)
		}
		if back == 0 {
			if got := ss.sim.RunSnapshot(); !reflect.DeepEqual(got, want) {
				t.Fatalf("seek to the end differs from the live run:\nlive %+v\nseek %+v", want, got)
			}
			if got := ss.sim.Snapshot(); !reflect.DeepEqual(got.Global, wantStats.Global) || !reflect.DeepEqual(got.Nodes, wantStats.Nodes) {
				t.Fatal("per-node stats differ after seeking to the end")
			}
		}
	}
	// a continuous run with the same edits at the same simulated times must agree with a seek to any moment
	ref := &session{seed: 7, sc: sc, speed: 1}
	runLive(t, ref, sc.Diagram)
	if err := ss.seek(9000); err != nil {
		t.Fatal(err)
	}
	mid := ss.sim.RunSnapshot()
	replay := engine.New(sc.Diagram, sc.Options(7))
	replay.Advance(4130)
	replay.Advance(777)
	replay.Sync(bumpThreads(sc.Diagram, 2))
	replay.Advance(9000 - replay.Now())
	if got := replay.RunSnapshot(); !reflect.DeepEqual(got, mid) {
		t.Fatalf("seek(9 s) differs from a hand-built replay:\nseek %+v\nhand %+v", mid, got)
	}
}

func TestSeekRefusesWhenThereIsNothingToRewindOrHistoryWasDropped(t *testing.T) {
	ss := &session{seed: 1, speed: 1, diagram: scenarios.Get("hello-world").Solution}
	if err := ss.seek(1000); err == nil {
		t.Fatal("seeking before any run must say so")
	}
	ss.build()
	ss.sim.Advance(2000)
	if err := ss.seek(5000); err == nil {
		t.Fatal("cannot seek past the present")
	}
	for i := 0; i < maxEditLog+10; i++ {
		ss.applyDiagram(bumpThreads(ss.diagram, float64(1+i%8)))
	}
	if len(ss.edits) > maxEditLog {
		t.Fatalf("the edit log grew to %d, cap is %d", len(ss.edits), maxEditLog)
	}
	if err := ss.seek(1000); err == nil {
		t.Fatal("once the oldest edits were dropped, replay must be refused rather than approximated")
	}
}

func TestSeekDropsTheFutureSoNewEditsStartFromHere(t *testing.T) {
	sc := scenarios.Get("opening-night")
	ss := &session{seed: 7, sc: sc, speed: 1}
	runLive(t, ss, sc.Diagram)
	if err := ss.seek(5000); err != nil {
		t.Fatal(err)
	}
	for _, e := range ss.edits {
		if e.t > 5000 {
			t.Fatalf("edit at %v survived a rewind to 5000", e.t)
		}
	}
}

func TestSeekMessageAnswersPausedAtThatTimeWithTheDesignThenAndErrorsBeforeARun(t *testing.T) {
	ts := newTestServer(t)
	c, ctx := dial(t, ts)
	sc := scenarios.Get("opening-night")
	readType(t, c, ctx, "frame")
	sendMsg(t, c, ctx, clientMsg{Type: "seek", T: 1})
	readType(t, c, ctx, "error")

	sendMsg(t, c, ctx, clientMsg{Type: "init", Diagram: &sc.Diagram})
	readType(t, c, ctx, "frame")
	for i := 0; i < 80; i++ { // 8 simulated seconds
		sendMsg(t, c, ctx, clientMsg{Type: "step"})
	}
	bumped := bumpThreads(sc.Diagram, 3)
	sendMsg(t, c, ctx, clientMsg{Type: "diagram", Diagram: &bumped})
	for i := 0; i < 20; i++ {
		sendMsg(t, c, ctx, clientMsg{Type: "step"})
	}
	sendMsg(t, c, ctx, clientMsg{Type: "seek", T: 5})
	var f frameMsg
	for {
		var m frameMsg
		_ = json.Unmarshal(readType(t, c, ctx, "frame"), &m)
		if m.Diagram != nil {
			f = m
			break
		}
	}
	if f.Status != "paused" || f.Stats == nil || f.Stats.Global.Now < 4.95 || f.Stats.Global.Now > 5.05 {
		t.Fatalf("expected paused at 5 s, got %+v", f.Stats.Global.Now)
	}
	for _, n := range f.Diagram.Nodes {
		if n.Kind == engine.KAppServer && n.Params["threads"] == 3.0 {
			t.Fatal("at 5 s the thread edit (made at 8 s) had not happened yet")
		}
	}
}

func TestResendingTheSameDesignIsNotAnEdit(t *testing.T) {
	sc := scenarios.Get("opening-night")
	ss := &session{seed: 7, sc: sc, speed: 1, diagram: sc.Diagram}
	ss.build()
	ss.sim.Advance(1000)
	ss.applyDiagram(sc.Diagram)
	if len(ss.edits) != 0 {
		t.Fatalf("the client echoes the design back after a seek; that must not grow the log: %d", len(ss.edits))
	}
	ss.applyDiagram(bumpThreads(sc.Diagram, 5))
	if len(ss.edits) != 1 {
		t.Fatal("a real change must still be logged")
	}
}
