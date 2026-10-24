package engine

// A write that has to tell others about itself: save to the database and publish an event to a queue, topic or log.
// As two separate steps (dual write) a failure of the second leaves the first without its event for good. With an
// outbox the event is stored with the data in one transaction and a relay publishes it, retrying until it is taken.

// eventWrite handles a write under eventPublish dualWrite or outbox. It reports false when the server has no
// database or no place to publish to, so the ordinary path runs instead.
func (s *Sim) eventWrite(n *node, c *call, mode string, done func(Result)) bool {
	pe := s.primaryEdge(n)
	qe := s.first(n, KQueue)
	if qe == nil {
		qe = s.first(n, KTopic)
	}
	if qe == nil {
		qe = s.first(n, KLog)
	}
	if pe == nil || qe == nil {
		return false
	}
	ec := *c
	qe.to.watchIds = true
	s.dbWrite(n, c, func(res Result) {
		if !res.OK {
			done(res)
			return
		}
		if mode == "outbox" {
			n.outboxBacklog++ // the event is already safe: it was saved with the data
			var relay func()
			relay = func() {
				s.callCmd(n, qe, &ec, cmdEnqueue, 0, func(r Result) {
					if r.OK {
						n.outboxBacklog--
						return
					}
					s.after(n.f("outboxRetryMs"), relay)
				})
			}
			relay()
			done(res)
			return
		}
		s.callCmd(n, qe, &ec, cmdEnqueue, 0, func(r Result) {
			if !r.OK && c.counted {
				s.runEventsLost++ // the data is saved and nobody was told
			}
			done(res)
		})
	})
	return true
}

// eventRepeat is whether a queue, topic or log has already accepted this message id. It only watches the targets of an
// event publisher. With dedupeIds the repeat is acknowledged without being stored again; without it the repeat is
// counted and stored like any other message.
func (s *Sim) eventRepeat(n *node, c *call) (repeat, drop bool) {
	if !n.watchIds || c.rid == 0 {
		return false, false
	}
	if n.seenIds == nil {
		n.seenIds = map[int]struct{}{}
	}
	if _, seen := n.seenIds[c.rid]; seen {
		drop := n.b("dedupeIds")
		if c.counted && !drop {
			s.runEventDups++ // stored a second time: the consumers will see it twice
		}
		return true, drop
	}
	n.seenIds[c.rid] = struct{}{}
	return false, false
}
