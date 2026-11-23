package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// The reasoning is kept for /think rather than shown live: a scrolling
// ticker of it was too busy to read.
type status struct {
	mu         sync.Mutex
	start      time.Time
	phase      string
	thinking   []byte
	thoughtFor time.Duration
	stop       chan struct{}
	done       chan struct{}
}

func newStatus(draw bool) *status {
	s := &status{start: time.Now(), phase: "thinking", stop: make(chan struct{}), done: make(chan struct{})}
	if draw {
		go s.draw()
	} else {
		close(s.done)
	}
	return s
}

func (s *status) draw() {
	defer close(s.done)
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for i := 0; ; i++ {
		select {
		case <-s.stop:
			fmt.Fprint(os.Stderr, "\r\033[K")
			return
		case <-tick.C:
		}
		s.mu.Lock()
		phase := s.phase
		s.mu.Unlock()
		fmt.Fprintf(os.Stderr, "\r\033[2m%c %s %ds\033[0m\033[K",
			frames[i%len(frames)], phase, int(time.Since(s.start).Seconds()))
	}
}

func (s *status) addThinking(t string) {
	s.mu.Lock()
	s.thinking = append(s.thinking, t...)
	s.mu.Unlock()
}

func (s *status) writing() {
	s.mu.Lock()
	if s.phase == "thinking" {
		s.thoughtFor = time.Since(s.start)
	}
	s.phase = "writing"
	s.mu.Unlock()
}

func (s *status) close() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}
