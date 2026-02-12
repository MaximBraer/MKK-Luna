package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"MKK-Luna/internal/repository"
)

var (
	ErrUnknownField = errors.New("unknown field")
)

type TaskService struct {
	tasks    taskRepo
	teams    teamRepo
	members  teamMemberRepo
	comments taskCommentRepo
}

type taskRepo interface {
	Create(ctx context.Context, t repository.Task) (int64, error)
	GetByID(ctx context.Context, taskID int64) (*repository.Task, error)
	List(ctx context.Context, f repository.TaskListFilter) ([]repository.Task, int64, error)
	Update(ctx context.Context, taskID int64, fields map[string]any) error
	Delete(ctx context.Context, taskID int64) error
}

type teamRepo interface {
	GetByID(ctx context.Context, teamID int64) (*repository.Team, error)
}

type teamMemberRepo interface {
	GetRole(ctx context.Context, teamID, userID int64) (string, bool, error)
	IsMember(ctx context.Context, teamID, userID int64) (bool, error)
}

type taskCommentRepo interface {
	Create(ctx context.Context, taskID, userID int64, body string) (int64, error)
	ListByTask(ctx context.Context, taskID int64) ([]repository.TaskComment, error)
	GetByID(ctx context.Context, commentID int64) (*repository.TaskComment, error)
	Update(ctx context.Context, commentID int64, body string) error
	Delete(ctx context.Context, commentID int64) error
}

func NewTaskService(tasks taskRepo, teams teamRepo, members teamMemberRepo, comments taskCommentRepo) *TaskService {
	return &TaskService{
		tasks:    tasks,
		teams:    teams,
		members:  members,
		comments: comments,
	}
}

type CreateTaskInput struct {
	TeamID      int64
	Title       string
	Description string
	Status      string
	Priority    string
	AssigneeID  *int64
	DueDate     *time.Time
}

func (s *TaskService) CreateTask(ctx context.Context, userID int64, in CreateTaskInput) (int64, error) {
	team, err := s.teams.GetByID(ctx, in.TeamID)
	if err != nil {
		return 0, err
	}
	if team == nil {
		return 0, ErrNotFound
	}
	if ok, err := s.members.IsMember(ctx, in.TeamID, userID); err != nil {
		return 0, err
	} else if !ok {
		return 0, ErrForbidden
	}

	status := in.Status
	if status == "" {
		status = "todo"
	}
	if !isValidStatus(status) {
		return 0, ErrBadRequest
	}

	priority := in.Priority
	if priority == "" {
		priority = "medium"
	}
	if !isValidPriority(priority) {
		return 0, ErrBadRequest
	}

	if in.AssigneeID != nil {
		ok, err := s.members.IsMember(ctx, in.TeamID, *in.AssigneeID)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, ErrBadRequest
		}
	}

	var desc sql.NullString
	if strings.TrimSpace(in.Description) != "" {
		desc = sql.NullString{String: in.Description, Valid: true}
	}
	var assignee sql.NullInt64
	if in.AssigneeID != nil {
		assignee = sql.NullInt64{Int64: *in.AssigneeID, Valid: true}
	}
	var due sql.NullTime
	if in.DueDate != nil {
		due = sql.NullTime{Time: *in.DueDate, Valid: true}
	}

	task := repository.Task{
		TeamID:      in.TeamID,
		Title:       in.Title,
		Description: desc,
		Status:      status,
		Priority:    priority,
		AssigneeID:  assignee,
		CreatedBy:   sql.NullInt64{Int64: userID, Valid: true},
		DueDate:     due,
	}
	return s.tasks.Create(ctx, task)
}

func (s *TaskService) GetTask(ctx context.Context, userID, taskID int64) (*repository.Task, error) {
	task, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, ErrNotFound
	}
	if ok, err := s.members.IsMember(ctx, task.TeamID, userID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrForbidden
	}
	return task, nil
}

type TaskListInput struct {
	TeamID     int64
	Status     *string
	AssigneeID *int64
	Limit      int
	Offset     int
}

func (s *TaskService) ListTasks(ctx context.Context, userID int64, in TaskListInput) ([]repository.Task, int64, error) {
	team, err := s.teams.GetByID(ctx, in.TeamID)
	if err != nil {
		return nil, 0, err
	}
	if team == nil {
		return nil, 0, ErrNotFound
	}
	if ok, err := s.members.IsMember(ctx, in.TeamID, userID); err != nil {
		return nil, 0, err
	} else if !ok {
		return nil, 0, ErrForbidden
	}

	return s.tasks.List(ctx, repository.TaskListFilter{
		TeamID:     in.TeamID,
		Status:     in.Status,
		AssigneeID: in.AssigneeID,
		Limit:      in.Limit,
		Offset:     in.Offset,
	})
}

