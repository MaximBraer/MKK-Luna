package service

import (
	"context"
	"testing"

	"MKK-Luna/internal/repository"
)

type taskRepoWithCreate struct {
	fakeTaskRepo
	createFn func(ctx context.Context, t repository.Task) (int64, error)
	listFn   func(ctx context.Context, f repository.TaskListFilter) ([]repository.Task, int64, error)
	delFn    func(ctx context.Context, taskID int64) error
}

func (f *taskRepoWithCreate) Create(ctx context.Context, t repository.Task) (int64, error) {
	if f.createFn != nil {
		return f.createFn(ctx, t)
	}
	return 0, nil
}

func (f *taskRepoWithCreate) List(ctx context.Context, flt repository.TaskListFilter) ([]repository.Task, int64, error) {
	if f.listFn != nil {
		return f.listFn(ctx, flt)
	}
	return nil, 0, nil
}

func (f *taskRepoWithCreate) Delete(ctx context.Context, taskID int64) error {
	if f.delFn != nil {
		return f.delFn(ctx, taskID)
	}
	return nil
}

type commentRepoFns struct {
	createFn func(context.Context, int64, int64, string) (int64, error)
	listFn   func(context.Context, int64) ([]repository.TaskComment, error)
	getFn    func(context.Context, int64) (*repository.TaskComment, error)
	updateFn func(context.Context, int64, string) error
	deleteFn func(context.Context, int64) error
}

func (f *commentRepoFns) Create(ctx context.Context, taskID, userID int64, body string) (int64, error) {
	if f.createFn != nil {
		return f.createFn(ctx, taskID, userID, body)
	}
	return 0, nil
}
func (f *commentRepoFns) ListByTask(ctx context.Context, taskID int64) ([]repository.TaskComment, error) {
	if f.listFn != nil {
		return f.listFn(ctx, taskID)
	}
	return nil, nil
}
func (f *commentRepoFns) GetByID(ctx context.Context, commentID int64) (*repository.TaskComment, error) {
	if f.getFn != nil {
		return f.getFn(ctx, commentID)
	}
	return nil, nil
}
func (f *commentRepoFns) Update(ctx context.Context, commentID int64, body string) error {
	if f.updateFn != nil {
		return f.updateFn(ctx, commentID, body)
	}
	return nil
}
func (f *commentRepoFns) Delete(ctx context.Context, commentID int64) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, commentID)
	}
	return nil
}

func TestTaskService_CreateTask_Table(t *testing.T) {
	teamExists := &repository.Team{ID: 10, Name: "t"}
	memberRepo := &fakeMemberRepo{isMember: func(context.Context, int64, int64) (bool, error) { return true, nil }}

	tests := []struct {
		name    string
		teamFn  func(context.Context, int64) (*repository.Team, error)
		memFn   func(context.Context, int64, int64) (bool, error)
		input   CreateTaskInput
		wantErr error
	}{
		{name: "team not found", teamFn: func(context.Context, int64) (*repository.Team, error) { return nil, nil }, input: CreateTaskInput{TeamID: 10, Title: "x"}, wantErr: ErrNotFound},
		{name: "not member", teamFn: func(context.Context, int64) (*repository.Team, error) { return teamExists, nil }, memFn: func(context.Context, int64, int64) (bool, error) { return false, nil }, input: CreateTaskInput{TeamID: 10, Title: "x"}, wantErr: ErrForbidden},
		{name: "invalid status", teamFn: func(context.Context, int64) (*repository.Team, error) { return teamExists, nil }, input: CreateTaskInput{TeamID: 10, Title: "x", Status: "bad"}, wantErr: ErrBadRequest},
		{name: "invalid priority", teamFn: func(context.Context, int64) (*repository.Team, error) { return teamExists, nil }, input: CreateTaskInput{TeamID: 10, Title: "x", Priority: "bad"}, wantErr: ErrBadRequest},
		{name: "assignee outsider", teamFn: func(context.Context, int64) (*repository.Team, error) { return teamExists, nil }, memFn: func(context.Context, int64, int64) (bool, error) { return false, nil }, input: CreateTaskInput{TeamID: 10, Title: "x", AssigneeID: int64Ptr(5)}, wantErr: ErrForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			members := memberRepo
			if tt.memFn != nil {
				members = &fakeMemberRepo{isMember: tt.memFn}
			}
			svc := NewTaskService(
				&taskRepoWithCreate{},
				&fakeTeamRepo{getByID: tt.teamFn},
				members,
				&commentRepoFns{},
			)
			_, err := svc.CreateTask(context.Background(), 1, tt.input)
			if err != tt.wantErr {
				t.Fatalf("err=%v want=%v", err, tt.wantErr)
			}
		})
	}
}

