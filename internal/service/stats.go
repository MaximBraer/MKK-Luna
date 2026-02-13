package service

import (
	"context"
	"log/slog"
	"time"

	dcache "MKK-Luna/internal/domain/cache"
	"MKK-Luna/internal/repository"
)

const (
	maxStatsRange = 365 * 24 * time.Hour
)

type analyticsStore interface {
	GetTeamDoneStats(ctx context.Context, userID int64, from, to time.Time) ([]repository.TeamDoneStat, error)
	GetTopCreatorsByTeam(ctx context.Context, userID int64, from, to time.Time, limit int) ([]repository.TeamTopCreator, error)
	FindTasksWithAssigneeNotMember(ctx context.Context) ([]repository.TaskIntegrityIssue, error)
}

type StatsService struct {
	repo       analyticsStore
	cache      dcache.StatsCache
	adminUsers map[int64]struct{}
	logger     *slog.Logger
}

func NewStatsService(repo analyticsStore, statsCache dcache.StatsCache, adminUserIDs []int64, logger *slog.Logger) *StatsService {
	admins := make(map[int64]struct{}, len(adminUserIDs))
	for _, id := range adminUserIDs {
		if id > 0 {
			admins[id] = struct{}{}
		}
	}
	if logger != nil && len(admins) == 0 {
		logger.Warn("admin allowlist is empty")
	}
	return &StatsService{repo: repo, cache: statsCache, adminUsers: admins, logger: logger}
}

func (s *StatsService) GetTeamDoneStats(ctx context.Context, userID int64, from, to time.Time) ([]repository.TeamDoneStat, error) {
	if err := validateStatsRange(from, to); err != nil {
		return nil, err
	}
	if s.cache != nil {
		if rows, ok, err := s.cache.GetDone(ctx, userID, from, to); err == nil && ok {
			return rows, nil
		}
	}

	rows, err := s.repo.GetTeamDoneStats(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.SetDone(ctx, userID, from, to, rows)
	}
	return rows, nil
}

func (s *StatsService) GetTopCreatorsByTeam(ctx context.Context, userID int64, from, to time.Time, limit int) ([]repository.TeamTopCreator, error) {
	if err := validateStatsRange(from, to); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10 {
		return nil, ErrBadRequest
	}
	if s.cache != nil {
		if rows, ok, err := s.cache.GetTop(ctx, userID, from, to, limit); err == nil && ok {
			return rows, nil
		}
	}

	rows, err := s.repo.GetTopCreatorsByTeam(ctx, userID, from, to, limit)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.SetTop(ctx, userID, from, to, limit, rows)
	}
	return rows, nil
}

func (s *StatsService) FindTasksWithAssigneeNotMember(ctx context.Context, userID int64) ([]repository.TaskIntegrityIssue, error) {
	if !s.isAdmin(userID) {
		if s.logger != nil {
			s.logger.Warn("non-admin access to integrity endpoint", slog.Int64("user_id", userID))
		}
		return nil, ErrForbidden
	}
	return s.repo.FindTasksWithAssigneeNotMember(ctx)
}

func (s *StatsService) isAdmin(userID int64) bool {
	_, ok := s.adminUsers[userID]
	return ok
}

func validateStatsRange(from, to time.Time) error {
	if !from.Before(to) {
		return ErrBadRequest
	}
	if to.Sub(from) > maxStatsRange {
		return ErrBadRequest
	}
	return nil
}
