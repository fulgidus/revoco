// Package components contains reusable Bubble Tea UI widgets.
package components

import (
	"fmt"
	"strings"
)

const sparklineChars = "▁▂▃▄▅▆▇█"

// Sparkline tracks a rolling window of float64 samples and renders an ASCII
// throughput graph using Unicode block characters.
type Sparkline struct {
	// Width is the number of columns the sparkline should fill.
	Width int
	// Label is displayed after the graph (e.g. "MB/s").
	Label string
	// samples is the ring buffer of values.
	samples []float64
	cap     int
}

// NewSparkline creates a Sparkline with the given width and label.
func NewSparkline(width int, label string) Sparkline {
	cap := width
	if cap < 4 {
		cap = 4
	}
	return Sparkline{
		Width:   width,
		Label:   label,
		samples: make([]float64, 0, cap),
		cap:     cap,
	}
}

// Push adds a new sample, evicting the oldest if the buffer is full.
func (s *Sparkline) Push(v float64) {
	if len(s.samples) >= s.cap {
		s.samples = s.samples[1:]
	}
	s.samples = append(s.samples, v)
}

// View renders the sparkline as a single line: graph + current value + label.
func (s Sparkline) View() string {
	if len(s.samples) == 0 {
		return strings.Repeat("▁", s.Width) + " 0.0 " + s.Label
	}

	// Find max for normalisation
	max := 0.0
	for _, v := range s.samples {
		if v > max {
			max = v
		}
	}

	runes := []rune(sparklineChars)
	nLevels := len(runes) // 8

	var sb strings.Builder
	// Pad left with lowest char if fewer samples than width
	padding := s.Width - len(s.samples)
	if padding < 0 {
		padding = 0
	}
	for i := 0; i < padding; i++ {
		sb.WriteRune(runes[0])
	}

	for _, v := range s.samples {
		idx := 0
		if max > 0 {
			idx = int(v / max * float64(nLevels-1))
		}
		if idx >= nLevels {
			idx = nLevels - 1
		}
		sb.WriteRune(runes[idx])
	}

	current := s.samples[len(s.samples)-1]
	sb.WriteString(fmt.Sprintf(" %5.1f %s", current, s.Label))
	return sb.String()
}
