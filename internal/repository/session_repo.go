package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
)

type Session struct {
	ID         int64      `db:"id"`
	UserID     int64      `db:"user_id"`
	TokenHash  string     `db:"token_hash"`
	ExpiresAt  time.Time  `db:"expires_at"`
	RevokedAt  *time.Time `db:"revoked_at"`
	LastUsedAt *time.Time `db:"last_used_at"`
	UserAgent  *string    `db:"user_agent"`
	IP         *string    `db:"ip"`
	CreatedAt  time.Time  `db:"created_at"`
}

type SessionRepository struct {
	db *sqlx.DB
}

func NewSessionRepository(db *sqlx.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, s *Session) (int64, error) {
	return createSession(ctx, r.db, s)
}

func (r *SessionRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	return getSession(ctx, r.db, tokenHash, false)
}

func (r *SessionRepository) GetByTokenHashForUpdate(ctx context.Context, tx *sqlx.Tx, tokenHash string) (*Session, error) {
	return getSession(ctx, tx, tokenHash, true)
}

func (r *SessionRepository) Revoke(ctx context.Context, tokenHash string, revokedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE token_hash = ?`, revokedAt, tokenHash)
	return err
}

func (r *SessionRepository) RevokeWithTx(ctx context.Context, tx *sqlx.Tx, tokenHash string, revokedAt time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE token_hash = ?`, revokedAt, tokenHash)
	return err
}

func (r *SessionRepository) UpdateLastUsed(ctx context.Context, tokenHash string, ts time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET last_used_at = ? WHERE token_hash = ?`, ts, tokenHash)
	return err
}

func (r *SessionRepository) RevokeAllByUser(ctx context.Context, userID int64, revokedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, revokedAt, userID)
	return err
}

func (r *SessionRepository) GetActiveSessionsByUser(ctx context.Context, userID int64) ([]Session, error) {
	var sessions []Session
	err := r.db.SelectContext(ctx, &sessions,
		`SELECT id, user_id, token_hash, expires_at, revoked_at, last_used_at, user_agent, ip, created_at
		 FROM sessions WHERE user_id = ? AND revoked_at IS NULL`, userID,
	)
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *SessionRepository) WithTx(ctx context.Context, fn func(*sqlx.Tx) error) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (r *SessionRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, s *Session) (int64, error) {
	return createSession(ctx, tx, s)
}

func createSession(ctx context.Context, exec sqlx.ExtContext, s *Session) (int64, error) {
	res, err := sqlx.NamedExecContext(ctx, exec,
		`INSERT INTO sessions (user_id, token_hash, expires_at, revoked_at, last_used_at, user_agent, ip)
		 VALUES (:user_id, :token_hash, :expires_at, :revoked_at, :last_used_at, :user_agent, :ip)`,
		s,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func getSession(ctx context.Context, q sqlx.QueryerContext, tokenHash string, forUpdate bool) (*Session, error) {
	var s Session
	query := `SELECT id, user_id, token_hash, expires_at, revoked_at, last_used_at, user_agent, ip, created_at
		 FROM sessions WHERE token_hash = ?`
	if forUpdate {
		query += " FOR UPDATE"
	}

	err := sqlx.GetContext(ctx, q, &s, query, tokenHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}
