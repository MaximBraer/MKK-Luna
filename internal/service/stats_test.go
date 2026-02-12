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
	svc := NewStatsService(&fakeAnalyticsRepo{}, nil, nil)

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
	svc := NewStatsService(&fakeAnalyticsRepo{}, nil, nil)
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
	svc := NewStatsService(&fakeAnalyticsRepo{done: nil}, nil, nil)
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
	svc := NewStatsService(&fakeAnalyticsRepo{}, nil, nil)
	_, err := svc.FindTasksWithAssigneeNotMember(context.Background(), 1)
	if err != ErrForbidden {
		t.Fatalf("expected ErrForbidden for empty allowlist, got %v", err)
	}
}
