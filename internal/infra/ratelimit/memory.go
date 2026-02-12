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

	now := time.Now().UTC()
	reset := fixedWindowReset(now, l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.entries[key]
	if !ok || now.After(e.reset) {
		e = &entry{count: 0, reset: reset}
		l.entries[key] = e
	}

	if e.count >= l.limit {
		ra := time.Until(e.reset)
		if ra <= 0 {
			ra = time.Second
		}
		return false, ra
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

func fixedWindowReset(now time.Time, window time.Duration) time.Time {
	if window <= 0 {
		return now.Add(time.Second)
	}
	ws := int64(window.Seconds())
	if ws <= 0 {
		return now.Add(time.Second)
	}
	epoch := now.Unix() / ws
	start := time.Unix(epoch*ws, 0).UTC()
	return start.Add(window)
}
