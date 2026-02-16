package ratelimit

import (
	"testing"
	"time"
)

func TestRemainingDuration(t *testing.T) {
	now := time.Unix(120, 0).UTC()
	d := remainingDuration(60*time.Second, now)
	if d != 60*time.Second {
		t.Fatalf("expected 60s at boundary, got %v", d)
	}

	now = time.Unix(121, 0).UTC()
	d = remainingDuration(60*time.Second, now)
	if d != 59*time.Second {
		t.Fatalf("expected 59s remaining, got %v", d)
	}

	d = remainingDuration(0, now)
	if d != time.Second {
		t.Fatalf("expected 1s when window invalid, got %v", d)
	}
}
