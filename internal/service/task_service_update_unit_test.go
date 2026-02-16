package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"

	"MKK-Luna/internal/repository"
)

type fakeTaskRepo struct {
	getByID          func(ctx context.Context, taskID int64) (*repository.Task, error)
	getByIDForUpdate func(ctx context.Context, tx *sqlx.Tx, taskID int64) (*repository.Task, error)
	update           func(ctx context.Context, taskID int64, fields map[string]any) error
	updateTx         func(ctx context.Context, tx *sqlx.Tx, taskID int64, fields map[string]any) error
	deleteFn         func(ctx context.Context, taskID int64) error
	deleteTx         func(ctx context.Context, tx *sqlx.Tx, taskID int64) error
}

func (f *fakeTaskRepo) Create(context.Context, repository.Task) (int64, error) { return 0, nil }
func (f *fakeTaskRepo) GetByID(ctx context.Context, taskID int64) (*repository.Task, error) {
	return f.getByID(ctx, taskID)
}
func (f *fakeTaskRepo) GetByIDForUpdateTx(ctx context.Context, tx *sqlx.Tx, taskID int64) (*repository.Task, error) {
	if f.getByIDForUpdate != nil {
		return f.getByIDForUpdate(ctx, tx, taskID)
	}
	if f.getByID != nil {
		return f.getByID(ctx, taskID)
	}
	return nil, nil
}
func (f *fakeTaskRepo) List(context.Context, repository.TaskListFilter) ([]repository.Task, int64, error) {
	return nil, 0, nil
}
func (f *fakeTaskRepo) Update(ctx context.Context, taskID int64, fields map[string]any) error {
	if f.update != nil {
		return f.update(ctx, taskID, fields)
	}
	return nil
}
func (f *fakeTaskRepo) UpdateTx(ctx context.Context, tx *sqlx.Tx, taskID int64, fields map[string]any) error {
	if f.updateTx != nil {
		return f.updateTx(ctx, tx, taskID, fields)
	}
	if f.update != nil {
		return f.update(ctx, taskID, fields)
	}
	return nil
}
func (f *fakeTaskRepo) Delete(ctx context.Context, taskID int64) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, taskID)
	}
	return nil
}
func (f *fakeTaskRepo) DeleteTx(ctx context.Context, tx *sqlx.Tx, taskID int64) error {
	if f.deleteTx != nil {
		return f.deleteTx(ctx, tx, taskID)
	}
	if f.deleteFn != nil {
		return f.deleteFn(ctx, taskID)
	}
	return nil
}

type fakeMemberRepo struct {
	role     string
	hasRole  bool
	getRole  func(ctx context.Context, teamID, userID int64) (string, bool, error)
	isMember func(ctx context.Context, teamID, userID int64) (bool, error)
}

func (f *fakeMemberRepo) GetRole(ctx context.Context, teamID, userID int64) (string, bool, error) {
	if f.getRole != nil {
		return f.getRole(ctx, teamID, userID)
	}
	return f.role, f.hasRole, nil
}
func (f *fakeMemberRepo) IsMember(ctx context.Context, teamID, userID int64) (bool, error) {
	if f.isMember != nil {
		return f.isMember(ctx, teamID, userID)
	}
	return true, nil
}

type fakeTeamRepo struct {
	getByID func(ctx context.Context, teamID int64) (*repository.Team, error)
}

func (f *fakeTeamRepo) GetByID(ctx context.Context, teamID int64) (*repository.Team, error) {
	if f.getByID != nil {
		return f.getByID(ctx, teamID)
	}
	return nil, nil
}

type fakeCommentRepo struct{}

func (f *fakeCommentRepo) Create(context.Context, int64, int64, string) (int64, error) {
	return 0, nil
}
func (f *fakeCommentRepo) ListByTask(context.Context, int64) ([]repository.TaskComment, error) {
	return nil, nil
}
func (f *fakeCommentRepo) GetByID(context.Context, int64) (*repository.TaskComment, error) {
	return nil, nil
}
func (f *fakeCommentRepo) Update(context.Context, int64, string) error { return nil }
func (f *fakeCommentRepo) Delete(context.Context, int64) error         { return nil }

type fakeHistoryRepo struct {
	createBatchTx func(ctx context.Context, tx *sqlx.Tx, entries []repository.TaskHistoryCreate) error
	listByTask    func(ctx context.Context, taskID int64, limit, offset int) ([]repository.TaskHistory, int64, error)
}

func (f *fakeHistoryRepo) CreateBatchTx(ctx context.Context, tx *sqlx.Tx, entries []repository.TaskHistoryCreate) error {
	if f.createBatchTx != nil {
		return f.createBatchTx(ctx, tx, entries)
	}
	return nil
}

func (f *fakeHistoryRepo) ListByTask(ctx context.Context, taskID int64, limit, offset int) ([]repository.TaskHistory, int64, error) {
	if f.listByTask != nil {
		return f.listByTask(ctx, taskID, limit, offset)
	}
	return nil, 0, nil
}

