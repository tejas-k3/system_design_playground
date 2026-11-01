package engine

import "math"

// keySampler draws item keys for a client: Zipf (default), uniform or a sequential scan.
// Key 0 is reserved for the "hot key" override.
type keySampler struct {
	cdf      []float64
	cdfK     int
	cdfS     float64
	scanNext int
}

// sample draws a key in [0, keySpace) from the named distribution, optionally sending hotPct percent of the draws to
// key 0.
func (k *keySampler) sample(dist string, keySpace int, zipfS, hotPct float64, rng *prng) int {
	if hotPct > 0 && rng.Next() < hotPct/100 {
		return 0
	}
	if keySpace < 1 {
		keySpace = 1
	}
	switch dist {
	case "scan":
		k.scanNext++
		return 1 + (k.scanNext-1)%keySpace
	case "uniform":
		return 1 + int(rng.Next()*float64(keySpace))
	}
	if zipfS <= 0 {
		return 1 + int(rng.Next()*float64(keySpace))
	}
	return 1 + k.rank(keySpace, zipfS, rng.Next())
}

func (k *keySampler) rank(n int, s, u float64) int {
	if k.cdf == nil || k.cdfK != n || k.cdfS != s {
		k.cdf = make([]float64, n)
		var sum float64
		for i := 1; i <= n; i++ {
			sum += math.Pow(float64(i), -s)
			k.cdf[i-1] = sum
		}
		for i := range k.cdf {
			k.cdf[i] /= sum
		}
		k.cdf[n-1] = 1
		k.cdfK, k.cdfS = n, s
	}
	lo, hi := 0, n-1
	for lo < hi {
		m := (lo + hi) / 2
		if k.cdf[m] > u {
			hi = m
		} else {
			lo = m + 1
		}
	}
	return lo
}
