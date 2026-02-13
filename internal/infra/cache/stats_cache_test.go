package cache

import (
	"testing"
	"time"
)

func TestStatsCacheKeyDayNormalized(t *testing.T) {
	from := time.Date(2026, 2, 13, 10, 22, 11, 0, time.UTC)
	to := time.Date(2026, 2, 20, 23, 59, 59, 0, time.UTC)

	gotDone := doneKey(42, from, to)
	wantDone := "stats:done:u:42:f:20260213:t:20260220"
	if gotDone != wantDone {
		t.Fatalf("done key=%q want=%q", gotDone, wantDone)
	}

	gotTop := topKey(42, from, to, 5)
	wantTop := "stats:top:u:42:f:20260213:t:20260220:l:5"
	if gotTop != wantTop {
		t.Fatalf("top key=%q want=%q", gotTop, wantTop)
	}
}
