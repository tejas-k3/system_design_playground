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