func (s *TaskService) UpdateTask(ctx context.Context, userID, taskID int64, raw map[string]json.RawMessage) (int64, error) {
	task, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		return 0, err
	}
	if task == nil {
		return 0, ErrNotFound
	}
	role, ok, err := s.members.GetRole(ctx, task.TeamID, userID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrForbidden
	}

	allowed := allowedTaskFields(role)
	for key := range raw {
		if !isKnownTaskField(key) {
			return 0, ErrBadRequest
		}
		if !allowed[key] {
			return 0, ErrForbidden
		}
	}

	fields := make(map[string]any)
	for key, val := range raw {
		switch key {
		case "title":
			var v string
			if err := json.Unmarshal(val, &v); err != nil || strings.TrimSpace(v) == "" {
				return 0, ErrBadRequest
			}
			fields["title"] = v
		case "description":
			var v *string
			if err := json.Unmarshal(val, &v); err != nil {
				return 0, ErrBadRequest
			}
			if v == nil {
				fields["description"] = nil
			} else {
				fields["description"] = *v
			}
		case "status":
			var v string
			if err := json.Unmarshal(val, &v); err != nil || !isValidStatus(v) {
				return 0, ErrBadRequest
			}
			fields["status"] = v
		case "priority":
			var v string
			if err := json.Unmarshal(val, &v); err != nil || !isValidPriority(v) {
				return 0, ErrBadRequest
			}
			fields["priority"] = v
		case "assignee_id":
			var v *int64
			if err := json.Unmarshal(val, &v); err != nil {
				return 0, ErrBadRequest
			}
			if v == nil {
				fields["assignee_id"] = nil
			} else {
				ok, err := s.members.IsMember(ctx, task.TeamID, *v)
				if err != nil {
					return 0, err
				}
				if !ok {
					return 0, ErrBadRequest
				}
				fields["assignee_id"] = *v
			}
		case "due_date":
			var v *string
			if err := json.Unmarshal(val, &v); err != nil {
				return 0, ErrBadRequest
			}
			if v == nil {
				fields["due_date"] = nil
			} else {
				tm, err := time.Parse("2006-01-02", *v)
				if err != nil {
					return 0, ErrBadRequest
				}
				fields["due_date"] = tm
			}
		}
	}

	if len(fields) == 0 {
		return 0, ErrBadRequest
	}
	if err := s.tasks.Update(ctx, taskID, fields); err != nil {
		return 0, err
	}
	return task.TeamID, nil
}

func (s *TaskService) DeleteTask(ctx context.Context, userID, taskID int64) (int64, error) {
	task, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		return 0, err
	}
	if task == nil {
		return 0, ErrNotFound
	}
	role, ok, err := s.members.GetRole(ctx, task.TeamID, userID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrForbidden
	}
	if role != RoleOwner && role != RoleAdmin {
		return 0, ErrForbidden
	}
	if err := s.tasks.Delete(ctx, taskID); err != nil {
		return 0, err
	}
	return task.TeamID, nil
}

func (s *TaskService) CreateComment(ctx context.Context, userID, taskID int64, body string) (int64, error) {
	task, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		return 0, err
	}
	if task == nil {
		return 0, ErrNotFound
	}
	if ok, err := s.members.IsMember(ctx, task.TeamID, userID); err != nil {
		return 0, err
	} else if !ok {
		return 0, ErrForbidden
	}
	return s.comments.Create(ctx, taskID, userID, body)
}

func (s *TaskService) ListComments(ctx context.Context, userID, taskID int64) ([]repository.TaskComment, error) {
	task, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, ErrNotFound
	}
	if ok, err := s.members.IsMember(ctx, task.TeamID, userID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrForbidden
	}
	return s.comments.ListByTask(ctx, taskID)
}

func (s *TaskService) UpdateComment(ctx context.Context, userID, commentID int64, body string) error {
	comment, err := s.comments.GetByID(ctx, commentID)
	if err != nil {
		return err
	}
	if comment == nil {
		return ErrNotFound
	}
	task, err := s.tasks.GetByID(ctx, comment.TaskID)
	if err != nil {
		return err
	}
	if task == nil {
		return ErrNotFound
	}
	role, ok, err := s.members.GetRole(ctx, task.TeamID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	if comment.UserID != userID && role != RoleOwner && role != RoleAdmin {
		return ErrForbidden
	}
	return s.comments.Update(ctx, commentID, body)
}

func (s *TaskService) DeleteComment(ctx context.Context, userID, commentID int64) error {
	comment, err := s.comments.GetByID(ctx, commentID)
	if err != nil {
		return err
	}
	if comment == nil {
		return ErrNotFound
	}
	task, err := s.tasks.GetByID(ctx, comment.TaskID)
	if err != nil {
		return err
	}
	if task == nil {
		return ErrNotFound
	}
	role, ok, err := s.members.GetRole(ctx, task.TeamID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	if comment.UserID != userID && role != RoleOwner && role != RoleAdmin {
		return ErrForbidden
	}
	return s.comments.Delete(ctx, commentID)
}

func isKnownTaskField(field string) bool {
	switch field {
	case "title", "description", "status", "assignee_id", "priority", "due_date":
		return true
	default:
		return false
	}
}

func allowedTaskFields(role string) map[string]bool {
	switch role {
	case RoleOwner, RoleAdmin:
		return map[string]bool{
			"title": true, "description": true, "status": true, "assignee_id": true, "priority": true, "due_date": true,
		}
	case RoleMember:
		return map[string]bool{
			"status": true, "assignee_id": true,
		}
	default:
		return map[string]bool{}
	}
}

func isValidStatus(v string) bool {
	return v == "todo" || v == "in_progress" || v == "done"
}

func isValidPriority(v string) bool {
	return v == "low" || v == "medium" || v == "high"
}