func TestTaskService_CreateTask_DefaultsApplied(t *testing.T) {
	var got repository.Task
	svc := NewTaskService(
		&taskRepoWithCreate{
			createFn: func(_ context.Context, t repository.Task) (int64, error) {
				got = t
				return 1, nil
			},
		},
		&fakeTeamRepo{getByID: func(context.Context, int64) (*repository.Team, error) { return &repository.Team{ID: 1}, nil }},
		&fakeMemberRepo{isMember: func(context.Context, int64, int64) (bool, error) { return true, nil }},
		&commentRepoFns{},
	)

	_, err := svc.CreateTask(context.Background(), 11, CreateTaskInput{TeamID: 1, Title: "x"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Status != "todo" || got.Priority != "medium" {
		t.Fatalf("defaults not applied: status=%s priority=%s", got.Status, got.Priority)
	}
	if !got.CreatedBy.Valid || got.CreatedBy.Int64 != 11 {
		t.Fatalf("created_by not set")
	}
}

func TestTaskService_GetListDelete(t *testing.T) {
	svc := NewTaskService(
		&taskRepoWithCreate{
			fakeTaskRepo: fakeTaskRepo{
				getByID: func(context.Context, int64) (*repository.Task, error) {
					return &repository.Task{ID: 1, TeamID: 10}, nil
				},
			},
			listFn: func(_ context.Context, f repository.TaskListFilter) ([]repository.Task, int64, error) {
				return []repository.Task{{ID: 1, TeamID: f.TeamID}}, 1, nil
			},
			delFn: func(context.Context, int64) error { return nil },
		},
		&fakeTeamRepo{getByID: func(context.Context, int64) (*repository.Team, error) { return &repository.Team{ID: 10}, nil }},
		&fakeMemberRepo{
			role:    RoleOwner,
			hasRole: true,
			isMember: func(context.Context, int64, int64) (bool, error) {
				return true, nil
			},
		},
		&commentRepoFns{},
	)

	if _, err := svc.GetTask(context.Background(), 1, 999); err != nil {
	}
	items, total, err := svc.ListTasks(context.Background(), 1, TaskListInput{TeamID: 10, Limit: 10, Offset: 0})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("list failed: err=%v total=%d len=%d", err, total, len(items))
	}
	teamID, err := svc.DeleteTask(context.Background(), 1, 1)
	if err != nil || teamID != 10 {
		t.Fatalf("delete failed: err=%v teamID=%d", err, teamID)
	}
}

func TestTaskService_Comments(t *testing.T) {
	taskRepo := &taskRepoWithCreate{
		fakeTaskRepo: fakeTaskRepo{
			getByID: func(_ context.Context, taskID int64) (*repository.Task, error) {
				switch taskID {
				case 1:
					return &repository.Task{ID: 1, TeamID: 10}, nil
				default:
					return nil, nil
				}
			},
		},
	}
	comments := &commentRepoFns{
		createFn: func(context.Context, int64, int64, string) (int64, error) { return 7, nil },
		listFn: func(context.Context, int64) ([]repository.TaskComment, error) {
			return []repository.TaskComment{{ID: 1, TaskID: 1, UserID: 2}}, nil
		},
		getFn: func(context.Context, int64) (*repository.TaskComment, error) {
			return &repository.TaskComment{ID: 1, TaskID: 1, UserID: 2}, nil
		},
		updateFn: func(context.Context, int64, string) error { return nil },
		deleteFn: func(context.Context, int64) error { return nil },
	}

	svc := NewTaskService(
		taskRepo,
		&fakeTeamRepo{getByID: func(context.Context, int64) (*repository.Team, error) { return &repository.Team{ID: 10}, nil }},
		&fakeMemberRepo{
			role:    RoleOwner,
			hasRole: true,
			isMember: func(context.Context, int64, int64) (bool, error) {
				return true, nil
			},
		},
		comments,
	)

	if _, err := svc.CreateComment(context.Background(), 2, 1, "x"); err != nil {
		t.Fatalf("create comment err=%v", err)
	}
	if _, err := svc.ListComments(context.Background(), 2, 1); err != nil {
		t.Fatalf("list comments err=%v", err)
	}
	if err := svc.UpdateComment(context.Background(), 2, 1, "upd"); err != nil {
		t.Fatalf("update own comment err=%v", err)
	}
	if err := svc.DeleteComment(context.Background(), 2, 1); err != nil {
		t.Fatalf("delete own comment err=%v", err)
	}
}

func TestTaskService_ListComments_Errors(t *testing.T) {
	t.Run("task repo error", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return nil, errMock("task") }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{},
			&commentRepoFns{},
		)
		if _, err := svc.ListComments(context.Background(), 1, 1); err == nil || err.Error() != "task" {
			t.Fatalf("expected task error, got %v", err)
		}
	})

	t.Run("membership repo error", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{isMember: func(context.Context, int64, int64) (bool, error) { return false, errMock("member") }},
			&commentRepoFns{},
		)
		if _, err := svc.ListComments(context.Background(), 1, 1); err == nil || err.Error() != "member" {
			t.Fatalf("expected member error, got %v", err)
		}
	})

	t.Run("comment list repo error", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{isMember: func(context.Context, int64, int64) (bool, error) { return true, nil }},
			&commentRepoFns{listFn: func(context.Context, int64) ([]repository.TaskComment, error) { return nil, errMock("list") }},
		)
		if _, err := svc.ListComments(context.Background(), 1, 1); err == nil || err.Error() != "list" {
			t.Fatalf("expected list error, got %v", err)
		}
	})
}

