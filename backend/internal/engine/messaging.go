package engine

import "math"

// ---- queue + workers ----------------------------------------------------------------

func (s *Sim) arriveQueue(n *node, c *call, cb func(Result)) {
	if _, drop := s.eventRepeat(n, c); drop {
		s.after(0.2, func() { s.okAt(n, Result{Ver: -1, Done: true}, cb) })
		return
	}
	if !c.write {
		s.failAt(n, RNoRoute, cb)
		return
	}
	if c.lg != nil && !c.claim && s.reqBytes(c.lg) > n.f("maxMessageKB")*1000 { // sizes are 0 (not modelled) unless the users set them; a claim check carries a pointer
		s.failAt(n, RMessageTooLarge, cb)
		return
	}
	if s.queued(n)+n.delayed >= int(n.f("capacity")) {
		s.failAt(n, RQueueFull, cb)
		return
	}
	j := &job{rid: c.rid, key: c.key, wl: c.wl, idem: c.idem, counted: c.counted, enq: s.now, high: c.high}
	if j.counted {
		s.jobEnqueued++
	}
	if d := n.f("delayMs"); d > 0 {
		n.delayed++
		s.after(d, func() {
			n.delayed--
			s.requeue(n, j)
			s.dispatch(n)
		})
	} else {
		s.requeue(n, j)
	}
	s.after(0.2, func() {
		s.okAt(n, Result{Ver: -1, Done: true}, cb)
		s.dispatch(n)
	})
}

// queued is the number of jobs waiting for a worker.
func (s *Sim) queued(q *node) int {
	if q.kind == KLog {
		n := 0
		for _, l := range q.lanes {
			n += len(l.jobs) - l.head
		}
		return n
	}
	if q.kind == KTopic {
		n := 0
		for _, e := range q.out {
			n += len(e.subJobs) - e.subHead
		}
		return n
	}
	return len(q.jobs) - q.jh + len(q.hjobs) - q.hh
}

// requeue puts a job at the back of its line: the urgent line of a priority queue, or the ordinary one.
func (s *Sim) requeue(q *node, j *job) {
	if j.sub != nil {
		j.sub.subJobs = append(j.sub.subJobs, j)
		return
	}
	if j.high && q.b("priorityQueue") {
		q.hjobs = append(q.hjobs, j)
		return
	}
	q.jobs = append(q.jobs, j)
}

func (s *Sim) dispatch(q *node) {
	if q.kind == KTopic {
		s.dispatchTopic(q)
		return
	}
	if q.kind == KLog {
		s.dispatchLog(q)
		return
	}
	for s.queued(q) > 0 && !q.down {
		var w *node
		for _, e := range q.out {
			t := e.to
			if e.removed || t.kind != KWorker || t.down || t.removed || t.busy >= s.capacity(t) {
				continue
			}
			if w == nil || t.busy < w.busy {
				w = t
			}
		}
		if w == nil {
			break
		}
		var j *job
		if q.hh < len(q.hjobs) {
			j = q.hjobs[q.hh]
			q.hjobs[q.hh] = nil
			q.hh++
		} else {
			j = q.jobs[q.jh]
			q.jobs[q.jh] = nil
			q.jh++
		}
		s.runJob(q, w, j)
	}
	if q.hh > 256 && q.hh*2 > len(q.hjobs) {
		q.hjobs = append([]*job(nil), q.hjobs[q.hh:]...)
		q.hh = 0
	}
	if q.jh > 256 && q.jh*2 > len(q.jobs) {
		q.jobs = append([]*job(nil), q.jobs[q.jh:]...)
		q.jh = 0
	}
}

