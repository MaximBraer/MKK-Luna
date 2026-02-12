package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type Task struct {
	ID          int64          `db:"id"`
	TeamID      int64          `db:"team_id"`
	Title       string         `db:"title"`
	Description sql.NullString `db:"description"`
	Status      string         `db:"status"`
	Priority    string         `db:"priority"`
	AssigneeID  sql.NullInt64  `db:"assignee_id"`
	CreatedBy   sql.NullInt64  `db:"created_by"`
	DueDate     sql.NullTime   `db:"due_date"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
}

type TaskRepository struct {
	db *sqlx.DB
}

func NewTaskRepository(db *sqlx.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(ctx context.Context, t Task) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO tasks (team_id, title, description, status, priority, assignee_id, created_by, due_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, t.TeamID, t.Title, nullableString(t.Description), t.Status, t.Priority, nullableInt64(t.AssigneeID), nullableInt64(t.CreatedBy), nullableTime(t.DueDate))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *TaskRepository) GetByID(ctx context.Context, taskID int64) (*Task, error) {
	var t Task
	err := r.db.GetContext(ctx, &t, `
		SELECT id, team_id, title, description, status, priority, assignee_id, created_by, due_date, created_at, updated_at
		FROM tasks WHERE id = ?
	`, taskID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

type TaskListFilter struct {
	TeamID     int64
	Status     *string
	AssigneeID *int64
	Limit      int
	Offset     int
}

func (r *TaskRepository) List(ctx context.Context, f TaskListFilter) ([]Task, int64, error) {
	where := []string{"team_id = ?"}
	args := []any{f.TeamID}
	if f.Status != nil {
		where = append(where, "status = ?")
		args = append(args, *f.Status)
	}
	if f.AssigneeID != nil {
		where = append(where, "assignee_id = ?")
		args = append(args, *f.AssigneeID)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	countSQL := "SELECT COUNT(*) FROM tasks WHERE " + whereSQL
	if err := r.db.GetContext(ctx, &total, countSQL, args...); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, team_id, title, description, status, priority, assignee_id, created_by, due_date, created_at, updated_at
		FROM tasks
		WHERE ` + whereSQL + `
		ORDER BY updated_at DESC, id DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, f.Limit, f.Offset)
	var tasks []Task
	if err := r.db.SelectContext(ctx, &tasks, query, args...); err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func (r *TaskRepository) Update(ctx context.Context, taskID int64, fields map[string]any) error {
	if len(fields) == 0 {
		return fmt.Errorf("no fields to update")
	}
	cols := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)
	for k, v := range fields {
		cols = append(cols, k+" = ?")
		args = append(args, v)
	}
	query := "UPDATE tasks SET " + strings.Join(cols, ", ") + " WHERE id = ?"
	args = append(args, taskID)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *TaskRepository) Delete(ctx context.Context, taskID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, taskID)
	return err
}

func nullableString(ns sql.NullString) any {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func nullableInt64(n sql.NullInt64) any {
	if n.Valid {
		return n.Int64
	}
	return nil
}

func nullableTime(nt sql.NullTime) any {
	if nt.Valid {
		return nt.Time
	}
	return nil
}
