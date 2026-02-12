package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
)

type TaskHistory struct {
	ID        int64           `db:"id"`
	TaskID    int64           `db:"task_id"`
	ChangedBy sql.NullInt64   `db:"changed_by"`
	FieldName string          `db:"field_name"`
	OldValue  json.RawMessage `db:"old_value"`
	NewValue  json.RawMessage `db:"new_value"`
	CreatedAt time.Time       `db:"created_at"`
}

type TaskHistoryCreate struct {
	TaskID    int64
	ChangedBy *int64
	FieldName string
	OldValue  *json.RawMessage
	NewValue  *json.RawMessage
}

type TaskHistoryRepository struct {
	db *sqlx.DB
}

func NewTaskHistoryRepository(db *sqlx.DB) *TaskHistoryRepository {
	return &TaskHistoryRepository{db: db}
}

func (r *TaskHistoryRepository) CreateBatchTx(ctx context.Context, tx *sqlx.Tx, entries []TaskHistoryCreate) error {
	if len(entries) == 0 {
		return nil
	}

	for _, e := range entries {
		var changedBy any
		if e.ChangedBy != nil {
			changedBy = *e.ChangedBy
		}
		var oldValue any
		if e.OldValue != nil {
			oldValue = []byte(*e.OldValue)
		}
		var newValue any
		if e.NewValue != nil {
			newValue = []byte(*e.NewValue)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_history (task_id, changed_by, field_name, old_value, new_value)
			VALUES (?, ?, ?, ?, ?)
		`, e.TaskID, changedBy, e.FieldName, oldValue, newValue); err != nil {
			return err
		}
	}
	return nil
}

func (r *TaskHistoryRepository) ListByTask(ctx context.Context, taskID int64, limit, offset int) ([]TaskHistory, int64, error) {
	var total int64
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM task_history WHERE task_id = ?`, taskID); err != nil {
		return nil, 0, err
	}

	items := make([]TaskHistory, 0)
	if err := r.db.SelectContext(ctx, &items, `
		SELECT id, task_id, changed_by, field_name, old_value, new_value, created_at
		FROM task_history
		WHERE task_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, taskID, limit, offset); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
