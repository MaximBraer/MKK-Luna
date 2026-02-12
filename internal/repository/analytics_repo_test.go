package repository

import (
	"testing"
	"time"
)

func TestAnalyticsTopCreatorsLimitValidation(t *testing.T) {
	repo := NewAnalyticsRepository(nil)

	if _, err := repo.GetTopCreatorsByTeam(nil, zeroTime(), zeroTime(), 0); err != ErrInvalidLimit {
		t.Fatalf("expected ErrInvalidLimit for limit=0, got %v", err)
	}
	if _, err := repo.GetTopCreatorsByTeam(nil, zeroTime(), zeroTime(), 11); err != ErrInvalidLimit {
		t.Fatalf("expected ErrInvalidLimit for limit=11, got %v", err)
	}
}

func zeroTime() (t time.Time) {
	return t
}
