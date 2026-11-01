package engine

// restoreFromStore is the recovery time when the backups sit in an object store the database is wired to: the
// backup streams back at the slowest of the link (bandwidth / 8), the store's streams (its per-stream throughput,
// times the parts in parallel when multipart is on, else one stream) and the database's own restore speed, after
// the store's time to first byte.
func (s *Sim) restoreFromStore(n *node, oe *edge, dbMBps float64) float64 {
	st := oe.to
	rate := dbMBps
	streams := 1.0
	if st.f("multipartKB") > 0 {
		streams = max(1, st.f("multipartParallel"))
	}
	rate = min(rate, st.f("throughputMBps")*streams)
	if oe.bw > 0 {
		rate = min(rate, oe.bw/8)
	}
	return st.f("firstByteMs")/1000 + n.f("dataGB")*1024/rate
}