func TestTaskService_CommentAndDeleteErrors(t *testing.T) {
	svc := NewTaskService(
		&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return nil, nil }}},
		&fakeTeamRepo{},
		&fakeMemberRepo{},
		&commentRepoFns{
			getFn: func(context.Context, int64) (*repository.TaskComment, error) { return nil, nil },
		},
	)
	if _, err := svc.CreateComment(context.Background(), 1, 1, "x"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound create comment, got %v", err)
	}
	if _, err := svc.ListComments(context.Background(), 1, 1); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound list comments, got %v", err)
	}
	if err := svc.UpdateComment(context.Background(), 1, 1, "x"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound update comment, got %v", err)
	}
	if err := svc.DeleteComment(context.Background(), 1, 1); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound delete comment, got %v", err)
	}
}

func TestTaskService_DeleteForbiddenAndNotFound(t *testing.T) {
	t.Run("task not found", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return nil, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{},
			&commentRepoFns{},
		)
		if _, err := svc.DeleteTask(context.Background(), 1, 1); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound got %v", err)
		}
	})

	t.Run("no role", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{hasRole: false},
			&commentRepoFns{},
		)
		if _, err := svc.DeleteTask(context.Background(), 1, 1); err != ErrForbidden {
			t.Fatalf("expected ErrForbidden got %v", err)
		}
	})

	t.Run("member role forbidden", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{role: RoleMember, hasRole: true},
			&commentRepoFns{},
		)
		if _, err := svc.DeleteTask(context.Background(), 1, 1); err != ErrForbidden {
			t.Fatalf("expected ErrForbidden got %v", err)
		}
	})
}

