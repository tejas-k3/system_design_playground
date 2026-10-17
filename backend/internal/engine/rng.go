package engine

import "math"

// RNG is a small deterministic PRNG (mulberry32) so a run is reproducible from its seed.
type RNG struct{ s uint32 }

func NewRNG(seed uint32) *RNG { return &RNG{s: seed} }

// Next returns a uniform float in [0,1).
func (r *RNG) Next() float64 {
	r.s += 0x6D2B79F5
	t := r.s
	t = (t ^ (t >> 15)) * (t | 1)
	t ^= t + (t^(t>>7))*(t|61)
	return float64(t^(t>>14)) / 4294967296.0
}

// Exp returns an Exp(1) sample.
func (r *RNG) Exp() float64 { return -math.Log(1 - r.Next()) }

// Service turns a mean service time into a sample between 0.5*mean and ~mean*(0.5+k).
func (r *RNG) Service(mean float64) float64 { return mean * (0.5 + 0.5*r.Exp()) }

// Hash32 is an integer finalizer used to derive sub-stream seeds and stable per-request hashes.
func Hash32(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x85ebca6b
	x ^= x >> 13
	x *= 0xc2b2ae35
	x ^= x >> 16
	return x
}

// ServiceDist draws a service time with the given mean and the spread a node's "variability" setting asks for:
//
//	steady (default, ""):  0.5*mean + Exp(0.5*mean), SCV 0.25, the original model
//	exponential:           Exp(mean), SCV 1, what queueing textbooks assume
//	heavy:                 lognormal with SCV 4, a long right tail like real services
//
// SCV is the squared coefficient of variation (variance / mean^2). The mean is the same in every mode.
func (r *RNG) ServiceDist(mean float64, mode string) float64 {
	switch mode {
	case "exponential":
		return mean * r.Exp()
	case "heavy":
		const sigma2 = 1.6094379124341003 // ln(1 + SCV) for SCV = 4
		u1, u2 := 1-r.Next(), r.Next()
		z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
		return mean * math.Exp(math.Sqrt(sigma2)*z-sigma2/2)
	}
	return r.Service(mean)
}
