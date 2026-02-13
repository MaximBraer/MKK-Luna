package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"MKK-Luna/internal/repository"
)

type fakeTeamStore struct {
	getByID    func(ctx context.Context, teamID int64) (*repository.Team, error)
	createTx   func(ctx context.Context, tx *sqlx.Tx, name string, createdBy int64) (int64, error)
	listByUser func(ctx context.Context, userID int64) ([]repository.Team, error)
}

func (f *fakeTeamStore) CreateTx(ctx context.Context, tx *sqlx.Tx, name string, createdBy int64) (int64, error) {
	if f.createTx != nil {
		return f.createTx(ctx, tx, name, createdBy)
	}
	return 0, nil
}
func (f *fakeTeamStore) GetByID(ctx context.Context, teamID int64) (*repository.Team, error) {
	if f.getByID != nil {
		return f.getByID(ctx, teamID)
	}
	return nil, nil
}
func (f *fakeTeamStore) ListByUser(ctx context.Context, userID int64) ([]repository.Team, error) {
	if f.listByUser != nil {
		return f.listByUser(ctx, userID)
	}
	return nil, nil
}

type fakeTeamMemberStore struct {
	getRole  func(ctx context.Context, teamID, userID int64) (string, bool, error)
	isMember func(ctx context.Context, teamID, userID int64) (bool, error)
	add      func(ctx context.Context, teamID, userID int64, role string) error
	addTx    func(ctx context.Context, tx *sqlx.Tx, teamID, userID int64, role string) error
}

func (f *fakeTeamMemberStore) AddTx(ctx context.Context, tx *sqlx.Tx, teamID, userID int64, role string) error {
	if f.addTx != nil {
		return f.addTx(ctx, tx, teamID, userID, role)
	}
	return nil
}
func (f *fakeTeamMemberStore) Add(ctx context.Context, teamID, userID int64, role string) error {
	if f.add != nil {
		return f.add(ctx, teamID, userID, role)
	}
	return nil
}
func (f *fakeTeamMemberStore) GetRole(ctx context.Context, teamID, userID int64) (string, bool, error) {
	if f.getRole != nil {
		return f.getRole(ctx, teamID, userID)
	}
	return "", false, nil
}
func (f *fakeTeamMemberStore) IsMember(ctx context.Context, teamID, userID int64) (bool, error) {
	if f.isMember != nil {
		return f.isMember(ctx, teamID, userID)
	}
	return false, nil
}

type fakeUserStore struct {
	getByEmail func(ctx context.Context, email string) (*repository.User, error)
}

func (f *fakeUserStore) GetByEmail(ctx context.Context, email string) (*repository.User, error) {
	if f.getByEmail != nil {
		return f.getByEmail(ctx, email)
	}
	return nil, nil
}

type fakeEmailSender struct {
	err error
}

func (f fakeEmailSender) SendInvite(ctx context.Context, toEmail, teamName string) error {
	return f.err
}

type fakeInviteLocker struct {
	acquire func(ctx context.Context, key string, ttl time.Duration) (string, bool, error)
	release func(ctx context.Context, key, token string) error
}

func (f *fakeInviteLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	if f.acquire != nil {
		return f.acquire(ctx, key, ttl)
	}
	return "token", true, nil
}

func (f *fakeInviteLocker) Release(ctx context.Context, key, token string) error {
	if f.release != nil {
		return f.release(ctx, key, token)
	}
	return nil
}

