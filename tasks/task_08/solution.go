package main

import (
	"math"
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
		clock:      clock,
		capacity:   burst,
		refillRate: math.Max(0, ratePerSec),
	}
	if clock == nil || burst <= 0 {
		return limiter
	}
	limiter.accountedAt = clock.Now()
	return limiter
}

func (l *Limiter) AllowN(n int) bool {
	if l.clock == nil || n <= 0 || n > l.capacity {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if observed := l.clock.Now(); observed.After(l.accountedAt) {
		elapsed := observed.Sub(l.accountedAt).Seconds()
		l.spent = math.Max(0, l.spent-elapsed*l.refillRate)
		l.accountedAt = observed
	}

	available := float64(l.capacity) - l.spent
	allowed := float64(n) <= available
	if allowed {
		l.spent += float64(n)
	}
	return allowed
}
