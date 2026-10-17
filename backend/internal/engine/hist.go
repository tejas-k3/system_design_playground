package engine

import "math"

const (
	histGrowth = 1.02
	histOffset = 400
	histBins   = 1400
)

var histLog = math.Log(histGrowth)

// Hist is a log-bucket latency histogram giving ~2% relative quantile error.
type Hist struct {
	counts [histBins]float64
	n, sum float64
}

func (h *Hist) Add(v float64) {
	b := int(math.Floor(math.Log(math.Max(v, 0.001))/histLog)) + histOffset
	if b < 0 {
		b = 0
	} else if b >= histBins {
		b = histBins - 1
	}
	h.counts[b]++
	h.n++
	h.sum += v
}

func (h *Hist) Count() int { return int(h.n) }

func (h *Hist) Quantile(q float64) float64 {
	if h.n == 0 {
		return 0
	}
	target := math.Max(1, math.Ceil(q*h.n))
	var acc float64
	for i := 0; i < histBins; i++ {
		acc += h.counts[i]
		if acc >= target {
			return math.Pow(histGrowth, float64(i-histOffset)+0.5)
		}
	}
	return math.Pow(histGrowth, 1000)
}

func (h *Hist) Mean() float64 {
	if h.n == 0 {
		return 0
	}
	return h.sum / h.n
}

// LatencyBounds are the upper edges (ms) of the buckets shown in the latency distribution; one more bucket catches everything above.
var LatencyBounds = []float64{10, 20, 50, 100, 150, 200, 300, 500, 750, 1000, 1500, 2000, 3000, 5000, 10000}

// Buckets counts the recorded values per display bucket: bounds[i-1] < v <= bounds[i], plus a last bucket above the largest bound.
// Counts come from the fine log bins, so a value is placed by its bin's midpoint (about 1% accurate).
func (h *Hist) Buckets(bounds []float64) []int {
	out := make([]int, len(bounds)+1)
	for i := 0; i < histBins; i++ {
		if h.counts[i] == 0 {
			continue
		}
		mid := math.Pow(histGrowth, float64(i-histOffset)+0.5)
		k := 0
		for k < len(bounds) && mid > bounds[k] {
			k++
		}
		out[k] += int(h.counts[i])
	}
	return out
}
