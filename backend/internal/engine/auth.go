package engine

// Who is calling. Every request carries a token. The app server either asks the auth service whether it is valid (and may
// remember a yes for a while, per user), or verifies the signature itself. When the auth service fails, the server
// refuses the request (closed) or lets it through unchecked (open). Encryption is a time cost only; nothing is encrypted.

// securityMs is the CPU time a request spends on its signature check and on a TLS handshake. It draws no random numbers:
// which requests open a new connection or resume a session is a hash of the request id.
func (s *Sim) securityMs(n *node, c *call) float64 {
	ms := 0.0
	if n.s("authMode") == "localSignature" {
		ms += n.f("verifyMs")
	}
	if tls := n.f("tlsMs"); tls > 0 {
		if float64(Hash32(uint32(c.rid)*2654435761+5)%100) < n.f("newConnPct") {
			if float64(Hash32(uint32(c.rid)*40503+23)%100) < n.f("tlsResumePct") {
				tls /= 3
			}
			ms += tls
		}
	}
	return ms
}

// authenticate asks the auth service about the request's token, unless a yes for that user is still remembered.
func (s *Sim) authenticate(n *node, e *edge, c *call, then func(Result)) {
	user := int(Hash32(uint32(c.rid)*40503+9) % uint32(max(1, int(n.f("tokenUsers")))))
	ttl := n.f("tokenCacheSec") * 1000
	if ttl > 0 {
		if exp, ok := n.authTokens[user]; ok && exp > s.now {
			if c.counted {
				s.runAuthCached++
			}
			then(Result{OK: true, Ver: -1, Done: true, Src: n})
			return
		}
	}
	if c.counted {
		s.runAuthCalls++
	}
	s.callCmd(n, e, c, cmdAuth, 0, func(res Result) {
		if res.OK {
			if ttl > 0 {
				if n.authTokens == nil {
					n.authTokens = map[int]float64{}
				}
				n.authTokens[user] = s.now + ttl
			}
			then(res)
			return
		}
		if n.s("authFailMode") == "open" {
			if c.counted {
				s.runUnchecked++
			}
			then(Result{OK: true, Ver: -1, Done: true, Src: n})
			return
		}
		then(res)
	})
}
