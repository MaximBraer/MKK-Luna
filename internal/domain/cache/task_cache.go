package cache

import (
	"context"
)

type TaskCache interface {
	GetList(ctx context.Context, teamID int64, filters map[string]string) (data []byte, ok bool, err error)
	SetList(ctx context.Context, teamID int64, filters map[string]string, data []byte) error
	InvalidateTeam(ctx context.Context, teamID int64) error
}
