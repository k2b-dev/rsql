package telemetry

import "time"

var latencyBoundsMS = [...]float64{
	0.25, 0.5, 1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000,
}

// Histogram is a fixed, mergeable latency histogram.
type Histogram struct {
	Counts []uint64 `json:"counts"`
	Total  uint64   `json:"total"`
	SumMS  float64  `json:"sum_ms"`
}

func newHistogram() Histogram {
	return Histogram{Counts: make([]uint64, len(latencyBoundsMS)+1)}
}

func (h *Histogram) observe(duration time.Duration) {
	if len(h.Counts) == 0 {
		h.Counts = make([]uint64, len(latencyBoundsMS)+1)
	}
	ms := float64(duration) / float64(time.Millisecond)
	index := len(latencyBoundsMS)
	for i, bound := range latencyBoundsMS {
		if ms <= bound {
			index = i
			break
		}
	}
	h.Counts[index]++
	h.Total++
	h.SumMS += ms
}

func (h *Histogram) merge(other Histogram) {
	if len(h.Counts) == 0 {
		h.Counts = make([]uint64, len(latencyBoundsMS)+1)
	}
	for i, count := range other.Counts {
		if i < len(h.Counts) {
			h.Counts[i] += count
		}
	}
	h.Total += other.Total
	h.SumMS += other.SumMS
}

// PercentileMS returns an approximate latency percentile in milliseconds.
func (h Histogram) PercentileMS(percentile float64) float64 {
	if h.Total == 0 {
		return 0
	}
	if percentile <= 0 {
		percentile = 0.5
	}
	if percentile > 1 {
		percentile = 1
	}
	target := uint64(float64(h.Total)*percentile + 0.999999)
	var cumulative uint64
	for i, count := range h.Counts {
		cumulative += count
		if cumulative < target {
			continue
		}
		if i < len(latencyBoundsMS) {
			return latencyBoundsMS[i]
		}
		return latencyBoundsMS[len(latencyBoundsMS)-1]
	}
	return latencyBoundsMS[len(latencyBoundsMS)-1]
}

// AverageMS returns the arithmetic mean in milliseconds.
func (h Histogram) AverageMS() float64 {
	if h.Total == 0 {
		return 0
	}
	return h.SumMS / float64(h.Total)
}
