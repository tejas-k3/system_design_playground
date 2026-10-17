package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// limiter is a small fixed-window rate limit per client address. It is deliberately simple:
// good enough to stop one client filling the design store, not a substitute for a proxy limit.
type limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*bucket
	now    func() time.Time
}

type bucket struct {
	start time.Time
	n     int
}

func newLimiter(limit int, per time.Duration) *limiter {
	return &limiter{limit: limit, window: per, hits: map[string]*bucket{}, now: time.Now}
}

// allow reports whether this client may act now, counting the attempt.
func (l *limiter) allow(client string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.hits) > 10000 { // forget idle clients so the map cannot grow without bound
		for k, w := range l.hits {
			if now.Sub(w.start) > l.window {
				delete(l.hits, k)
			}
		}
	}
	w := l.hits[client]
	if w == nil || now.Sub(w.start) >= l.window {
		l.hits[client] = &bucket{start: now, n: 1}
		return true
	}
	if w.n >= l.limit {
		return false
	}
	w.n++
	return true
}

// clientAddr is the peer address without its port. Behind a reverse proxy this is the proxy.
func clientAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
