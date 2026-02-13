package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"MKK-Luna/internal/repository"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

type TeamService struct {
	db      *sqlx.DB
	teams   teamStore
	members teamMemberStore
	users   userStore
	email   EmailSender
	locker  InviteLocker
	lockTTL time.Duration
	logger  *slog.Logger
	metrics TeamMetrics
}

type teamStore interface {
	CreateTx(ctx context.Context, tx *sqlx.Tx, name string, createdBy int64) (int64, error)
	GetByID(ctx context.Context, teamID int64) (*repository.Team, error)
	ListByUser(ctx context.Context, userID int64) ([]repository.Team, error)
}

type teamMemberStore interface {
	AddTx(ctx context.Context, tx *sqlx.Tx, teamID, userID int64, role string) error
	Add(ctx context.Context, teamID, userID int64, role string) error
	GetRole(ctx context.Context, teamID, userID int64) (string, bool, error)
	IsMember(ctx context.Context, teamID, userID int64) (bool, error)
}

type userStore interface {
	GetByEmail(ctx context.Context, email string) (*repository.User, error)
}

type EmailSender interface {
	SendInvite(ctx context.Context, toEmail, teamName string) error
}

type InviteLocker interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (token string, ok bool, err error)
	Release(ctx context.Context, key, token string) error
}

type TeamMetrics interface {
	IncLockReleaseError()
}

func NewTeamService(
	db *sqlx.DB,
	teams teamStore,
	members teamMemberStore,
	users userStore,
	emailSender EmailSender,
	locker InviteLocker,
	lockTTL time.Duration,
	logger *slog.Logger,
	metrics TeamMetrics,
) *TeamService {
	return &TeamService{
		db: db, teams: teams, members: members, users: users, email: emailSender,
		locker: locker, lockTTL: lockTTL, logger: logger, metrics: metrics,
	}
}

func (s *TeamService) CreateTeam(ctx context.Context, userID int64, name string) (int64, error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	teamID, err := s.teams.CreateTx(ctx, tx, name, userID)
	if err != nil {
		return 0, err
	}
	if err := s.members.AddTx(ctx, tx, teamID, userID, RoleOwner); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return teamID, nil
}

func (s *TeamService) ListTeams(ctx context.Context, userID int64) ([]repository.Team, error) {
	return s.teams.ListByUser(ctx, userID)
}

func (s *TeamService) EnsureMemberRole(ctx context.Context, teamID, userID int64) (string, error) {
	team, err := s.teams.GetByID(ctx, teamID)
	if err != nil {
		return "", err
	}
	if team == nil {
		return "", ErrNotFound
	}
	role, ok, err := s.members.GetRole(ctx, teamID, userID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrForbidden
	}
	return role, nil
}

func (s *TeamService) InviteByEmail(ctx context.Context, inviterID, teamID int64, email, role string) error {
	team, err := s.teams.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if team == nil {
		return ErrNotFound
	}

	inviterRole, ok, err := s.members.GetRole(ctx, teamID, inviterID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	if !canInvite(inviterRole, role) {
		return ErrForbidden
	}

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrNotFound
	}

	if s.locker != nil {
		lockKey := "lock:invite:" + strconv.FormatInt(teamID, 10) + ":" + strconv.FormatInt(user.ID, 10)
		lockToken, ok, err := s.locker.Acquire(ctx, lockKey, s.lockTTL)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("invite lock acquire failed, bypassing", "err", err, "team_id", teamID, "user_id", user.ID)
			}
		} else if !ok {
			return ErrConflict
		} else {
			defer func() {
				if err := s.locker.Release(context.Background(), lockKey, lockToken); err != nil {
					if s.logger != nil {
						s.logger.Warn("invite lock release failed", "err", err, "team_id", teamID, "user_id", user.ID)
					}
					if s.metrics != nil {
						s.metrics.IncLockReleaseError()
					}
				}
			}()
		}
	}

	if ok, err := s.members.IsMember(ctx, teamID, user.ID); err != nil {
		return err
	} else if ok {
		return ErrConflict
	}

	if s.email != nil {
		if err := s.email.SendInvite(ctx, email, team.Name); err != nil {
			return ErrUnavailable
		}
	}

	if err := s.members.Add(ctx, teamID, user.ID, role); err != nil {
		if isDuplicate(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

func canInvite(inviterRole, targetRole string) bool {
	switch inviterRole {
	case RoleOwner:
		return targetRole == RoleMember || targetRole == RoleAdmin
	case RoleAdmin:
		return targetRole == RoleMember
	default:
		return false
	}
}

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1062
	}
	return false
}