func (s *Sim) runJob(q, w *node, j *job) {
	s.markBusy(w)
	w.busy++
	j.attempts++
	j.finished = false
	ep := w.epoch
	if vis := q.f("visibilityTimeoutMs"); vis > 0 && !j.ghost {
		s.after(vis, func() {
			if j.finished || q.removed {
				return
			}
			// not acknowledged in time: the queue offers the job again; this copy still runs, and so does the new one
			if j.counted {
				s.jobRedelivered++
			}
			s.requeue(q, &job{rid: j.rid, key: j.key, wl: j.wl, idem: j.idem, counted: j.counted, enq: j.enq, attempts: j.attempts, ghost: true, high: j.high})
			s.dispatch(q)
		})
	}
	fin := func(res Result) {
		s.markBusy(w)
		w.busy--
		if j.ghost {
			if res.OK {
				w.c.ok++
				w.c.done++
			} else {
				w.c.err++
				w.c.reason[res.Reason]++
			}
			s.dispatch(q)
			return
		}
		j.finished = true
		if res.OK && j.lane != nil {
			j.lane.running = false
			s.orderCheck(q, j)
		}
		if res.OK {
			w.c.ok++
			w.c.done++
			if j.counted {
				s.jobCompleted++
				s.jobHist.Add(s.now - j.enq)
				if j.high {
					s.jobHistHigh.Add(s.now - j.enq)
				}
				s.jobsW.Push(s.now, true, s.now-j.enq)
			}
		} else {
			w.c.err++
			w.c.reason[res.Reason]++
			if j.attempts < int(q.f("maxAttempts")) {
				if j.counted {
					s.jobRetried++
				}
				s.after(100*math.Pow(2, float64(j.attempts-1)), func() {
					if j.lane != nil { // retried in place: the partition waited, so later messages did not overtake it
						j.lane.jobs = append([]*job{j}, j.lane.jobs[j.lane.head:]...)
						j.lane.head = 0
						j.lane.running = false
					} else {
						s.requeue(q, j)
					}
					s.dispatch(q)
				})
			} else if j.lane != nil {
				j.lane.running = false
				if j.counted {
					if q.b("deadLetter") {
						s.jobDead++
					} else {
						s.jobLost++
					}
				}
			} else if j.counted {
				if q.b("deadLetter") {
					s.jobDead++
				} else {
					s.jobLost++
				}
			}
		}
		s.dispatch(q)
	}
	w.c.arr++
	s.after(s.service(w, w.f("processMs"))*w.slow, func() {
		if w.epoch != ep || w.removed {
			fin(fail(RNodeDown))
			return
		}
		if pct := w.f("poisonPct"); pct > 0 && float64(Hash32(uint32(j.rid)*2654435761+211)%100) < pct {
			fin(fail(RPoisonJob)) // a malformed job fails the same way every time
			return
		}
		c := &call{key: j.key, wl: j.wl, rid: j.rid, idem: j.idem, counted: j.counted, write: true, ver: -1, job: j}
		step := func() {
			e := s.primaryEdge(w)
			if e == nil {
				fin(Result{OK: true})
				return
			}
			s.callCmd(w, e, c, cmdWrite, 0, func(res Result) {
				if !res.OK {
					fin(res)
					return
				}
				s.invalidate(w, c, res.Ver, func() { fin(res) })
			})
		}
		if ee := s.first(w, KExternalAPI); ee != nil {
			s.callCmd(w, ee, c, cmdExternal, 0, func(res Result) {
				if !res.OK {
					fin(res)
					return
				}
				step()
			})
			return
		}
		step()
	})
}

// ---- topic (publish/subscribe) -------------------------------------------------------

// arriveTopic gives every subscriber (each outgoing edge to a worker) its own copy of the message. The publisher
// is acknowledged at once: it never waits for, or hears about, a slow subscriber. A subscriber whose backlog is
// full loses its copy (counted as a lost delivery).
func (s *Sim) arriveTopic(n *node, c *call, cb func(Result)) {
	if _, drop := s.eventRepeat(n, c); drop {
		s.after(0.2, func() { s.okAt(n, Result{Ver: -1, Done: true}, cb) })
		return
	}
	if !c.write {
		s.failAt(n, RNoRoute, cb)
		return
	}
	for _, e := range n.out {
		if e.removed || e.to.kind != KWorker || e.to.removed {
			continue
		}
		j := &job{rid: c.rid, key: c.key, wl: c.wl, idem: c.idem, counted: c.counted, enq: s.now, sub: e}
		if j.counted {
			s.jobEnqueued++
		}
		if len(e.subJobs)-e.subHead >= int(n.f("capacity")) {
			if j.counted {
				s.jobLost++
			}
			continue
		}
		e.subJobs = append(e.subJobs, j)
	}
	s.after(0.2, func() {
		s.okAt(n, Result{Ver: -1, Done: true}, cb)
		s.dispatch(n)
	})
}

