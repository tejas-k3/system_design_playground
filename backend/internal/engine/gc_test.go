package engine

import (
	"reflect"
	"testing"
)

func gcDesign(pauseMs, everySec float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 60.0, "readPct": 100.0, "timeoutMs": 10000.0}).node("a", KAppServer, Params{"threads": 16.0, "gcPauseMs": pauseMs, "gcEverySec": everySec}).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestGCPausesAddTailLatencyButNotMedianLatency(t *testing.T) {
	off := run(gcDesign(0, 4), Options{Seed: 7}, 60).Snapshot().Global.Run
	on := run(gcDesign(400, 4), Options{Seed: 7}, 60).Snapshot().Global.Run
	if on.P99-off.P99 < 250 {
		t.Fatalf("400 ms stop-the-world pauses should add several hundred ms to p99: off %.0f on %.0f", off.P99, on.P99)
	}
	if on.P50 > off.P50*1.15 {
		t.Fatalf("pauses are rare, the median should barely move: off %.0f on %.0f", off.P50, on.P50)
	}
}

func TestNothingCompletesDuringAPause(t *testing.T) {
	quiet := func(d Diagram) int {
		s := run(d, Options{Seed: 7}, 60)
		n := 0
		for _, p := range s.Snapshot().Series {
			if p.T > 3 && p.OkRps == 0 {
				n++
			}
		}
		return n
	}
	if got := quiet(gcDesign(0, 4)); got != 0 {
		t.Fatalf("without GC no half-second bucket should be empty at 60 rps, found %d", got)
	}
	if got := quiet(gcDesign(1500, 6)); got == 0 {
		t.Fatal("a 1.5 s stop-the-world pause should leave at least one empty half-second of completions")
	}
}

func TestGCOffChangesNothingAndGCOnIsDeterministic(t *testing.T) {
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 60.0, "readPct": 100.0, "timeoutMs": 10000.0}).node("a", KAppServer, Params{"threads": 16.0}).node("db", KDatabase, nil).edge("u", "a").edge("a", "db")
	d0, _ := Sanitize(plain.d)
	a := run(d0, Options{Seed: 21}, 30).RunSnapshot()
	b := run(gcDesign(0, 4), Options{Seed: 21}, 30).RunSnapshot()
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("gcPauseMs 0 must be identical to a design that never mentions GC:\n%+v\n%+v", a, b)
	}
	x := run(gcDesign(300, 4), Options{Seed: 21}, 30).RunSnapshot()
	y := run(gcDesign(300, 4), Options{Seed: 21}, 30).RunSnapshot()
	if !reflect.DeepEqual(x, y) {
		t.Fatal("same seed, same GC schedule, same result")
	}
	z := run(gcDesign(300, 4), Options{Seed: 22}, 30).Snapshot().Events
	w := run(gcDesign(300, 4), Options{Seed: 21}, 30).Snapshot().Events
	if len(z) == 0 || reflect.DeepEqual(z, w) {
		t.Fatalf("different seeds should draw different pause times: %v vs %v", z, w)
	}
}

func TestGCPausesAreLoggedAtTheRightRate(t *testing.T) {
	ev := run(gcDesign(200, 5), Options{Seed: 13}, 300).Snapshot().Events
	n := 0
	for _, e := range ev {
		if e.Type == "gc" {
			n++
		}
	}
	// the event log keeps the last 200; mean gap 5 s over 300 s is ~60 pauses (Poisson: 60 +/- 8 sigma-ish)
	if n < 40 || n > 85 {
		t.Fatalf("expected ~60 pauses in 300 s at a mean of one per 5 s, got %d", n)
	}
}

func TestGCSettingsAreOffByDefault(t *testing.T) {
	var found int
	for _, p := range Spec(KAppServer).Params {
		if p.Key == "gcPauseMs" && p.Default == 0.0 && p.Min == 0 {
			found++
		}
		if p.Key == "gcEverySec" && p.Min >= 1 {
			found++
		}
	}
	if found != 2 {
		t.Fatal("app servers need gcPauseMs (default 0 = off) and gcEverySec")
	}
}
