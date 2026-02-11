package ratelimit

import (
	"context"
	"sync"
	"time"
)

type memoryLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*entry
	cleanup time.Duration
}

type entry struct {
	count int
	reset time.Time
}

func NewMemory(limit int, window time.Duration) *memoryLimiter {
	l := &memoryLimiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]*entry),
		cleanup: window * 2,
	}
	if l.cleanup < time.Minute {
		l.cleanup = time.Minute
	}
	go l.startCleanup()
	return l
}

func (l *memoryLimiter) Allow(_ context.Context, key string) (allowed bool, retryAfter time.Duration) {
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

func (l *memoryLimiter) startCleanup() {
	t := time.NewTicker(l.cleanup)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		l.mu.Lock()
		for k, e := range l.entries {
			if now.After(e.reset) {
				delete(l.entries, k)
			}
		}
		l.mu.Unlock()
	}
}