func TestTeamService_InviteByEmail_Table(t *testing.T) {
	baseTeam := &repository.Team{ID: 1, Name: "team"}
	baseUser := &repository.User{ID: 99, Email: "u@test.com"}

	tests := []struct {
		name       string
		teamFn     func(ctx context.Context, teamID int64) (*repository.Team, error)
		roleFn     func(ctx context.Context, teamID, userID int64) (string, bool, error)
		userFn     func(ctx context.Context, email string) (*repository.User, error)
		isMemFn    func(ctx context.Context, teamID, userID int64) (bool, error)
		addFn      func(ctx context.Context, teamID, userID int64, role string) error
		emailErr   error
		locker     *fakeInviteLocker
		targetRole string
		wantErr    error
	}{
		{
			name:       "team not found",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return nil, nil },
			targetRole: RoleMember,
			wantErr:    ErrNotFound,
		},
		{
			name:       "team repo error",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return nil, errors.New("db") },
			targetRole: RoleMember,
			wantErr:    errors.New("db"),
		},
		{
			name:       "inviter not member",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return baseTeam, nil },
			roleFn:     func(context.Context, int64, int64) (string, bool, error) { return "", false, nil },
			targetRole: RoleMember,
			wantErr:    ErrForbidden,
		},
		{
			name:       "admin cannot invite admin",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return baseTeam, nil },
			roleFn:     func(context.Context, int64, int64) (string, bool, error) { return RoleAdmin, true, nil },
			targetRole: RoleAdmin,
			wantErr:    ErrForbidden,
		},
		{
			name:       "user not found",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return baseTeam, nil },
			roleFn:     func(context.Context, int64, int64) (string, bool, error) { return RoleOwner, true, nil },
			userFn:     func(context.Context, string) (*repository.User, error) { return nil, nil },
			targetRole: RoleMember,
			wantErr:    ErrNotFound,
		},
		{
			name:       "already member",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return baseTeam, nil },
			roleFn:     func(context.Context, int64, int64) (string, bool, error) { return RoleOwner, true, nil },
			userFn:     func(context.Context, string) (*repository.User, error) { return baseUser, nil },
			isMemFn:    func(context.Context, int64, int64) (bool, error) { return true, nil },
			targetRole: RoleMember,
			wantErr:    ErrConflict,
		},
		{
			name:       "duplicate key from repo",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return baseTeam, nil },
			roleFn:     func(context.Context, int64, int64) (string, bool, error) { return RoleOwner, true, nil },
			userFn:     func(context.Context, string) (*repository.User, error) { return baseUser, nil },
			isMemFn:    func(context.Context, int64, int64) (bool, error) { return false, nil },
			addFn:      func(context.Context, int64, int64, string) error { return &mysql.MySQLError{Number: 1062} },
			targetRole: RoleMember,
			wantErr:    ErrConflict,
		},
		{
			name:       "add repo error",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return baseTeam, nil },
			roleFn:     func(context.Context, int64, int64) (string, bool, error) { return RoleOwner, true, nil },
			userFn:     func(context.Context, string) (*repository.User, error) { return baseUser, nil },
			isMemFn:    func(context.Context, int64, int64) (bool, error) { return false, nil },
			addFn:      func(context.Context, int64, int64, string) error { return errors.New("write failed") },
			targetRole: RoleMember,
			wantErr:    errors.New("write failed"),
		},
		{
			name:       "email sender error",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return baseTeam, nil },
			roleFn:     func(context.Context, int64, int64) (string, bool, error) { return RoleOwner, true, nil },
			userFn:     func(context.Context, string) (*repository.User, error) { return baseUser, nil },
			isMemFn:    func(context.Context, int64, int64) (bool, error) { return false, nil },
			addFn:      func(context.Context, int64, int64, string) error { return nil },
			emailErr:   errors.New("email down"),
			targetRole: RoleMember,
			wantErr:    ErrUnavailable,
		},
		{
			name:       "invite lock already held",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return baseTeam, nil },
			roleFn:     func(context.Context, int64, int64) (string, bool, error) { return RoleOwner, true, nil },
			userFn:     func(context.Context, string) (*repository.User, error) { return baseUser, nil },
			locker:     &fakeInviteLocker{acquire: func(context.Context, string, time.Duration) (string, bool, error) { return "", false, nil }},
			targetRole: RoleMember,
			wantErr:    ErrConflict,
		},
		{
			name:    "invite lock redis error fallback",
			teamFn:  func(context.Context, int64) (*repository.Team, error) { return baseTeam, nil },
			roleFn:  func(context.Context, int64, int64) (string, bool, error) { return RoleOwner, true, nil },
			userFn:  func(context.Context, string) (*repository.User, error) { return baseUser, nil },
			isMemFn: func(context.Context, int64, int64) (bool, error) { return false, nil },
			addFn:   func(context.Context, int64, int64, string) error { return nil },
			locker: &fakeInviteLocker{acquire: func(context.Context, string, time.Duration) (string, bool, error) {
				return "", false, errors.New("redis down")
			}},
			targetRole: RoleMember,
			wantErr:    nil,
		},
		{
			name:       "success",
			teamFn:     func(context.Context, int64) (*repository.Team, error) { return baseTeam, nil },
			roleFn:     func(context.Context, int64, int64) (string, bool, error) { return RoleOwner, true, nil },
			userFn:     func(context.Context, string) (*repository.User, error) { return baseUser, nil },
			isMemFn:    func(context.Context, int64, int64) (bool, error) { return false, nil },
			addFn:      func(context.Context, int64, int64, string) error { return nil },
			targetRole: RoleMember,
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var locker InviteLocker
			if tt.locker != nil {
				locker = tt.locker
			}
			svc := NewTeamService(
				nil,
				&fakeTeamStore{getByID: tt.teamFn},
				&fakeTeamMemberStore{getRole: tt.roleFn, isMember: tt.isMemFn, add: tt.addFn},
				&fakeUserStore{getByEmail: tt.userFn},
				fakeEmailSender{err: tt.emailErr},
				locker,
				0,
				nil,
				nil,
			)
			err := svc.InviteByEmail(context.Background(), 1, 1, "u@test.com", tt.targetRole)
			switch {
			case tt.wantErr == nil && err != nil:
				t.Fatalf("unexpected err: %v", err)
			case tt.wantErr != nil && err == nil:
				t.Fatalf("expected err: %v", tt.wantErr)
			case tt.wantErr != nil && tt.wantErr.Error() != err.Error():
				t.Fatalf("want=%v got=%v", tt.wantErr, err)
			}
		})
	}
}
