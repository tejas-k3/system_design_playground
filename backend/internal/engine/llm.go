package engine

// An LLM server: a few GPUs, each running a batch of sequences. The model is deliberately small enough to check by
// hand (see llm_test.go):
//
//   - A sequence is admitted to a GPU when it has a free place in the batch (batchSize) and its tokens fit in the
//     GPU's KV cache (promptTokens + outputTokens each, reserved for the whole life of the sequence).
//   - Admitted sequences are prefilled one after another, promptTokens / prefillTokPerSec seconds each, and nothing
//     else on that GPU moves meanwhile.
//   - Then every decode step gives one token to every sequence in the batch and takes
//     decodeBaseMs + decodePerSeqMs x (sequences in the batch) milliseconds. The first token of a sequence is out
//     at the end of the first step after its prefill (time to first token); the answer is done after outputTokens steps.
//   - Between steps a GPU admits more sequences (continuous batching) from the queue they all share.

type llmSeq struct {
	arrived     float64
	prompt, out int
	done        int
	first       float64 // when its first token came out; 0 until then
	counted     bool
	cb          func(Result)
	n           *node
}

type llmGPU struct {
	active []*llmSeq
	used   int // KV cache tokens reserved
	busy   bool
}

type llmState struct {
	gpus   []*llmGPU
	queue  []*llmSeq
	tokens int     // output tokens produced since the start
	steps  int     // decode steps taken
	seqSum int     // sequences summed over the steps, for the average batch
	maxB   int     // the largest batch seen
	busyMs float64 // GPU time spent, over all GPUs
}

func (s *Sim) arriveLLM(n *node, c *call, cb func(Result)) {
	if n.llm == nil {
		n.llm = &llmState{}
		for i := 0; i < int(n.f("gpus")); i++ {
			n.llm.gpus = append(n.llm.gpus, &llmGPU{})
		}
	}
	st := n.llm
	if len(st.queue) >= int(n.f("queueCap")) {
		s.failAt(n, RQueueFull, cb)
		return
	}
	// a request's size is the node's typical size, spread by up to tokenSpreadPct either way (a hash of the request,
	// not a random draw, so the same requests are big in every run)
	spread := func(base float64, salt uint32) int {
		pct := n.f("tokenSpreadPct")
		f := 1.0
		if pct > 0 {
			f = 1 + (float64(hash32(uint32(c.rid)*2654435761+salt)%2001)/1000-1)*pct/100
		}
		return max(1, int(base*f))
	}
	prompt := spread(n.f("promptTokens"), 3)
	if c.tok > 0 { // the request says how long its prompt is
		prompt = int(c.tok)
	}
	sq := &llmSeq{arrived: s.now, prompt: prompt, out: spread(n.f("outputTokens"), 5), counted: c.counted, cb: cb, n: n}
	st.queue = append(st.queue, sq)
	for _, g := range st.gpus {
		s.llmKick(n, g)
	}
}

// llmKick starts a GPU's next round if it is idle: admit what fits, prefill it, then one decode step.
func (s *Sim) llmKick(n *node, g *llmGPU) {
	st := n.llm
	if g.busy || n.down {
		return
	}
	var prefillMs float64
	for len(st.queue) > 0 && len(g.active) < int(n.f("batchSize")) {
		sq := st.queue[0]
		if g.used+sq.prompt+sq.out > int(n.f("kvCacheTokens")) && len(g.active) > 0 {
			break // wait for room in the cache
		}
		st.queue = st.queue[1:]
		g.active = append(g.active, sq)
		g.used += sq.prompt + sq.out
		prefillMs += float64(sq.prompt) / n.f("prefillTokPerSec") * 1000
	}
	if len(g.active) == 0 {
		return
	}
	b := len(g.active)
	stepMs := n.f("decodeBaseMs") + n.f("decodePerSeqMs")*float64(b)
	g.busy = true
	ep := n.epoch
	s.after((prefillMs+stepMs)*n.slow, func() {
		if n.epoch != ep || n.removed {
			return
		}
		g.busy = false
		st.steps++
		st.seqSum += b
		st.maxB = max(st.maxB, b)
		st.busyMs += (prefillMs + stepMs) * n.slow
		keep := g.active[:0]
		for _, sq := range g.active {
			sq.done++
			st.tokens++
			if sq.first == 0 {
				sq.first = s.now
			}
			if sq.done >= sq.out {
				g.used -= sq.prompt + sq.out
				s.llmFinish(n, sq)
				continue
			}
			keep = append(keep, sq)
		}
		g.active = keep
		for _, other := range st.gpus { // a finished sequence frees room for the queue on any GPU
			s.llmKick(n, other)
		}
	})
}

func (s *Sim) llmFinish(n *node, sq *llmSeq) {
	if sq.counted {
		s.ttftHist.Add(sq.first - sq.arrived)
		if sq.out > 1 {
			s.itlHist.Add((s.now - sq.first) / float64(sq.out-1))
		}
	}
	s.okAt(n, Result{Ver: -1, Done: true}, sq.cb)
}

// llmCrashed drops everything the server held: its sequences and its queue fail.
func (s *Sim) llmCrashed(n *node) {
	if n.llm == nil {
		return
	}
	st := n.llm
	for _, g := range st.gpus {
		for _, sq := range g.active {
			s.failAt(n, RNodeDown, sq.cb)
		}
		g.active, g.used, g.busy = nil, 0, false
	}
	for _, sq := range st.queue {
		s.failAt(n, RNodeDown, sq.cb)
	}
	st.queue = nil
}
