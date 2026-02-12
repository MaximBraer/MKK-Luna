package service

import (
	"context"
	"errors"
	"testing"

	"MKK-Luna/internal/repository"
)

func TestTeamService_EnsureMemberRole(t *testing.T) {
	tests := []struct {
		name    string
		teamFn  func(context.Context, int64) (*repository.Team, error)
		roleFn  func(context.Context, int64, int64) (string, bool, error)
		wantErr error
		want    string
	}{
		{
			name:    "team repo error",
			teamFn:  func(context.Context, int64) (*repository.Team, error) { return nil, errors.New("db") },
			wantErr: errors.New("db"),
		},
		{
			name:    "team not found",
			teamFn:  func(context.Context, int64) (*repository.Team, error) { return nil, nil },
			wantErr: ErrNotFound,
		},
		{
			name:    "member role repo error",
			teamFn:  func(context.Context, int64) (*repository.Team, error) { return &repository.Team{ID: 1}, nil },
			roleFn:  func(context.Context, int64, int64) (string, bool, error) { return "", false, errors.New("db2") },
			wantErr: errors.New("db2"),
		},
		{
			name:    "not member",
			teamFn:  func(context.Context, int64) (*repository.Team, error) { return &repository.Team{ID: 1}, nil },
			roleFn:  func(context.Context, int64, int64) (string, bool, error) { return "", false, nil },
			wantErr: ErrForbidden,
		},
		{
			name:   "success",
			teamFn: func(context.Context, int64) (*repository.Team, error) { return &repository.Team{ID: 1}, nil },
			roleFn: func(context.Context, int64, int64) (string, bool, error) { return RoleAdmin, true, nil },
			want:   RoleAdmin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewTeamService(
				nil,
				&fakeTeamStore{getByID: tt.teamFn},
				&fakeTeamMemberStore{getRole: tt.roleFn},
				&fakeUserStore{},
			)
			got, err := svc.EnsureMemberRole(context.Background(), 1, 1)
			switch {
			case tt.wantErr == nil && err != nil:
				t.Fatalf("unexpected err: %v", err)
			case tt.wantErr != nil && err == nil:
				t.Fatalf("expected err: %v", tt.wantErr)
			case tt.wantErr != nil && tt.wantErr.Error() != err.Error():
				t.Fatalf("want err=%v got=%v", tt.wantErr, err)
			}
			if tt.want != got {
				t.Fatalf("role=%q want=%q", got, tt.want)
			}
		})
	}
}