func TestTaskService_GetTaskForbidden(t *testing.T) {
	svc := NewTaskService(
		&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}},
		&fakeTeamRepo{},
		&fakeMemberRepo{isMember: func(context.Context, int64, int64) (bool, error) { return false, nil }},
		&commentRepoFns{},
	)
	if _, err := svc.GetTask(context.Background(), 1, 1); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden got %v", err)
	}
}

func TestTaskService_ListTaskErrors(t *testing.T) {
	svc := NewTaskService(
		&taskRepoWithCreate{},
		&fakeTeamRepo{getByID: func(context.Context, int64) (*repository.Team, error) { return nil, nil }},
		&fakeMemberRepo{},
		&commentRepoFns{},
	)
	if _, _, err := svc.ListTasks(context.Background(), 1, TaskListInput{TeamID: 1, Limit: 10, Offset: 0}); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound got %v", err)
	}

	svc = NewTaskService(
		&taskRepoWithCreate{},
		&fakeTeamRepo{getByID: func(context.Context, int64) (*repository.Team, error) { return &repository.Team{ID: 1}, nil }},
		&fakeMemberRepo{isMember: func(context.Context, int64, int64) (bool, error) { return false, nil }},
		&commentRepoFns{},
	)
	if _, _, err := svc.ListTasks(context.Background(), 1, TaskListInput{TeamID: 1, Limit: 10, Offset: 0}); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden got %v", err)
	}
}

func TestTaskService_CommentForbiddenCases(t *testing.T) {
	taskRepo := &taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}}
	comments := &commentRepoFns{
		getFn: func(context.Context, int64) (*repository.TaskComment, error) {
			return &repository.TaskComment{ID: 1, TaskID: 1, UserID: 9}, nil
		},
	}
	svc := NewTaskService(
		taskRepo,
		&fakeTeamRepo{},
		&fakeMemberRepo{role: RoleMember, hasRole: true, isMember: func(context.Context, int64, int64) (bool, error) { return false, nil }},
		comments,
	)
	if _, err := svc.CreateComment(context.Background(), 1, 1, "x"); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden create comment got %v", err)
	}

	svc = NewTaskService(
		taskRepo,
		&fakeTeamRepo{},
		&fakeMemberRepo{role: RoleMember, hasRole: true, isMember: func(context.Context, int64, int64) (bool, error) { return true, nil }},
		comments,
	)
	if err := svc.UpdateComment(context.Background(), 1, 1, "x"); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden update comment got %v", err)
	}
	if err := svc.DeleteComment(context.Background(), 1, 1); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden delete comment got %v", err)
	}
}

func int64Ptr(v int64) *int64 { return &v }

type errMock string

func (e errMock) Error() string { return string(e) }

