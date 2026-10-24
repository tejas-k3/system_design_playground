package engine

import (
	"math"
	"testing"
)

// The LLM server model, small enough to check by hand. One GPU holds up to batchSize sequences. A new sequence is
// first prefilled (its prompt is read: promptTokens / prefillTokPerSec seconds, during which nothing else on that
// GPU moves), then every decode step gives one new token to every sequence in the batch and takes
// decodeBaseMs + decodePerSeqMs x batch milliseconds.
func llmRun(llm Params, rps float64, sec float64) (*Sim, RunStats) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 0.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 600000.0, "retries": 0.0})
	b.node("a", KAppServer, Params{"threads": 4096.0, "queueCap": 5000.0})
	lp := Params{"gpus": 1.0, "batchSize": 8.0, "promptTokens": 1000.0, "outputTokens": 100.0, "prefillTokPerSec": 4000.0,
		"decodeBaseMs": 20.0, "decodePerSeqMs": 0.5, "kvCacheTokens": 1000000.0, "queueCap": 5000.0}
	for k, v := range llm {
		lp[k] = v
	}
	b.node("m", KLLMServer, lp)
	b.edge("u", "a").edge("a", "m")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 5})
	s.Advance(sec * 1000)
	return s, s.RunSnapshot()
}

// One request alone: 100 prompt tokens at 4,000 a second = 25 ms of prefill; each of the 5 decode steps takes
// 20 + 0.5 x 1 = 20.5 ms. The first token is out after 25 + 20.5 = 45.5 ms, and the gap between tokens is 20.5 ms
// (a request a second keeps the server busy 13% of the time, so most requests find it empty: compare medians).
func TestOneRequestAloneFollowsTheHandCalculation(t *testing.T) {
	s, _ := llmRun(Params{"promptTokens": 100.0, "outputTokens": 5.0}, 1, 300)
	ttft, itl := s.ttftHist.Quantile(0.5), s.itlHist.Quantile(0.5)
	t.Logf("TTFT p50 %.1f ms (hand: 45.5), inter-token gap p50 %.1f ms (hand: 20.5)", ttft, itl)
	if math.Abs(ttft-45.5) > 3 {
		t.Errorf("time to first token %.1f, expected 45.5", ttft)
	}
	if math.Abs(itl-20.5) > 1.5 {
		t.Errorf("gap between tokens %.1f, expected 20.5", itl)
	}
}

// With far more requests than the GPU can take, it runs full batches. Decode throughput of a batch of b is
// b / (decodeBaseMs + decodePerSeqMs x b) tokens a millisecond, but each new sequence costs prefill time too, so the
// whole-server rate is output tokens per second = 1 / (prefill per request / ... ) -- measured here against the model:
// per request, the GPU spends 250 ms prefilling and 100 steps x t(b)/b ms of its time on decode.
func TestThroughputGrowsWithBatchSizeThenFlattens(t *testing.T) {
	rate := func(b float64) float64 {
		s, _ := llmRun(Params{"batchSize": b}, 50, 60) // the first minute fills the batches: measure the next two
		t0 := s.nodes["m"].llm.tokens
		s.Advance(120000)
		return float64(s.nodes["m"].llm.tokens-t0) / 120
	}
	model := func(b float64) float64 {
		step := 20 + 0.5*b             // ms per decode step with b sequences
		perRequest := 250 + 100*step/b // ms of GPU time per finished request: its prefill and its share of 100 steps
		return 1000 / perRequest * 100 // output tokens per second
	}
	var prev float64
	for _, b := range []float64{1, 4, 16, 64} {
		got, want := rate(b), model(b)
		t.Logf("batch %2.0f: %.0f tokens/s measured, %.0f by the model", b, got, want)
		if math.Abs(got-want)/want > 0.10 {
			t.Errorf("batch %.0f: %.0f tokens/s is more than 10%% from the model's %.0f", b, got, want)
		}
		if got < prev {
			t.Errorf("a bigger batch never lowers throughput: %.0f after %.0f", got, prev)
		}
		prev = got
	}
}

// The KV cache holds the tokens of every sequence in the batch. 3,000 tokens of cache and 1,100 per sequence
// (1,000 prompt + 100 answer) fit two sequences, not three, whatever the batch size allows.
func TestTheKVCacheCapsHowManySequencesRunAtOnce(t *testing.T) {
	s, _ := llmRun(Params{"batchSize": 16.0, "kvCacheTokens": 3000.0}, 50, 40)
	ex := s.Snapshot().Nodes["m"].Extra
	t.Logf("average batch %.2f, largest %.0f with room for 2", ex["avgBatch"], ex["maxBatch"])
	if ex["maxBatch"] != 2 {
		t.Errorf("3,000 / 1,100 tokens fit 2 sequences: %v", ex["maxBatch"])
	}
}

func TestMoreGPUsServeMore(t *testing.T) {
	rate := func(g float64) float64 {
		s, _ := llmRun(Params{"gpus": g}, 50, 60)
		t0 := s.nodes["m"].llm.tokens
		s.Advance(120000)
		return float64(s.nodes["m"].llm.tokens-t0) / 120
	}
	a, b := rate(1), rate(4)
	t.Logf("1 GPU %.0f tokens/s, 4 GPUs %.0f", a, b)
	if b < 3.5*a || b > 4.2*a {
		t.Errorf("four GPUs should give about four times the throughput: %.0f vs %.0f", b, a)
	}
}

// Time to first token is the wait in the queue plus the prompt reading and one step: a server that is overloaded
// keeps its queue long and the first token comes late, while a lightly loaded one answers in about 45 ms.
func TestTimeToFirstTokenGrowsWithTheQueue(t *testing.T) {
	small := Params{"promptTokens": 100.0, "outputTokens": 5.0, "batchSize": 1.0}
	light, _ := llmRun(small, 1, 120)
	heavy, _ := llmRun(small, 12, 120) // 12 requests a second, one at a time at 127.5 ms each: about 7.8 a second possible
	a, b := light.ttftHist.Quantile(0.95), heavy.ttftHist.Quantile(0.95)
	t.Logf("TTFT p95: %.0f ms when idle, %.0f ms at 12 requests a second against a capacity of 7.8", a, b)
	if a > 400 || b < 5*a {
		t.Errorf("an overloaded server makes the first token wait in the queue: %.0f vs %.0f", a, b)
	}
}
