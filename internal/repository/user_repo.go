package repository

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

type User struct {
	ID           int64  `db:"id"`
	Email        string `db:"email"`
	Username     string `db:"username"`
	PasswordHash string `db:"password_hash"`
}

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, email, username, passwordHash string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (email, username, password_hash) VALUES (?, ?, ?)`,
		email, username, passwordHash,
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

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := r.db.GetContext(ctx, &u, `SELECT id, email, username, password_hash FROM users WHERE email = ?`, email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := r.db.GetContext(ctx, &u, `SELECT id, email, username, password_hash FROM users WHERE username = ?`, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}
