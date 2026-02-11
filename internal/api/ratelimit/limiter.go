package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*entry
}

type entry struct {
	count int
	reset time.Time
}

func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, entries: make(map[string]*entry)}
}

func (l *Limiter) Allow(key string) (allowed bool, retryAfter time.Duration) {
	if key == "" || l.limit <= 0 {
		return true, 0
	}

	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.entries[key]
	if !ok || now.After(e.reset) {
		e = &entry{count: 0, reset: now.Add(l.window)}
		l.entries[key] = e
	}

	if e.count >= l.limit {
		return false, time.Until(e.reset)
	}

	e.count++
	return true, 0
}
