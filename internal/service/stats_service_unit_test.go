package service

import (
	"context"
	"testing"
	"time"

	"MKK-Luna/internal/repository"
)

type fakeAnalyticsRepo struct {
	done      []repository.TeamDoneStat
	top       []repository.TeamTopCreator
	integrity []repository.TaskIntegrityIssue
	err       error
}

type fakeStatsCache struct {
	doneItems []repository.TeamDoneStat
	topItems  []repository.TeamTopCreator
	doneHit   bool
	topHit    bool
}

func (f *fakeStatsCache) GetDone(ctx context.Context, userID int64, from, to time.Time) ([]repository.TeamDoneStat, bool, error) {
	if f.doneHit {
		return f.doneItems, true, nil
	}
	return nil, false, nil
}

func (f *fakeStatsCache) SetDone(ctx context.Context, userID int64, from, to time.Time, items []repository.TeamDoneStat) error {
	f.doneItems = items
	return nil
}

func (f *fakeStatsCache) GetTop(ctx context.Context, userID int64, from, to time.Time, limit int) ([]repository.TeamTopCreator, bool, error) {
	if f.topHit {
		return f.topItems, true, nil
	}
	return nil, false, nil
}

func (f *fakeStatsCache) SetTop(ctx context.Context, userID int64, from, to time.Time, limit int, items []repository.TeamTopCreator) error {
	f.topItems = items
	return nil
}

func (f *fakeAnalyticsRepo) GetTeamDoneStats(ctx context.Context, userID int64, from, to time.Time) ([]repository.TeamDoneStat, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.done, nil
}

func (f *fakeAnalyticsRepo) GetTopCreatorsByTeam(ctx context.Context, userID int64, from, to time.Time, limit int) ([]repository.TeamTopCreator, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.top, nil
}

func (f *fakeAnalyticsRepo) FindTasksWithAssigneeNotMember(ctx context.Context) ([]repository.TaskIntegrityIssue, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.integrity, nil
}

func TestStatsServiceRangeValidation(t *testing.T) {
	svc := NewStatsService(&fakeAnalyticsRepo{}, nil, nil, nil)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := svc.GetTeamDoneStats(context.Background(), 1, now, now); err != ErrBadRequest {
		t.Fatalf("expected ErrBadRequest for from==to, got %v", err)
	}

	if _, err := svc.GetTeamDoneStats(context.Background(), 1, now.Add(2*time.Hour), now); err != ErrBadRequest {
		t.Fatalf("expected ErrBadRequest for from>to, got %v", err)
	}

	if _, err := svc.GetTeamDoneStats(context.Background(), 1, now, now.Add(366*24*time.Hour)); err != ErrBadRequest {
		t.Fatalf("expected ErrBadRequest for range>365d, got %v", err)
	}
}

func TestStatsServiceLimitValidation(t *testing.T) {
	svc := NewStatsService(&fakeAnalyticsRepo{}, nil, nil, nil)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := now.Add(24 * time.Hour)

	if _, err := svc.GetTopCreatorsByTeam(context.Background(), 1, now, later, 0); err != ErrBadRequest {
		t.Fatalf("expected ErrBadRequest for limit=0, got %v", err)
	}
	if _, err := svc.GetTopCreatorsByTeam(context.Background(), 1, now, later, 11); err != ErrBadRequest {
		t.Fatalf("expected ErrBadRequest for limit=11, got %v", err)
	}
}

func TestStatsServiceEmptyTeamsReturnsEmpty(t *testing.T) {
	svc := NewStatsService(&fakeAnalyticsRepo{done: nil}, nil, nil, nil)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := now.Add(24 * time.Hour)

	rows, err := svc.GetTeamDoneStats(context.Background(), 1, now, later)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected empty rows, got %d", len(rows))
	}
}

func TestStatsServiceAdminAllowlistEmpty(t *testing.T) {
	svc := NewStatsService(&fakeAnalyticsRepo{}, nil, nil, nil)
	_, err := svc.FindTasksWithAssigneeNotMember(context.Background(), 1)
	if err != ErrForbidden {
		t.Fatalf("expected ErrForbidden for empty allowlist, got %v", err)
	}
}

func TestStatsServiceUsesCache(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := now.Add(24 * time.Hour)

	cache := &fakeStatsCache{
		doneHit:   true,
		doneItems: []repository.TeamDoneStat{{TeamID: 9, DoneCount: 11}},
	}
	svc := NewStatsService(&fakeAnalyticsRepo{done: []repository.TeamDoneStat{{TeamID: 1, DoneCount: 1}}}, cache, nil, nil)

	rows, err := svc.GetTeamDoneStats(context.Background(), 1, now, later)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || rows[0].TeamID != 9 {
		t.Fatalf("expected cached rows, got %+v", rows)
	}
}
