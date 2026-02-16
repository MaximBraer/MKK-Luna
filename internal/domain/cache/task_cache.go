package cache

import (
	"context"
	"time"

	"MKK-Luna/internal/repository"
)

type TaskCache interface {
	GetList(ctx context.Context, teamID int64, filters map[string]string) (data []byte, ok bool, err error)
	SetList(ctx context.Context, teamID int64, filters map[string]string, data []byte) error
	InvalidateTeam(ctx context.Context, teamID int64) error
}

type StatsCache interface {
	GetDone(ctx context.Context, userID int64, from, to time.Time) (items []repository.TeamDoneStat, ok bool, err error)
	SetDone(ctx context.Context, userID int64, from, to time.Time, items []repository.TeamDoneStat) error
	GetTop(ctx context.Context, userID int64, from, to time.Time, limit int) (items []repository.TeamTopCreator, ok bool, err error)
	SetTop(ctx context.Context, userID int64, from, to time.Time, limit int, items []repository.TeamTopCreator) error
}