// dispatchTopic hands each subscriber's deliveries to its own worker, in order, as the worker has room.
func (s *Sim) dispatchTopic(q *node) {
	for _, e := range q.out {
		w := e.to
		if e.removed || w.kind != KWorker || w.down || w.removed {
			continue
		}
		for e.subHead < len(e.subJobs) && w.busy < s.capacity(w) && !q.down {
			j := e.subJobs[e.subHead]
			e.subJobs[e.subHead] = nil
			e.subHead++
			s.runJob(q, w, j)
		}
		if e.subHead > 256 && e.subHead*2 > len(e.subJobs) {
			e.subJobs = append([]*job(nil), e.subJobs[e.subHead:]...)
			e.subHead = 0
		}
	}
}

// ---- partitioned log -----------------------------------------------------------------

// lane is one partition as seen by one consumer group: an ordered line of messages that one member works through
// one at a time.
type lane struct {
	part, group int
	jobs        []*job
	head        int
	running     bool
}

// logGroups lists the consumer groups reading a log (the distinct group numbers of its workers), smallest first.
func (s *Sim) logGroups(q *node) []int {
	var gs []int
	seen := map[int]bool{}
	for _, e := range q.out {
		if e.removed || e.to.kind != KWorker || e.to.removed {
			continue
		}
		g := int(e.to.f("group"))
		if !seen[g] {
			seen[g] = true
			gs = append(gs, g)
		}
	}
	for i := 1; i < len(gs); i++ { // tiny insertion sort: determinism without importing sort here
		for k := i; k > 0 && gs[k] < gs[k-1]; k-- {
			gs[k], gs[k-1] = gs[k-1], gs[k]
		}
	}
	return gs
}

// laneOwner is the member of the lane's group that reads its partition: the group's running members take the
// partitions in turn.
func (s *Sim) laneOwner(q *node, l *lane) *node {
	var members []*node
	for _, e := range q.out {
		if e.removed || e.to.kind != KWorker || e.to.removed || e.to.down || int(e.to.f("group")) != l.group {
			continue
		}
		members = append(members, e.to)
	}
	if len(members) == 0 {
		return nil
	}
	return members[l.part%len(members)]
}

// workerLag is the number of messages waiting in the partitions a worker reads.
func (s *Sim) workerLag(q, w *node) int {
	n := 0
	for _, l := range q.lanes {
		if s.laneOwner(q, l) == w {
			n += len(l.jobs) - l.head
		}
	}
	return n
}

func (s *Sim) arriveLog(n *node, c *call, cb func(Result)) {
	if _, drop := s.eventRepeat(n, c); drop {
		s.after(0.2, func() { s.okAt(n, Result{Ver: -1, Done: true}, cb) })
		return
	}
	if !c.write {
		s.failAt(n, RNoRoute, cb)
		return
	}
	parts := int(n.f("partitions"))
	p := int(Hash32(uint32(c.key)*2654435761+7) % uint32(parts))
	if n.s("partitionBy") == "roundRobin" {
		p = n.rrNext % parts
		n.rrNext++
	}
	s.logSeq++
	seq := s.logSeq
	if n.laneIdx == nil {
		n.laneIdx = map[[2]int]*lane{}
	}
	for _, g := range s.logGroups(n) {
		l := n.laneIdx[[2]int{p, g}]
		if l == nil {
			l = &lane{part: p, group: g}
			n.laneIdx[[2]int{p, g}] = l
			n.lanes = append(n.lanes, l)
		}
		j := &job{rid: c.rid, key: c.key, wl: c.wl, idem: c.idem, counted: c.counted, enq: s.now, lane: l, seq: seq}
		if j.counted {
			s.jobEnqueued++
		}
		if len(l.jobs)-l.head >= int(n.f("capacity")) {
			if j.counted {
				s.jobLost++
			}
			continue
		}
		l.jobs = append(l.jobs, j)
	}
	ack := 0.2
	if n.s("ackMode") == "all" { // the acknowledgement waits until the followers have copied the message
		ack += n.f("replicationLagMs")
	}
	s.after(ack, func() {
		s.okAt(n, Result{Ver: -1, Done: true}, cb)
		s.dispatch(n)
	})
}

