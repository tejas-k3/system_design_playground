package engine

import (
	"math"
	"sort"
)

// Chaos: scheduled and manual crashes, slow-downs and network partitions.

// ChaosRecord is a chaos event that fired.
type ChaosRecord struct {
	T      float64 `json:"t"`
	Type   string  `json:"type"`
	NodeID string  `json:"nodeId"`
	Label  string  `json:"label"`
	Lost   int     `json:"lost,omitempty"` // promotion: acknowledged writes that did not survive
}

func (s *Sim) fireChaos(ev ChaosEvent) {
	if ev.Type == "configPush" {
		s.configPush(ev)
		return
	}
	if ev.Type == "linkDown" {
		s.linkDown(ev)
		return
	}
	if ev.Type == "badDeploy" || ev.Type == "rollingRestart" {
		s.changeEvent(ev)
		return
	}
	if ev.Type == "regionDown" { // a bigger correlated failure: everything in the region crashes together
		for _, n := range s.order {
			if n.region == ev.Region && n.kind != KClient && !n.removed && !n.down {
				s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: "crash", NodeID: n.id, Label: n.label})
				s.CrashNode(n.id, ev.DurationSec)
			}
		}
		return
	}
	if ev.Type == "zoneDown" { // a correlated failure: everything placed in the zone crashes together
		for _, n := range s.order {
			if n.zone == ev.Zone && n.kind != KClient && !n.removed && !n.down {
				s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: "crash", NodeID: n.id, Label: n.label})
				s.CrashNode(n.id, ev.DurationSec)
			}
		}
		return
	}
	var t *node
	if len(ev.NodeID) > 7 && ev.NodeID[:7] == "random:" {
		kind := ev.NodeID[7:]
		var c []*node
		for _, n := range s.order {
			if n.kind == kind && !n.down && !n.removed {
				c = append(c, n)
			}
		}
		if len(c) == 0 {
			return
		}
		t = c[int(s.chaosRng.Next()*float64(len(c)))]
	} else {
		t = s.nodes[ev.NodeID]
	}
	if t == nil || t.removed {
		return
	}
	s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: ev.Type, NodeID: t.id, Label: t.label})
	switch ev.Type {
	case "crash":
		s.CrashNode(t.id, ev.DownSec)
		return
	case "corrupt": // silent damage from now on: nothing fails, the data is just wrong until someone restores
		t.corruptAt = s.now
		s.logEvent("corrupt", t, 0)
		return
	case "restore":
		if t.kind == KDatabase {
			s.restore(t)
		}
		return
	case "replay": // a log's consumer groups read the last Factor seconds again
		if t.kind == KLog {
			s.logReplay(t, ev.Factor)
		}
		return
	case "addNode": // a shard router gets one more shard: a copy of its first
		if t.kind == KShardRouter {
			s.addShard(t)
		}
		return
	case "memberOmission", "memberLies":
		if t.kind == KCoordinator {
			s.coordMembersFault(t, ev)
		}
		return
	case "clusterNodeDown":
		if t.kind == KCoordinator {
			s.coordNodesDown(t, int(ev.Factor), ev.DurationSec*1000, ev.Mode)
		} else if t.kind == KKVStore {
			s.kvNodesDown(t, int(ev.Factor), ev.DurationSec*1000)
		}
		return
	case "leaderCut":
		if t.kind == KCoordinator {
			s.coordLeaderCut(t, ev.DurationSec*1000)
		}
		return
	case "leaderDown":
		if t.kind == KCoordinator {
			s.coordLeaderDown(t, ev.DurationSec*1000)
		}
		return
	case "kvNodeDown":
		if t.kind == KKVStore {
			s.kvNodesDown(t, int(ev.Factor), ev.DurationSec*1000)
		}
		return
	case "partition":
		s.partition(t, ev.DurationSec)
		return
	case "purge": // invalidate everything a cache or CDN holds: the next requests all miss
		if t.store != nil {
			s.logEvent("purge", t, t.store.Clear())
		}
		return
	}
	factor := math.Max(1, ev.Factor)
	s.logEvent("slow", t, 0)
	t.slow = factor
	t.slowUntil = s.now + ev.DurationSec*1000
	until := t.slowUntil
	// intermittent: slow for PulseSec, normal for PulseSec, ... (the last step to normal is the end below)
	for at, on := s.now+ev.PulseSec*1000, false; ev.PulseSec > 0 && at < until; at, on = at+ev.PulseSec*1000, !on {
		slow := factor
		if !on {
			slow = 1
		}
		s.at(at, func() {
			if t.slowUntil == until && s.now < until {
				t.slow = slow
			}
		})
	}
	s.at(until, func() {
		if s.now >= t.slowUntil && t.slowUntil == until {
			t.slow = 1
			s.logEvent("slowEnd", t, 0)
		}
	})
}

