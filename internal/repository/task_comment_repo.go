package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
)

type TaskComment struct {
	ID        int64     `db:"id"`
	TaskID    int64     `db:"task_id"`
	UserID    int64     `db:"user_id"`
	Body      string    `db:"body"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type TaskCommentRepository struct {
	db *sqlx.DB
}

func NewTaskCommentRepository(db *sqlx.DB) *TaskCommentRepository {
	return &TaskCommentRepository{db: db}
}

func (r *TaskCommentRepository) Create(ctx context.Context, taskID, userID int64, body string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO task_comments (task_id, user_id, body) VALUES (?, ?, ?)`,
		taskID, userID, body,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *TaskCommentRepository) ListByTask(ctx context.Context, taskID int64) ([]TaskComment, error) {
	var items []TaskComment
	err := r.db.SelectContext(ctx, &items, `
		SELECT id, task_id, user_id, body, created_at, updated_at
		FROM task_comments
		WHERE task_id = ?
		ORDER BY created_at ASC, id ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (r *TaskCommentRepository) GetByID(ctx context.Context, commentID int64) (*TaskComment, error) {
	var c TaskComment
	err := r.db.GetContext(ctx, &c, `
		SELECT id, task_id, user_id, body, created_at, updated_at
		FROM task_comments WHERE id = ?
	`, commentID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *TaskCommentRepository) Update(ctx context.Context, commentID int64, body string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE task_comments SET body = ? WHERE id = ?`, body, commentID)
	return err
}

func (r *TaskCommentRepository) Delete(ctx context.Context, commentID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM task_comments WHERE id = ?`, commentID)
	return err
}
