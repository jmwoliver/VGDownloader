package reps

import (
	"fmt"
	"time"
)

type Spinner struct {
	spinChars string
	message   string
	total     *int
	i         int
}

func NewSpinner(message string, total *int) *Spinner {
	return &Spinner{spinChars: `|/-\`, message: message, total: total}
}

func (s *Spinner) Tick(completed *int, total *int) {
	fmt.Printf("\r%c %s (%d / %d)", s.spinChars[s.i], s.message, *completed, *total)
	s.i = (s.i + 1) % len(s.spinChars)
	time.Sleep(100 * time.Millisecond)
}

func (s *Spinner) Finished() {
	fmt.Printf("\r%c %s (%d / %d)", s.spinChars[s.i], s.message, *s.total, *s.total)
}

func (s *Spinner) Loading(completed *int, total *int) {
	// Loading screen
	for {
		s.Tick(completed, total)
	}
}