// logLoseUnreplicated is a log's leader dying with ackMode leader: messages it acknowledged less than the copy lag ago
// had not reached the followers and are gone, though their producers were told they were safe.
func (s *Sim) logLoseUnreplicated(q *node) {
	cut := s.now - q.f("replicationLagMs")
	for _, l := range q.lanes {
		keep := l.head
		for i := l.head; i < len(l.jobs); i++ {
			j := l.jobs[i]
			if j.enq > cut {
				if j.counted {
					s.runLost++
				}
				continue
			}
			l.jobs[keep] = j
			keep++
		}
		l.jobs = l.jobs[:keep]
	}
}

// logRebalanceCheck notices a change in who is in a consumer group (a member crashed, came back, was added or
// removed) and, with rebalanceSec set, stops the whole group reading for that long while the partitions are handed out.
func (s *Sim) logRebalanceCheck(q *node) {
	if q.grpSig == nil {
		q.grpSig, q.grpPause = map[int]string{}, map[int]float64{}
	}
	members := map[int]string{}
	for _, e := range q.out {
		if e.removed || e.to.kind != KWorker || e.to.removed || e.to.down {
			continue
		}
		g := int(e.to.f("group"))
		members[g] += e.to.id + ","
	}
	for _, g := range s.logGroups(q) {
		sig := members[g]
		old, seen := q.grpSig[g]
		q.grpSig[g] = sig
		if !seen || old == sig {
			continue
		}
		q.rebalances++
		if sec := q.f("rebalanceSec"); sec > 0 {
			until := s.now + sec*1000
			q.grpPause[g] = until
			s.at(until, func() { s.dispatch(q) })
		}
	}
}

func (s *Sim) dispatchLog(q *node) {
	s.logRebalanceCheck(q)
	for _, l := range q.lanes {
		if l.running || l.head >= len(l.jobs) || q.down || q.grpPause[l.group] > s.now {
			continue
		}
		if keep := q.f("retentionSec") * 1000; keep > 0 { // messages older than the retention are gone
			for l.head < len(l.jobs) && s.now-l.jobs[l.head].enq > keep {
				if l.jobs[l.head].counted {
					s.jobLost++
				}
				l.jobs[l.head] = nil
				l.head++
			}
			if l.head >= len(l.jobs) {
				continue
			}
		}
		w := s.laneOwner(q, l)
		if w == nil || w.busy >= s.capacity(w) {
			continue
		}
		j := l.jobs[l.head]
		keep := q.f("retentionSec") * 1000
		if keep <= 0 { // nothing is kept once it has been read
			l.jobs[l.head] = nil
		}
		l.head++
		l.running = true
		s.runJob(q, w, j)
		cut := l.head
		if keep > 0 { // read messages stay until they are older than the retention, so a group can read them again
			cut = 0
			for cut < l.head && (l.jobs[cut] == nil || s.now-l.jobs[cut].enq > keep) {
				l.jobs[cut] = nil
				cut++
			}
		}
		if cut > 256 && cut*2 > len(l.jobs) {
			l.jobs = append([]*job(nil), l.jobs[cut:]...)
			l.head -= cut
		}
	}
}

// orderCheck counts a message that a group's consumer finished after a newer message of the same key.
func (s *Sim) orderCheck(q *node, j *job) {
	if q.lastSeq == nil {
		q.lastSeq = map[[2]int]int{}
	}
	k := [2]int{j.lane.group, j.key}
	if j.seq < q.lastSeq[k] {
		if j.counted {
			s.jobOrderViolations++
		}
		return
	}
	q.lastSeq[k] = j.seq
}

// logReplay makes every consumer group read again the messages the log still has from the last back seconds.
func (s *Sim) logReplay(q *node, back float64) {
	from := s.now - back*1000
	for _, l := range q.lanes {
		i := l.head
		for i > 0 && l.jobs[i-1] != nil && l.jobs[i-1].enq >= from {
			i--
		}
		s.jobRedelivered += l.head - i
		l.head = i
	}
	s.dispatch(q)
}
