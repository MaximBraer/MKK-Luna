package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"

	"MKK-Luna/internal/repository"
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

func TestTaskService_UpdateTask_Tx_NoOpRollback(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	task := &repository.Task{ID: 1, TeamID: 10, Title: "same", Status: "todo", Priority: "medium"}
	taskRepo := &fakeTaskRepo{
		getByIDForUpdate: func(context.Context, *sqlx.Tx, int64) (*repository.Task, error) { return task, nil },
	}
	members := &fakeMemberRepo{role: RoleOwner, hasRole: true}
	history := &fakeHistoryRepo{}

	svc := NewTaskService(db, taskRepo, &fakeTeamRepo{}, members, &fakeCommentRepo{}, history)
	teamID, err := svc.UpdateTask(context.Background(), 1, 1, map[string]json.RawMessage{"title": json.RawMessage(`"same"`)})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if teamID != task.TeamID {
		t.Fatalf("unexpected teamID: %d", teamID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskService_UpdateTask_Tx_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	task := &repository.Task{ID: 1, TeamID: 10, Title: "old", Status: "todo", Priority: "medium"}
	updated := false
	historyCreated := false
	taskRepo := &fakeTaskRepo{
		getByIDForUpdate: func(context.Context, *sqlx.Tx, int64) (*repository.Task, error) { return task, nil },
		updateTx: func(context.Context, *sqlx.Tx, int64, map[string]any) error {
			updated = true
			return nil
		},
	}
	history := &fakeHistoryRepo{
		createBatchTx: func(context.Context, *sqlx.Tx, []repository.TaskHistoryCreate) error {
			historyCreated = true
			return nil
		},
	}
	members := &fakeMemberRepo{role: RoleOwner, hasRole: true}

	svc := NewTaskService(db, taskRepo, &fakeTeamRepo{}, members, &fakeCommentRepo{}, history)
	_, err := svc.UpdateTask(context.Background(), 1, 1, map[string]json.RawMessage{"title": json.RawMessage(`"new"`)})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !updated || !historyCreated {
		t.Fatalf("expected update and history create")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskService_UpdateTask_Tx_UpdateError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	task := &repository.Task{ID: 1, TeamID: 10, Title: "old", Status: "todo", Priority: "medium"}
	taskRepo := &fakeTaskRepo{
		getByIDForUpdate: func(context.Context, *sqlx.Tx, int64) (*repository.Task, error) { return task, nil },
		updateTx: func(context.Context, *sqlx.Tx, int64, map[string]any) error {
			return errors.New("update err")
		},
	}
	history := &fakeHistoryRepo{}
	members := &fakeMemberRepo{role: RoleOwner, hasRole: true}

	svc := NewTaskService(db, taskRepo, &fakeTeamRepo{}, members, &fakeCommentRepo{}, history)
	if _, err := svc.UpdateTask(context.Background(), 1, 1, map[string]json.RawMessage{"title": json.RawMessage(`"new"`)}); err == nil {
		t.Fatalf("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskService_UpdateTask_Tx_HistoryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	task := &repository.Task{ID: 1, TeamID: 10, Title: "old", Status: "todo", Priority: "medium"}
	taskRepo := &fakeTaskRepo{
		getByIDForUpdate: func(context.Context, *sqlx.Tx, int64) (*repository.Task, error) { return task, nil },
		updateTx:         func(context.Context, *sqlx.Tx, int64, map[string]any) error { return nil },
	}
	history := &fakeHistoryRepo{
		createBatchTx: func(context.Context, *sqlx.Tx, []repository.TaskHistoryCreate) error {
			return errors.New("history err")
		},
	}
	members := &fakeMemberRepo{role: RoleOwner, hasRole: true}

	svc := NewTaskService(db, taskRepo, &fakeTeamRepo{}, members, &fakeCommentRepo{}, history)
	if _, err := svc.UpdateTask(context.Background(), 1, 1, map[string]json.RawMessage{"title": json.RawMessage(`"new"`)}); err == nil {
		t.Fatalf("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskService_DeleteTask_Tx_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	task := &repository.Task{ID: 1, TeamID: 10, Title: "t", Status: "done", Priority: "medium"}
	taskRepo := &fakeTaskRepo{
		getByIDForUpdate: func(context.Context, *sqlx.Tx, int64) (*repository.Task, error) { return task, nil },
		deleteTx:         func(context.Context, *sqlx.Tx, int64) error { return nil },
	}
	history := &fakeHistoryRepo{
		createBatchTx: func(context.Context, *sqlx.Tx, []repository.TaskHistoryCreate) error { return nil },
	}
	members := &fakeMemberRepo{role: RoleOwner, hasRole: true}

	svc := NewTaskService(db, taskRepo, &fakeTeamRepo{}, members, &fakeCommentRepo{}, history)
	teamID, err := svc.DeleteTask(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if teamID != task.TeamID {
		t.Fatalf("unexpected teamID: %d", teamID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskService_DeleteTask_Tx_ForbiddenRole(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	task := &repository.Task{ID: 1, TeamID: 10, Title: "t", Status: "done", Priority: "medium"}
	taskRepo := &fakeTaskRepo{
		getByIDForUpdate: func(context.Context, *sqlx.Tx, int64) (*repository.Task, error) { return task, nil },
	}
	history := &fakeHistoryRepo{}
	members := &fakeMemberRepo{role: RoleMember, hasRole: true}

	svc := NewTaskService(db, taskRepo, &fakeTeamRepo{}, members, &fakeCommentRepo{}, history)
	if _, err := svc.DeleteTask(context.Background(), 1, 1); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskService_DeleteTask_Tx_HistoryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	task := &repository.Task{ID: 1, TeamID: 10, Title: "t", Status: "done", Priority: "medium"}
	taskRepo := &fakeTaskRepo{
		getByIDForUpdate: func(context.Context, *sqlx.Tx, int64) (*repository.Task, error) { return task, nil },
	}
	history := &fakeHistoryRepo{
		createBatchTx: func(context.Context, *sqlx.Tx, []repository.TaskHistoryCreate) error {
			return errors.New("history err")
		},
	}
	members := &fakeMemberRepo{role: RoleOwner, hasRole: true}

	svc := NewTaskService(db, taskRepo, &fakeTeamRepo{}, members, &fakeCommentRepo{}, history)
	if _, err := svc.DeleteTask(context.Background(), 1, 1); err == nil {
		t.Fatalf("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskService_DeleteTask_Tx_DeleteError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	task := &repository.Task{ID: 1, TeamID: 10, Title: "t", Status: "done", Priority: "medium"}
	taskRepo := &fakeTaskRepo{
		getByIDForUpdate: func(context.Context, *sqlx.Tx, int64) (*repository.Task, error) { return task, nil },
		deleteTx: func(context.Context, *sqlx.Tx, int64) error {
			return errors.New("delete err")
		},
	}
	history := &fakeHistoryRepo{
		createBatchTx: func(context.Context, *sqlx.Tx, []repository.TaskHistoryCreate) error { return nil },
	}
	members := &fakeMemberRepo{role: RoleOwner, hasRole: true}

	svc := NewTaskService(db, taskRepo, &fakeTeamRepo{}, members, &fakeCommentRepo{}, history)
	if _, err := svc.DeleteTask(context.Background(), 1, 1); err == nil {
		t.Fatalf("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestTaskDeleteSnapshot(t *testing.T) {
	task := repository.Task{
		ID:       1,
		TeamID:   2,
		Title:    "t",
		Status:   "done",
		Priority: "high",
	}
	raw, err := taskDeleteSnapshot(task)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if raw == nil || len(*raw) == 0 {
		t.Fatalf("expected snapshot json")
	}
}