// partition cuts a node off the network: calls to it get no answer (callers only escape through their own
// timeouts, unlike a crash that refuses at once) until it heals, when the waiting calls are delivered.
func (s *Sim) partition(n *node, durationSec float64) {
	n.cut = true
	s.logEvent("partition", n, 0)
	s.after(durationSec*1000, func() {
		n.cut = false
		s.logEvent("heal", n, 0)
		held := n.held
		n.held = nil
		for _, deliver := range held {
			deliver()
		}
	})
}

// scheduleFailover promotes a replica once the failure has been noticed and the replica has taken over, unless
// the primary came back first (a blip shorter than the detection time is not a failover).
func (s *Sim) scheduleFailover(db *node) {
	ep := db.epoch
	s.after(db.f("detectSec")*1000, func() {
		if db.down && db.epoch == ep {
			s.logEvent("failoverDetected", db, 0)
		}
	})
	s.after((db.f("detectSec")+db.f("promoteSec"))*1000, func() {
		if !(db.down && db.epoch == ep) {
			return
		}
		zk := s.first(db, KCoordinator)
		if db.s("electionBy") != "coordinator" || zk == nil {
			s.promote(db)
			return
		}
		// the decision belongs to the coordination service: wait for it to have a leader and a majority
		var try func()
		try = func() {
			if !(db.down && db.epoch == ep) {
				return
			}
			s.coordInit(zk.to)
			if zk.to.coordLeader >= 0 && s.coordMajority(zk.to) {
				s.promote(db)
				return
			}
			s.after(100, try)
		}
		try()
	})
}

// promote makes the first healthy replica the primary. Writes the old primary acknowledged less than the replica's
// replication lag before it crashed never reached the replica, so they are gone: that is the price of asynchronous replication.
func (s *Sim) promote(db *node) {
	var r *node
	for _, n := range s.order {
		if n.kind == KReplica && !n.removed && !n.down && !n.promoted {
			r = n
			break
		}
	}
	if r == nil {
		return // nothing to promote: the database stays down
	}
	lost := s.truncateVersions(db.downAt - s.replicaLag(r))
	s.runLost += lost
	r.promoted = true
	db.successor = r
	s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: "promotion", NodeID: r.id, Label: r.label, Lost: lost})
	s.logEvent("promotion", r, lost)
}

// CrashNode takes a node down; if downSec > 0 it restarts automatically.
func (s *Sim) CrashNode(id string, downSec float64) {
	n := s.nodes[id]
	if n == nil || n.down {
		return
	}
	n.down, n.downAt = true, s.now
	s.logEvent("crash", n, 0)
	n.epoch++
	n.txl, n.txWaits = map[int]*txLock{}, map[int]int{} // a crash drops every lock
	s.dropSessions(n)
	n.authTokens = nil // remembered token answers live in memory
	n.coldUp = false   // a restarted function starts cold
	if n.kind == KAppServer || (n.kind == KRedis && n.s("persistence") != "aof") {
		s.dropLoginSessions(n) // a server's sessions, or a Redis without persistence, are memory only
	}
	s.failQueued(n, RNodeDown)
	s.closePools(n)
	s.dropConns(n)
	if n.kind == KRedis {
		s.loseRedisData(n)
	}
	if n.kind == KLog && n.s("ackMode") == "leader" {
		s.logLoseUnreplicated(n)
	}
	if n.kind == KDatabase && n.s("onFailure") == "promote" {
		s.scheduleFailover(n)
	}
	if n.kind == KStream {
		s.streamCrashed(n)
	}
	if n.kind == KLLMServer {
		s.llmCrashed(n)
	}
	if n.kind == KLog && n.f("leaderElectionSec") > 0 && n.s("ackMode") != "instant" {
		ep := n.epoch
		s.after(n.f("leaderElectionSec")*1000, func() { // a follower becomes the leader while the old one is still down
			if n.down && n.epoch == ep {
				n.down = false
				s.logEvent("promotion", n, 0)
				s.dispatch(n)
			}
		})
	}
	if downSec > 0 {
		ep := n.epoch
		s.after(downSec*1000, func() {
			if n.down && n.epoch == ep {
				s.RestartNode(n.id)
			}
		})
	}
}

