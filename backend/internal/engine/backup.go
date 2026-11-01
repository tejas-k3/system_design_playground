package engine

import "math"

// Backups and restore. A database with backupEverySec set takes a backup that often. A restore (chaos "restore")
// puts it back to the last backup taken before the damage began, so every write since is lost (the recovery point,
// RPO), and the database answers nothing while the backup streams back (the recovery time, RTO).

func (s *Sim) scheduleBackup(n *node) {
	every := n.f("backupEverySec") * 1000
	var tick func()
	tick = func() {
		if n.removed {
			return
		}
		if !n.down {
			n.backups = append(n.backups, s.now)
		}
		if iv := n.f("backupEverySec") * 1000; iv > 0 {
			s.after(iv, tick)
		}
	}
	s.after(every, tick)
}

func (s *Sim) restore(n *node) {
	damage := s.now
	if n.corruptAt > 0 {
		damage = n.corruptAt
	}
	from := 0.0 // with no backup the database goes back to when it was created
	for _, b := range n.backups {
		if b <= damage && b > from {
			from = b
		}
	}
	s.truncateVersions(from)
	lost := 0
	for _, w := range n.writeLog {
		if w > from {
			lost++
		}
	}
	s.runLost += lost
	n.writeLog = nil
	s.runRPO = (s.now - from) / 1000
	rto := 0.0
	if mbps := n.f("restoreMBps"); mbps > 0 {
		rto = n.f("dataGB") * 1024 / mbps
		if oe := s.first(n, KObjectStore); oe != nil {
			rto = s.restoreFromStore(n, oe, mbps)
		}
	}
	s.runRTO = rto
	n.corruptAt = 0
	s.logEvent("restore", n, 0)
	if rto > 0 {
		s.CrashNode(n.id, rto)
	}
}

// multiLeaderWrite notes a write at one of several leaders. If the previous write of the same key was at another
// leader less than syncLagMs ago, neither leader had seen the other's write: a conflict. Last-writer-wins drops
// one of them, a merge keeps both.
func (s *Sim) multiLeaderWrite(n *node, c *call) {
	leaders := clampInt(int(n.f("leaders")), 2, 8)
	leader := int(hash32(c.hash*2246822519+3) % uint32(leaders))
	if n.mlLast == nil {
		n.mlLast = map[int]mlWrite{}
	}
	policy := n.s("conflictPolicy")
	prev, seen := n.mlLast[c.key]
	concurrent := seen && prev.leader != leader && s.now-prev.t < n.f("syncLagMs")
	if concurrent {
		n.mlConflicts++
		if policy != "merge" && c.counted {
			s.runLostUpdates++
		}
	}
	stamp := 0.0
	if policy == "lastWriterWins" {
		stamp = s.now + s.leaderClockError(n, leader)
		// Last-writer-wins keeps the write with the later time stamp, wherever it came from: a leader whose clock is
		// behind loses to an older write of the other leader, and its write is dropped without being recorded, so later
		// writes at that leader keep losing until the leader that is ahead writes the key again. Logical versions and
		// merging do not look at the clock. A drop that is also a true conflict is already counted as a lost update.
		if seen && prev.leader != leader && stamp < prev.stamp {
			if c.counted && !concurrent {
				s.runClockLost++
			}
			return
		}
	}
	n.mlLast[c.key] = mlWrite{t: s.now, leader: leader, stamp: stamp}
}

// leaderClockError is how far a leader's clock is ahead of true time. The first leader is the reference; leader i is
// ahead by i times the offset, plus i times the drift since its last synchronisation.
func (s *Sim) leaderClockError(n *node, leader int) float64 {
	spread, drift := n.f("clockSpreadMs"), n.f("clockDriftMsPerMin")
	if leader == 0 || (spread == 0 && drift == 0) {
		return 0
	}
	since := s.now
	if sync := n.f("clockSyncSec") * 1000; sync > 0 {
		since = math.Mod(s.now, sync)
	}
	return float64(leader) * (spread + drift*since/60000)
}
