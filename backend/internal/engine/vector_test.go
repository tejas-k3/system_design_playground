package engine

import (
	"math"
	"testing"
)

// A vector store searches an index of embeddings. The model, small enough to check by hand:
//   - a search looks at efSearch candidates: it takes searchBaseMs + efSearch x perCandidateMs;
//   - recall (the share of the true nearest neighbours it finds) is 1 - 0.5^(efSearch / 32): 50% at ef 32, 75% at 64,
//     94% at 128, 99.6% at 256 (each doubling halves what is still missed);
//   - memory is vectors x dims x 4 bytes, times 1.5 for the graph links.
func vectorRun(vp Params) (*Sim, RunStats) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0, "readPct": 100.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 5000.0, "retries": 0.0})
	b.node("a", KAppServer, Params{"threads": 64.0})
	p := Params{"searchBaseMs": 4.0, "perCandidateMs": 0.05, "connections": 64.0}
	for k, v := range vp {
		p[k] = v
	}
	b.node("v", KVectorStore, p)
	b.edge("u", "a").edge("a", "v")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 3})
	s.Advance(40000)
	return s, s.RunSnapshot()
}

func TestSearchLatencyRisesWithEfAndRecallFollowsTheFormula(t *testing.T) {
	var prevMs, prevRecall float64
	for _, ef := range []float64{16, 32, 64, 128, 256, 512} {
		s, r := vectorRun(Params{"efSearch": ef})
		ex := s.Snapshot().Nodes["v"].Extra
		wantMs := 4 + ef*0.05
		wantRecall := 1 - math.Pow(0.5, ef/32)
		t.Logf("ef %3.0f: search %.2f ms (hand %.2f), recall %.3f (formula %.3f), request mean %.2f ms", ef, ex["searchMs"], wantMs, ex["recall"], wantRecall, r.Mean)
		if math.Abs(ex["searchMs"]-wantMs) > 0.01 || math.Abs(ex["recall"]-wantRecall) > 0.001 {
			t.Errorf("ef %.0f: %.2f ms / %.3f, expected %.2f / %.3f", ef, ex["searchMs"], ex["recall"], wantMs, wantRecall)
		}
		if r.Mean <= prevMs && prevMs > 0 {
			t.Errorf("latency must rise with efSearch: %.2f after %.2f", r.Mean, prevMs)
		}
		if ex["recall"] <= prevRecall {
			t.Errorf("recall must rise with efSearch")
		}
		prevMs, prevRecall = r.Mean, ex["recall"]
	}
}

func TestAnIndexNeedsMemoryForEveryVector(t *testing.T) {
	s, _ := vectorRun(Params{"vectors": 10.0, "dims": 1536.0}) // 10 million vectors
	got := s.Snapshot().Nodes["v"].Extra["memoryGB"]
	want := 10e6 * 1536 * 4 * 1.5 / 1e9
	t.Logf("10 million vectors of 1,536 dimensions: %.1f GB (hand: %.1f)", got, want)
	if math.Abs(got-want) > 0.1 {
		t.Errorf("memory %.1f GB, expected %.1f", got, want)
	}
}

// Retrieval then generation: an app server asks the vector store first and the model second, so a request takes the
// search time plus the model's, and the model's own time to first token is unchanged.
func TestARetrievalChainAddsTheSearchToTheAnswer(t *testing.T) {
	run := func(withStore bool) (RunStats, float64) {
		b := &builder{}
		b.node("u", KClient, Params{"rps": 1.0, "readPct": 100.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 60000.0, "retries": 0.0})
		b.node("a", KAppServer, Params{"threads": 64.0})
		b.node("m", KLLMServer, Params{"promptTokens": 100.0, "outputTokens": 5.0})
		b.edge("u", "a").edge("a", "m")
		if withStore {
			b.node("v", KVectorStore, Params{"efSearch": 256.0}) // 4 + 256 x 0.05 = 16.8 ms
			b.edge("a", "v")
		}
		d, _ := Sanitize(b.d)
		s := New(d, Options{Seed: 4})
		s.Advance(120000)
		return s.RunSnapshot(), s.ttftHist.quantile(0.5)
	}
	plain, t0 := run(false)
	rag, t1 := run(true)
	t.Logf("answer mean %.1f ms without retrieval, %.1f ms with (search 16.8 ms plus a hop); model first token %.1f vs %.1f ms", plain.Mean, rag.Mean, t0, t1)
	if d := rag.Mean - plain.Mean; d < 16.8 || d > 30 {
		t.Errorf("retrieval should add the 16.8 ms search and a network hop: %.1f", d)
	}
	if math.Abs(t1-t0) > 3 {
		t.Errorf("the model's own first token is unchanged: %.1f vs %.1f", t1, t0)
	}
}