// RestartNode brings a crashed node back.
func (s *Sim) RestartNode(id string) {
	n := s.nodes[id]
	if n == nil || !n.down {
		return
	}
	n.down = false
	n.slow = 1
	s.releaseInDoubt(n)
	if n.store != nil { // a restarted cache or CDN comes back empty
		n.store.Clear()
	}
	s.logEvent("restart", n, 0)
	if n.kind == KDatabase && n.successor != nil && n.successor.promoted && !n.b("fencing") {
		s.rejoinUnfenced(n)
	}
	if n.kind == KClient {
		s.startClient(n)
	}
	s.startNext(n)
	if n.kind == KQueue || n.kind == KTopic || n.kind == KLog {
		s.dispatch(n)
	}
}

// ChaosLog returns chaos events that have fired so far.
func (s *Sim) ChaosLog() []ChaosRecord { return s.chaosLog }

// replicaLag is how far behind a replica effectively runs: none when it replicates synchronously.
func (s *Sim) replicaLag(r *node) float64 {
	if r.s("replication") == "sync" {
		return 0
	}
	return r.f("replicationLagMs") + s.primaryDistanceMs(r)
}

// primaryDistanceMs is the round trip between a replica and the database it copies from (the first database of the
// design): nothing in the same zone, 2 x 1.5 ms across zones, twice the one-way distance across regions. The changes
// have to travel that far before the replica can have them.
func (s *Sim) primaryDistanceMs(r *node) float64 {
	for _, n := range s.order {
		if n.kind != KDatabase || n.removed {
			continue
		}
		d := 0.0
		if n.zone != r.zone {
			d += 2 * crossZoneMs
		}
		return d + 2*regionMs(n.region, r.region)
	}
	return 0
}

// syncReplicaCost is the extra time a write to this primary spends waiting for its synchronous replicas
// (their write time plus a network round trip), and whether one of them is down, which blocks the write.
func (s *Sim) syncReplicaCost(db *node) (wait float64, down bool) {
	for _, r := range s.order {
		if r.kind != KReplica || r.removed || r.promoted || r.s("replication") != "sync" {
			continue
		}
		if r.down {
			return 0, true
		}
		wait = math.Max(wait, r.f("writeMs")+10)
	}
	return wait, false
}

// rejoinUnfenced is the split-brain ending: with no fencing the old primary comes back believing it is still the
// primary, clients return to it, and every write the promoted replica accepted in the meantime exists only there: lost.
func (s *Sim) rejoinUnfenced(db *node) {
	r := db.successor
	lost := r.sincePromotion
	s.runLost += lost
	r.promoted, r.sincePromotion = false, 0
	db.successor = nil
	s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: "splitbrain", NodeID: db.id, Label: db.label, Lost: lost})
	s.logEvent("splitbrain", db, lost)
}

// scheduleGC arranges the next stop-the-world pause of an app server: exponentially distributed gaps with a mean of
// gcEverySec, drawn from the chaos random stream (so servers with no GC setting draw nothing, and the request
// stream is untouched). While paused nothing on the node starts or finishes; queued work waits and the pause ends
// by starting it.
func (s *Sim) scheduleGC(n *node) {
	gap := -math.Log(1-s.chaosRng.Next()) * n.f("gcEverySec") * 1000
	s.after(gap, func() {
		if n.removed {
			return
		}
		pause := n.f("gcPauseMs")
		if pause <= 0 {
			return
		}
		n.pausedUntil = s.now + pause
		s.logEvent("gc", n, 0)
		s.after(pause, func() {
			if n.pausedUntil <= s.now {
				n.pausedUntil = 0
			}
			s.startNext(n)
		})
		s.scheduleGC(n)
	})
}

// scheduleCompaction makes an LSM database stall its writes for compactionStallMs every compactionEverySec. The
// period is fixed (no random draw): a compaction schedule is a policy, not a failure.
func (s *Sim) scheduleCompaction(n *node) {
	every := n.f("compactionEverySec") * 1000
	s.after(every, func() {
		if n.removed {
			return
		}
		n.stallUntil = s.now + n.f("compactionStallMs")
		s.logEvent("compaction", n, 0)
		s.scheduleCompaction(n)
	})
}

// scheduleBurst runs the credit account of a burstable app server every tick: it earns its baseline share of
// the CPU it has, spends the CPU work it started, and is throttled to the baseline while the balance is empty.
// Nothing here draws a random number.
func (s *Sim) scheduleBurst(n *node) {
	s.after(tickMs, func() {
		if n.removed {
			return
		}
		base := n.f("burstBaselinePct") / 100
		full := float64(s.capacity(n)) * 1000
		n.credits += base*full*tickMs/1000 - n.cpuWork
		n.cpuWork = 0
		if max := n.f("burstCreditSec") * full; n.credits > max {
			n.credits = max
		}
		if n.credits < 0 {
			n.credits = 0
		}
		n.throttled = n.credits <= 0
		s.scheduleBurst(n)
	})
}

