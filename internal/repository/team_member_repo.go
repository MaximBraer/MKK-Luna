package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
)

type TeamMember struct {
	TeamID    int64     `db:"team_id"`
	UserID    int64     `db:"user_id"`
	Role      string    `db:"role"`
	CreatedAt time.Time `db:"created_at"`
}

type TeamMemberRepository struct {
	db *sqlx.DB
}

func NewTeamMemberRepository(db *sqlx.DB) *TeamMemberRepository {
	return &TeamMemberRepository{db: db}
}

func (r *TeamMemberRepository) AddTx(ctx context.Context, tx *sqlx.Tx, teamID, userID int64, role string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)`,
		teamID, userID, role,
	)
	return err
}

func (r *TeamMemberRepository) Add(ctx context.Context, teamID, userID int64, role string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)`,
		teamID, userID, role,
	)
	return err
}

func (r *TeamMemberRepository) GetRole(ctx context.Context, teamID, userID int64) (string, bool, error) {
	var role string
	err := r.db.GetContext(ctx, &role, `SELECT role FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return role, true, nil
}

func (r *TeamMemberRepository) IsMember(ctx context.Context, teamID, userID int64) (bool, error) {
	var v int
	err := r.db.GetContext(ctx, &v, `SELECT 1 FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
