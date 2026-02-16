package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
)

type Team struct {
	ID        int64         `db:"id"`
	Name      string        `db:"name"`
	CreatedBy sql.NullInt64 `db:"created_by"`
	CreatedAt time.Time     `db:"created_at"`
}

type TeamRepository struct {
	db *sqlx.DB
}

func NewTeamRepository(db *sqlx.DB) *TeamRepository {
	return &TeamRepository{db: db}
}

func (r *TeamRepository) CreateTx(ctx context.Context, tx *sqlx.Tx, name string, createdBy int64) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO teams (name, created_by) VALUES (?, ?)`,
		name, createdBy,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *TeamRepository) GetByID(ctx context.Context, teamID int64) (*Team, error) {
	var t Team
	err := r.db.GetContext(ctx, &t, `SELECT id, name, created_by, created_at FROM teams WHERE id = ?`, teamID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (r *TeamRepository) ListByUser(ctx context.Context, userID int64) ([]Team, error) {
	var teams []Team
	err := r.db.SelectContext(ctx, &teams, `
		SELECT t.id, t.name, t.created_by, t.created_at
		FROM teams t
		JOIN team_members tm ON tm.team_id = t.id
		WHERE tm.user_id = ?
		ORDER BY t.id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	return teams, nil
}
