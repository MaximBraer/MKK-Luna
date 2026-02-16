package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"

	"MKK-Luna/internal/config"
)

func newMockDB(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlx.NewDb(db, "sqlmock"), mock
}

func TestUserRepository(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewUserRepository(db)

	mock.ExpectExec("INSERT INTO users").
		WithArgs("a@test.com", "user", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	id, err := repo.Create(context.Background(), "a@test.com", "user", "hash")
	if err != nil || id != 1 {
		t.Fatalf("create err=%v id=%d", err, id)
	}

	rows := sqlmock.NewRows([]string{"id", "email", "username", "password_hash"}).
		AddRow(1, "a@test.com", "user", "hash")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, username, password_hash FROM users WHERE email = ?")).
		WithArgs("a@test.com").
		WillReturnRows(rows)
	u, err := repo.GetByEmail(context.Background(), "a@test.com")
	if err != nil || u == nil {
		t.Fatalf("get by email err=%v", err)
	}

	rows = sqlmock.NewRows([]string{"id", "email", "username", "password_hash"}).
		AddRow(2, "b@test.com", "user2", "hash")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, username, password_hash FROM users WHERE username = ?")).
		WithArgs("user2").
		WillReturnRows(rows)
	_, err = repo.GetByUsername(context.Background(), "user2")
	if err != nil {
		t.Fatalf("get by username err=%v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, username, password_hash FROM users WHERE username = ?")).
		WithArgs("noneuser").
		WillReturnError(sql.ErrNoRows)
	u, err = repo.GetByUsername(context.Background(), "noneuser")
	if err != nil || u != nil {
		t.Fatalf("expected nil user on no rows username, err=%v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, username, password_hash FROM users WHERE email = ?")).
		WithArgs("none@test.com").
		WillReturnError(sql.ErrNoRows)
	u, err = repo.GetByEmail(context.Background(), "none@test.com")
	if err != nil || u != nil {
		t.Fatalf("expected nil user on no rows, err=%v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTeamRepository(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewTeamRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO teams").
		WithArgs("team", int64(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	tx, _ := db.BeginTxx(context.Background(), nil)
	id, err := repo.CreateTx(context.Background(), tx, "team", 1)
	if err != nil || id != 1 {
		t.Fatalf("create tx err=%v id=%d", err, id)
	}
	_ = tx.Commit()

	rows := sqlmock.NewRows([]string{"id", "name", "created_by", "created_at"}).
		AddRow(1, "team", int64(1), time.Now())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, created_by, created_at FROM teams WHERE id = ?")).
		WithArgs(int64(1)).
		WillReturnRows(rows)
	team, err := repo.GetByID(context.Background(), 1)
	if err != nil || team == nil {
		t.Fatalf("get by id err=%v", err)
	}

	rows = sqlmock.NewRows([]string{"id", "name", "created_by", "created_at"}).
		AddRow(1, "team", int64(1), time.Now())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT t.id, t.name, t.created_by, t.created_at FROM teams t JOIN team_members tm ON tm.team_id = t.id WHERE tm.user_id = ?")).
		WithArgs(int64(1)).
		WillReturnRows(rows)
	_, err = repo.ListByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("list by user err=%v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTeamMemberRepository(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewTeamMemberRepository(db)

	mock.ExpectExec("INSERT INTO team_members").
		WithArgs(int64(1), int64(2), "member").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.Add(context.Background(), 1, 2, "member"); err != nil {
		t.Fatalf("add err=%v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO team_members").
		WithArgs(int64(1), int64(3), "admin").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	tx, _ := db.BeginTxx(context.Background(), nil)
	if err := repo.AddTx(context.Background(), tx, 1, 3, "admin"); err != nil {
		t.Fatalf("add tx err=%v", err)
	}
	_ = tx.Commit()

	rows := sqlmock.NewRows([]string{"role"}).AddRow("owner")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT role FROM team_members WHERE team_id = ? AND user_id = ?")).
		WithArgs(int64(1), int64(2)).
		WillReturnRows(rows)
	role, ok, err := repo.GetRole(context.Background(), 1, 2)
	if err != nil || !ok || role != "owner" {
		t.Fatalf("get role err=%v ok=%v role=%s", err, ok, role)
	}

	rows = sqlmock.NewRows([]string{"1"}).AddRow(1)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM team_members WHERE team_id = ? AND user_id = ?")).
		WithArgs(int64(1), int64(2)).
		WillReturnRows(rows)
	ok, err = repo.IsMember(context.Background(), 1, 2)
	if err != nil || !ok {
		t.Fatalf("is member err=%v ok=%v", err, ok)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM team_members WHERE team_id = ? AND user_id = ?")).
		WithArgs(int64(1), int64(99)).
		WillReturnError(sql.ErrNoRows)
	ok, err = repo.IsMember(context.Background(), 1, 99)
	if err != nil || ok {
		t.Fatalf("expected not member, err=%v ok=%v", err, ok)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskRepository(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewTaskRepository(db)

	mock.ExpectExec("INSERT INTO tasks").
		WillReturnResult(sqlmock.NewResult(1, 1))
	_, err := repo.Create(context.Background(), Task{TeamID: 1, Title: "t", Status: "todo", Priority: "medium"})
	if err != nil {
		t.Fatalf("create err=%v", err)
	}

	rows := sqlmock.NewRows([]string{"id", "team_id", "title", "description", "status", "priority", "assignee_id", "created_by", "due_date", "created_at", "updated_at"}).
		AddRow(1, 1, "t", nil, "todo", "medium", nil, nil, nil, time.Now(), time.Now())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, team_id, title, description, status, priority, assignee_id, created_by, due_date, created_at, updated_at FROM tasks WHERE id = ?")).
		WithArgs(int64(1)).
		WillReturnRows(rows)
	_, err = repo.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("get by id err=%v", err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, team_id, title, description, status, priority, assignee_id, created_by, due_date, created_at, updated_at FROM tasks WHERE id = ? FOR UPDATE")).
		WithArgs(int64(1)).
		WillReturnRows(rows)
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	_, err = repo.GetByIDForUpdateTx(context.Background(), tx, 1)
	if err != nil {
		t.Fatalf("get by id for update err=%v", err)
	}
	_ = tx.Rollback()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tasks WHERE team_id = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, team_id, title, description, status, priority, assignee_id, created_by, due_date, created_at, updated_at FROM tasks WHERE team_id = ? ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?")).
		WithArgs(int64(1), 10, 0).
		WillReturnRows(rows)
	_, _, err = repo.List(context.Background(), TaskListFilter{TeamID: 1, Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("list err=%v", err)
	}

	mock.ExpectExec("UPDATE tasks SET").
		WillReturnResult(sqlmock.NewResult(1, 1))
	err = repo.Update(context.Background(), 1, map[string]any{"status": "done"})
	if err != nil {
		t.Fatalf("update err=%v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET").
		WillReturnResult(sqlmock.NewResult(1, 1))
	tx, err = db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	err = repo.UpdateTx(context.Background(), tx, 1, map[string]any{"status": "done"})
	if err != nil {
		t.Fatalf("update tx err=%v", err)
	}
	_ = tx.Rollback()

	mock.ExpectExec("DELETE FROM tasks WHERE id = ?").
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.Delete(context.Background(), 1); err != nil {
		t.Fatalf("delete err=%v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM tasks WHERE id = ?").
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	tx, err = db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := repo.DeleteTx(context.Background(), tx, 1); err != nil {
		t.Fatalf("delete tx err=%v", err)
	}
	_ = tx.Rollback()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskHistoryRepository(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewTaskHistoryRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO task_history").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	tx, _ := db.BeginTxx(context.Background(), nil)
	entry := TaskHistoryCreate{TaskID: 1, FieldName: "status"}
	if err := repo.CreateBatchTx(context.Background(), tx, []TaskHistoryCreate{entry}); err != nil {
		t.Fatalf("create batch err=%v", err)
	}
	_ = tx.Commit()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM task_history WHERE task_id = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, task_id, changed_by, field_name, old_value, new_value, created_at FROM task_history WHERE task_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?")).
		WithArgs(int64(1), 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "changed_by", "field_name", "old_value", "new_value", "created_at"}).
			AddRow(1, 1, nil, "status", []byte("null"), []byte("null"), time.Now()))
	_, _, err := repo.ListByTask(context.Background(), 1, 10, 0)
	if err != nil {
		t.Fatalf("list by task err=%v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskCommentRepository(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewTaskCommentRepository(db)

	mock.ExpectExec("INSERT INTO task_comments").
		WithArgs(int64(1), int64(2), "body").
		WillReturnResult(sqlmock.NewResult(1, 1))
	_, err := repo.Create(context.Background(), 1, 2, "body")
	if err != nil {
		t.Fatalf("create err=%v", err)
	}

	rows := sqlmock.NewRows([]string{"id", "task_id", "user_id", "body", "created_at", "updated_at"}).
		AddRow(1, 1, 2, "body", time.Now(), time.Now())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, task_id, user_id, body, created_at, updated_at FROM task_comments WHERE task_id = ?")).
		WithArgs(int64(1)).
		WillReturnRows(rows)
	_, err = repo.ListByTask(context.Background(), 1)
	if err != nil {
		t.Fatalf("list err=%v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, task_id, user_id, body, created_at, updated_at FROM task_comments WHERE id = ?")).
		WithArgs(int64(1)).
		WillReturnRows(rows)
	_, err = repo.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("get err=%v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE task_comments SET body = ? WHERE id = ?")).
		WithArgs("new", int64(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.Update(context.Background(), 1, "new"); err != nil {
		t.Fatalf("update err=%v", err)
	}

	mock.ExpectExec("DELETE FROM task_comments WHERE id = ?").
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.Delete(context.Background(), 1); err != nil {
		t.Fatalf("delete err=%v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestSessionRepository(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewSessionRepository(db)

	mock.ExpectExec("INSERT INTO sessions").
		WillReturnResult(sqlmock.NewResult(1, 1))
	_, err := repo.Create(context.Background(), &Session{UserID: 1, TokenHash: "h", ExpiresAt: time.Now()})
	if err != nil {
		t.Fatalf("create err=%v", err)
	}

	rows := sqlmock.NewRows([]string{"id", "user_id", "token_hash", "expires_at", "revoked_at", "last_used_at", "user_agent", "ip", "created_at"}).
		AddRow(1, 1, "h", time.Now(), nil, nil, nil, nil, time.Now())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, token_hash, expires_at, revoked_at, last_used_at, user_agent, ip, created_at FROM sessions WHERE token_hash = ?")).
		WithArgs("h").
		WillReturnRows(rows)
	_, err = repo.GetByTokenHash(context.Background(), "h")
	if err != nil {
		t.Fatalf("get by token err=%v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE sessions SET revoked_at = ? WHERE token_hash = ?")).
		WithArgs(sqlmock.AnyArg(), "h").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.Revoke(context.Background(), "h", time.Now()); err != nil {
		t.Fatalf("revoke err=%v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE sessions SET last_used_at = ? WHERE token_hash = ?")).
		WithArgs(sqlmock.AnyArg(), "h").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.UpdateLastUsed(context.Background(), "h", time.Now()); err != nil {
		t.Fatalf("update last used err=%v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL")).
		WithArgs(sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.RevokeAllByUser(context.Background(), 1, time.Now()); err != nil {
		t.Fatalf("revoke all err=%v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, token_hash, expires_at, revoked_at, last_used_at, user_agent, ip, created_at FROM sessions WHERE user_id = ? AND revoked_at IS NULL")).
		WithArgs(int64(1)).
		WillReturnRows(rows)
	_, err = repo.GetActiveSessionsByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("get active err=%v", err)
	}

	mock.ExpectBegin()
	mock.ExpectCommit()
	if err := repo.WithTx(context.Background(), func(tx *sqlx.Tx) error { return nil }); err != nil {
		t.Fatalf("with tx err=%v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO sessions").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE sessions SET revoked_at = ? WHERE token_hash = ?")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := repo.WithTx(context.Background(), func(tx *sqlx.Tx) error {
		if _, err := repo.CreateWithTx(context.Background(), tx, &Session{UserID: 1, TokenHash: "h2", ExpiresAt: time.Now()}); err != nil {
			return err
		}
		return repo.RevokeWithTx(context.Background(), tx, "h2", time.Now())
	}); err != nil {
		t.Fatalf("with tx ops err=%v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestNewMySQL(t *testing.T) {
	cfg := config.MySQLConfig{
		Host:     "localhost",
		Port:     3306,
		DBName:   "test",
		User:     "root",
		Password: "root",
	}
	db, err := NewMySQL(cfg)
	if err != nil {
		t.Fatalf("NewMySQL err=%v", err)
	}
	_ = db.Close()
}