// linkDown breaks one connection (every edge from NodeID to ToID) for DurationSec. mode "replies" is the nasty
// asymmetric case: requests arrive and are processed, only the answers are lost, so the caller times out
// although the work was done. Held messages are delivered when the link heals (late: callers that gave up ignore them).
func (s *Sim) linkDown(ev ChaosEvent) {
	from, to := s.nodes[ev.NodeID], s.nodes[ev.ToID]
	if from == nil || to == nil || from.removed || to.removed {
		return
	}
	var es []*edge
	for _, e := range s.edges {
		if e.from == from && e.to == to && !e.removed {
			es = append(es, e)
		}
	}
	if len(es) == 0 {
		return
	}
	sort.Slice(es, func(i, j int) bool { return es[i].id < es[j].id }) // map order must not decide delivery order
	req, resp := ev.Mode != "replies", ev.Mode != "requests"
	for _, e := range es {
		e.downReq, e.downResp = req, resp
	}
	s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: "linkDown", NodeID: from.id, Label: from.label + " to " + to.label})
	s.logEvent("partition", from, 0)
	s.after(ev.DurationSec*1000, func() {
		s.logEvent("heal", from, 0)
		for _, e := range es {
			e.downReq, e.downResp = false, false
			held := e.held
			e.held = nil
			for _, deliver := range held {
				deliver()
			}
		}
	})
}

// changeEvent handles the two failures a release can cause. badDeploy puts a bad version on a fraction of the
// components of a kind (at least one, taken in diagram order) for DurationSec, which is how long until someone rolls
// back: mode errors answers every request with an error, slow makes them Factor (default 5) times slower, crash
// takes them down. rollingRestart restarts the components of a kind one at a time, IntervalSec apart, each down for
// DownSec (default 2).
func (s *Sim) changeEvent(ev ChaosEvent) {
	var targets []*node
	for _, n := range s.order {
		if n.kind == ev.Kind && !n.removed && (ev.Version == "" || n.s("version") == ev.Version) {
			targets = append(targets, n)
		}
	}
	if len(targets) == 0 {
		return
	}
	if ev.Type == "rollingRestart" {
		down := ev.DownSec
		if down <= 0 {
			down = 2
		}
		for i, n := range targets {
			id := n.id
			s.after(float64(i)*ev.IntervalSec*1000, func() { s.CrashNode(id, down) })
		}
		return
	}
	count := int(math.Ceil(ev.FractionPct / 100 * float64(len(targets))))
	count = max(1, min(count, len(targets)))
	until := s.now + ev.DurationSec*1000
	for _, n := range targets[:count] {
		n := n
		s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: "badDeploy", NodeID: n.id, Label: n.label})
		s.logEvent("badDeploy", n, 0)
		switch ev.Mode {
		case "slow":
			n.slow, n.slowUntil = math.Max(2, ev.Factor), until
			if ev.Factor <= 1 {
				n.slow = 5
			}
			s.at(until, func() {
				if n.slowUntil == until {
					n.slow = 1
					s.logEvent("rollback", n, 0)
				}
			})
		case "crash":
			s.CrashNode(n.id, ev.DurationSec)
		default:
			n.badUntil = until
			s.at(until, func() {
				if n.badUntil == until {
					n.badUntil = 0
					s.logEvent("rollback", n, 0)
				}
			})
		}
	}
}

// configPush gives one setting a new value on every component of a kind (or on one component) at once, as a
// configuration rollout does, and puts the old values back DurationSec later (0 = it stays).
func (s *Sim) configPush(ev ChaosEvent) {
	type old struct {
		n   *node
		v   any
		had bool
	}
	var saved []old
	for _, n := range s.order {
		if n.removed || n.kind == KClient || (ev.Kind != "" && n.kind != ev.Kind) || (ev.NodeID != "" && ev.Kind == "" && n.id != ev.NodeID) {
			continue
		}
		v, had := n.p[ev.Key]
		saved = append(saved, old{n, v, had})
		n.p[ev.Key] = ev.Value
		s.chaosLog = append(s.chaosLog, ChaosRecord{T: s.now, Type: "configPush", NodeID: n.id, Label: n.label})
		s.logEvent("configPush", n, 0)
	}
	if ev.DurationSec > 0 {
		s.after(ev.DurationSec*1000, func() {
			for _, o := range saved {
				if o.had {
					o.n.p[ev.Key] = o.v
				} else {
					delete(o.n.p, ev.Key)
				}
			}
		})
	}
}
