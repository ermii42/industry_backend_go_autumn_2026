package main

import (
	"sync"
	"time"
)

type Clock interface{ Now() time.Time }

type Limiter struct {
	mu     sync.Mutex
	clock  Clock
	rate   float64
	burst  int
	tokens float64
	last   time.Time
}

func NewLimiter(clock Clock, ratePerSec float64, burst int) *Limiter {
	limiter := &Limiter{
		clock: clock,
		rate:  ratePerSec,
		burst: burst,
	}
	if clock == nil || burst <= 0 {
		return limiter
	}
	limiter.tokens = float64(burst)
	limiter.last = clock.Now()
	return limiter
}

func (l *Limiter) AllowN(n int) bool {
	if l.clock == nil || n <= 0 || n > l.burst {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if observed := l.clock.Now(); observed.After(l.last) {
		if l.rate > 0 {
			l.tokens = min(float64(l.burst), l.tokens+observed.Sub(l.last).Seconds()*l.rate)
		}
		l.last = observed
	}

	if l.tokens < float64(n) {
		return false
	}
	l.tokens -= float64(n)
	return true
}