func TestTaskService_UpdateTask_Table(t *testing.T) {
	baseTask := &repository.Task{ID: 1, TeamID: 10}

	tests := []struct {
		name       string
		role       string
		hasRole    bool
		raw        map[string]json.RawMessage
		isMember   func(context.Context, int64, int64) (bool, error)
		wantErr    error
		wantUpdate bool
	}{
		{name: "unknown field", role: RoleOwner, hasRole: true, raw: map[string]json.RawMessage{"x": json.RawMessage(`1`)}, wantErr: ErrBadRequest},
		{name: "member forbidden field", role: RoleMember, hasRole: true, raw: map[string]json.RawMessage{"title": json.RawMessage(`"x"`)}, wantErr: ErrForbidden},
		{name: "invalid status value", role: RoleMember, hasRole: true, raw: map[string]json.RawMessage{"status": json.RawMessage(`"bad"`)}, wantErr: ErrBadRequest},
		{name: "invalid status json", role: RoleMember, hasRole: true, raw: map[string]json.RawMessage{"status": json.RawMessage(`123`)}, wantErr: ErrBadRequest},
		{name: "invalid due_date", role: RoleOwner, hasRole: true, raw: map[string]json.RawMessage{"due_date": json.RawMessage(`"2026/01/01"`)}, wantErr: ErrBadRequest},
		{
			name:     "assignee outsider",
			role:     RoleMember,
			hasRole:  true,
			raw:      map[string]json.RawMessage{"assignee_id": json.RawMessage(`77`)},
			isMember: func(context.Context, int64, int64) (bool, error) { return false, nil },
			wantErr:  ErrBadRequest,
		},
		{name: "empty patch", role: RoleOwner, hasRole: true, raw: map[string]json.RawMessage{}, wantErr: ErrBadRequest},
		{name: "member valid status and null assignee", role: RoleMember, hasRole: true, raw: map[string]json.RawMessage{"status": json.RawMessage(`"done"`), "assignee_id": json.RawMessage(`null`)}, wantErr: nil, wantUpdate: true},
		{name: "owner full patch", role: RoleOwner, hasRole: true, raw: map[string]json.RawMessage{"title": json.RawMessage(`"new"`), "description": json.RawMessage(`"desc"`), "priority": json.RawMessage(`"high"`), "due_date": json.RawMessage(`"2026-01-01"`)}, wantErr: nil, wantUpdate: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := false
			taskRepo := &fakeTaskRepo{
				getByID: func(context.Context, int64) (*repository.Task, error) { return baseTask, nil },
				update: func(context.Context, int64, map[string]any) error {
					updated = true
					return nil
				},
			}
			members := &fakeMemberRepo{role: tt.role, hasRole: tt.hasRole, isMember: tt.isMember}
			svc := NewTaskService(nil, taskRepo, &fakeTeamRepo{}, members, &fakeCommentRepo{}, &fakeHistoryRepo{})

			_, err := svc.UpdateTask(context.Background(), 1, 1, tt.raw)
			if err != tt.wantErr {
				t.Fatalf("err=%v want=%v", err, tt.wantErr)
			}
			if updated != tt.wantUpdate {
				t.Fatalf("updated=%v want=%v", updated, tt.wantUpdate)
			}
		})
	}
}

func TestTaskService_UpdateTask_ErrorsBeforeValidation(t *testing.T) {
	svc := NewTaskService(
		nil,
		&fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return nil, nil }},
		&fakeTeamRepo{},
		&fakeMemberRepo{},
		&fakeCommentRepo{},
		&fakeHistoryRepo{},
	)
	if _, err := svc.UpdateTask(context.Background(), 1, 1, map[string]json.RawMessage{"status": json.RawMessage(`"done"`)}); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	svc = NewTaskService(
		nil,
		&fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }},
		&fakeTeamRepo{},
		&fakeMemberRepo{hasRole: false},
		&fakeCommentRepo{},
		&fakeHistoryRepo{},
	)
	if _, err := svc.UpdateTask(context.Background(), 1, 1, map[string]json.RawMessage{"status": json.RawMessage(`"done"`)}); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestTaskService_GetTaskHistory(t *testing.T) {
	svc := NewTaskService(
		nil,
		&fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }},
		&fakeTeamRepo{},
		&fakeMemberRepo{},
		&fakeCommentRepo{},
		nil,
	)
	if _, _, err := svc.GetTaskHistory(context.Background(), 1, 1, 20, 0); err != ErrBadRequest {
		t.Fatalf("expected ErrBadRequest, got %v", err)
	}

	svc = NewTaskService(
		nil,
		&fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return nil, nil }},
		&fakeTeamRepo{},
		&fakeMemberRepo{},
		&fakeCommentRepo{},
		&fakeHistoryRepo{},
	)
	if _, _, err := svc.GetTaskHistory(context.Background(), 1, 1, 20, 0); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	svc = NewTaskService(
		nil,
		&fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }},
		&fakeTeamRepo{},
		&fakeMemberRepo{isMember: func(context.Context, int64, int64) (bool, error) { return false, nil }},
		&fakeCommentRepo{},
		&fakeHistoryRepo{},
	)
	if _, _, err := svc.GetTaskHistory(context.Background(), 1, 1, 20, 0); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	items := []repository.TaskHistory{{ID: 1, TaskID: 1, FieldName: "status"}}
	svc = NewTaskService(
		nil,
		&fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }},
		&fakeTeamRepo{},
		&fakeMemberRepo{isMember: func(context.Context, int64, int64) (bool, error) { return true, nil }},
		&fakeCommentRepo{},
		&fakeHistoryRepo{listByTask: func(context.Context, int64, int, int) ([]repository.TaskHistory, int64, error) {
			return items, 1, nil
		}},
	)
	got, total, err := svc.GetTaskHistory(context.Background(), 1, 1, 20, 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if total != 1 || len(got) != 1 {
		t.Fatalf("unexpected history: total=%d len=%d", total, len(got))
	}
}
