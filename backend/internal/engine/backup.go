package engine

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
	}
	s.runRTO = rto
	n.corruptAt = 0
	s.logEvent("restore", n, 0)
	if rto > 0 {
		s.CrashNode(n.id, rto)
	}
}
