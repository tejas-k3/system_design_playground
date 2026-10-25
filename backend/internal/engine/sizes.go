package engine

import "math"

// sizeFactor is the multiplier on a request's mean size when the users' sizeDist is lognormal: exp(sigma*z - sigma^2/2),
// whose mean is exactly 1, with z a standard normal. It is a pure function of the run's seed and the request number
// (two hashes through the Box-Muller transform), so a request has the same size at every hop and on every retry, and
// no random stream is touched, which keeps every other draw of the run where it was. salt separates the request's
// size from its response's.
func sizeFactor(seed uint32, id int, salt uint32, sigma float64) float64 {
	h1 := Hash32(uint32(id)*2654435761 ^ seed*0x9e3779b1 ^ salt*0x85ebca6b)
	h2 := Hash32(h1 ^ 0x68e31da4 + salt)
	u1 := (float64(h1) + 0.5) / 4294967296
	u2 := (float64(h2) + 0.5) / 4294967296
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return math.Exp(sigma*z - sigma*sigma/2)
}

// lgSizes are the bytes of one request and one response of a user request: the users' sizes, fixed (default) or
// spread lognormally around the same mean. 0 means not modelled.
func (s *Sim) lgSizes(lg *logical) (req, resp float64) {
	req, resp = lg.client.f("reqKB")*1000, lg.client.f("respKB")*1000
	if (req > 0 || resp > 0) && lg.client.s("sizeDist") == "lognormal" {
		sigma := lg.client.f("sizeSigma")
		req *= sizeFactor(s.opts.Seed, lg.id, 1, sigma)
		resp *= sizeFactor(s.opts.Seed, lg.id, 2, sigma)
	}
	return req, resp
}

func (s *Sim) reqBytes(lg *logical) float64 {
	req, _ := s.lgSizes(lg)
	return req
}