func TestTaskService_UpdateComment_ErrorBranches(t *testing.T) {
	t.Run("comment repo error", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{},
			&fakeTeamRepo{},
			&fakeMemberRepo{},
			&commentRepoFns{getFn: func(context.Context, int64) (*repository.TaskComment, error) { return nil, errMock("cget") }},
		)
		if err := svc.UpdateComment(context.Background(), 1, 1, "x"); err == nil || err.Error() != "cget" {
			t.Fatalf("expected cget error, got %v", err)
		}
	})

	t.Run("task repo error", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return nil, errMock("tget") }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{},
			&commentRepoFns{getFn: func(context.Context, int64) (*repository.TaskComment, error) {
				return &repository.TaskComment{ID: 1, TaskID: 1, UserID: 1}, nil
			}},
		)
		if err := svc.UpdateComment(context.Background(), 1, 1, "x"); err == nil || err.Error() != "tget" {
			t.Fatalf("expected tget error, got %v", err)
		}
	})

	t.Run("role repo error", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{
				getRole: func(context.Context, int64, int64) (string, bool, error) { return "", false, errMock("role") },
			},
			&commentRepoFns{getFn: func(context.Context, int64) (*repository.TaskComment, error) {
				return &repository.TaskComment{ID: 1, TaskID: 1, UserID: 1}, nil
			}},
		)
		if err := svc.UpdateComment(context.Background(), 1, 1, "x"); err == nil || err.Error() != "role" {
			t.Fatalf("expected role error, got %v", err)
		}
	})

	t.Run("not member", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{
				getRole: func(context.Context, int64, int64) (string, bool, error) { return "", false, nil },
			},
			&commentRepoFns{getFn: func(context.Context, int64) (*repository.TaskComment, error) {
				return &repository.TaskComment{ID: 1, TaskID: 1, UserID: 1}, nil
			}},
		)
		if err := svc.UpdateComment(context.Background(), 1, 1, "x"); err != ErrForbidden {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("update repo error", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{
				getRole: func(context.Context, int64, int64) (string, bool, error) { return RoleOwner, true, nil },
			},
			&commentRepoFns{
				getFn: func(context.Context, int64) (*repository.TaskComment, error) {
					return &repository.TaskComment{ID: 1, TaskID: 1, UserID: 2}, nil
				},
				updateFn: func(context.Context, int64, string) error { return errMock("upd") },
			},
		)
		if err := svc.UpdateComment(context.Background(), 1, 1, "x"); err == nil || err.Error() != "upd" {
			t.Fatalf("expected upd error, got %v", err)
		}
	})
}

func TestTaskService_DeleteComment_ErrorBranches(t *testing.T) {
	t.Run("comment repo error", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{},
			&fakeTeamRepo{},
			&fakeMemberRepo{},
			&commentRepoFns{getFn: func(context.Context, int64) (*repository.TaskComment, error) { return nil, errMock("cget") }},
		)
		if err := svc.DeleteComment(context.Background(), 1, 1); err == nil || err.Error() != "cget" {
			t.Fatalf("expected cget error, got %v", err)
		}
	})

	t.Run("delete repo error", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{
				getRole: func(context.Context, int64, int64) (string, bool, error) { return RoleOwner, true, nil },
			},
			&commentRepoFns{
				getFn: func(context.Context, int64) (*repository.TaskComment, error) {
					return &repository.TaskComment{ID: 1, TaskID: 1, UserID: 2}, nil
				},
				deleteFn: func(context.Context, int64) error { return errMock("del") },
			},
		)
		if err := svc.DeleteComment(context.Background(), 1, 1); err == nil || err.Error() != "del" {
			t.Fatalf("expected del error, got %v", err)
		}
	})

	t.Run("task not found for comment task_id", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return nil, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{},
			&commentRepoFns{
				getFn: func(context.Context, int64) (*repository.TaskComment, error) {
					return &repository.TaskComment{ID: 1, TaskID: 777, UserID: 2}, nil
				},
			},
		)
		if err := svc.DeleteComment(context.Background(), 1, 1); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("role repo error", func(t *testing.T) {
		svc := NewTaskService(
			&taskRepoWithCreate{fakeTaskRepo: fakeTaskRepo{getByID: func(context.Context, int64) (*repository.Task, error) { return &repository.Task{ID: 1, TeamID: 1}, nil }}},
			&fakeTeamRepo{},
			&fakeMemberRepo{
				getRole: func(context.Context, int64, int64) (string, bool, error) { return "", false, errMock("role-del") },
			},
			&commentRepoFns{
				getFn: func(context.Context, int64) (*repository.TaskComment, error) {
					return &repository.TaskComment{ID: 1, TaskID: 1, UserID: 2}, nil
				},
			},
		)
		if err := svc.DeleteComment(context.Background(), 1, 1); err == nil || err.Error() != "role-del" {
			t.Fatalf("expected role-del error, got %v", err)
		}
	})
}
