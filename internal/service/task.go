package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"MKK-Luna/internal/repository"
)

var (
	ErrUnknownField = errors.New("unknown field")
)

type TaskService struct {
	db       *sqlx.DB
	tasks    taskRepo
	teams    teamRepo
	members  teamMemberRepo
	comments taskCommentRepo
	history  taskHistoryRepo
}

type taskRepo interface {
	Create(ctx context.Context, t repository.Task) (int64, error)
	GetByID(ctx context.Context, taskID int64) (*repository.Task, error)
	GetByIDForUpdateTx(ctx context.Context, tx *sqlx.Tx, taskID int64) (*repository.Task, error)
	List(ctx context.Context, f repository.TaskListFilter) ([]repository.Task, int64, error)
	Update(ctx context.Context, taskID int64, fields map[string]any) error
	UpdateTx(ctx context.Context, tx *sqlx.Tx, taskID int64, fields map[string]any) error
	Delete(ctx context.Context, taskID int64) error
	DeleteTx(ctx context.Context, tx *sqlx.Tx, taskID int64) error
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

type taskHistoryRepo interface {
	CreateBatchTx(ctx context.Context, tx *sqlx.Tx, entries []repository.TaskHistoryCreate) error
	ListByTask(ctx context.Context, taskID int64, limit, offset int) ([]repository.TaskHistory, int64, error)
}

func NewTaskService(db *sqlx.DB, tasks taskRepo, teams teamRepo, members teamMemberRepo, comments taskCommentRepo, history taskHistoryRepo) *TaskService {
	return &TaskService{
		db:       db,
		tasks:    tasks,
		teams:    teams,
		members:  members,
		comments: comments,
		history:  history,
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
	if s.db == nil || s.history == nil {
		return s.updateTaskNoTx(ctx, userID, taskID, raw)
	}

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	task, err := s.tasks.GetByIDForUpdateTx(ctx, tx, taskID)
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

	parsed, err := s.parseTaskPatch(ctx, task.TeamID, raw)
	if err != nil {
		return 0, err
	}

	allowed := allowedTaskFields(role)
	for key := range parsed {
		if !allowed[key] {
			return 0, ErrForbidden
		}
	}

	updates, entries := buildTaskDiffEntries(*task, userID, parsed)
	if len(updates) == 0 {
		return task.TeamID, nil
	}

	if err := s.tasks.UpdateTx(ctx, tx, taskID, updates); err != nil {
		return 0, err
	}
	if err := s.history.CreateBatchTx(ctx, tx, entries); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	return task.TeamID, nil
}

func (s *TaskService) updateTaskNoTx(ctx context.Context, userID, taskID int64, raw map[string]json.RawMessage) (int64, error) {
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

	parsed, err := s.parseTaskPatch(ctx, task.TeamID, raw)
	if err != nil {
		return 0, err
	}
	allowed := allowedTaskFields(role)
	for key := range parsed {
		if !allowed[key] {
			return 0, ErrForbidden
		}
	}

	updates, _ := buildTaskDiffEntries(*task, userID, parsed)
	if len(updates) == 0 {
		return task.TeamID, nil
	}
	if err := s.tasks.Update(ctx, taskID, updates); err != nil {
		return 0, err
	}
	return task.TeamID, nil
}

func (s *TaskService) DeleteTask(ctx context.Context, userID, taskID int64) (int64, error) {
	if s.db == nil || s.history == nil {
		return s.deleteTaskNoTx(ctx, userID, taskID)
	}

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	task, err := s.tasks.GetByIDForUpdateTx(ctx, tx, taskID)
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

	snapshot, err := taskDeleteSnapshot(*task)
	if err != nil {
		return 0, err
	}
	entry := repository.TaskHistoryCreate{
		TaskID:    task.ID,
		ChangedBy: &userID,
		FieldName: "task_deleted",
		OldValue:  snapshot,
		NewValue:  nil,
	}
	if err := s.history.CreateBatchTx(ctx, tx, []repository.TaskHistoryCreate{entry}); err != nil {
		return 0, err
	}
	if err := s.tasks.DeleteTx(ctx, tx, taskID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	return task.TeamID, nil
}

func (s *TaskService) deleteTaskNoTx(ctx context.Context, userID, taskID int64) (int64, error) {
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

func (s *TaskService) GetTaskHistory(ctx context.Context, userID, taskID int64, limit, offset int) ([]repository.TaskHistory, int64, error) {
	if s.history == nil {
		return nil, 0, ErrBadRequest
	}
	task, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		return nil, 0, err
	}
	if task == nil {
		return nil, 0, ErrNotFound
	}
	if ok, err := s.members.IsMember(ctx, task.TeamID, userID); err != nil {
		return nil, 0, err
	} else if !ok {
		return nil, 0, ErrForbidden
	}
	return s.history.ListByTask(ctx, taskID, limit, offset)
}

func (s *TaskService) parseTaskPatch(ctx context.Context, teamID int64, raw map[string]json.RawMessage) (map[string]any, error) {
	parsed := make(map[string]any, len(raw))
	for key, val := range raw {
		if !isKnownTaskField(key) {
			return nil, ErrBadRequest
		}

		switch key {
		case "title":
			var v string
			if err := json.Unmarshal(val, &v); err != nil || strings.TrimSpace(v) == "" {
				return nil, ErrBadRequest
			}
			parsed[key] = v
		case "description":
			var v *string
			if err := json.Unmarshal(val, &v); err != nil {
				return nil, ErrBadRequest
			}
			parsed[key] = v
		case "status":
			var v string
			if err := json.Unmarshal(val, &v); err != nil || !isValidStatus(v) {
				return nil, ErrBadRequest
			}
			parsed[key] = v
		case "priority":
			var v string
			if err := json.Unmarshal(val, &v); err != nil || !isValidPriority(v) {
				return nil, ErrBadRequest
			}
			parsed[key] = v
		case "assignee_id":
			var v *int64
			if err := json.Unmarshal(val, &v); err != nil {
				return nil, ErrBadRequest
			}
			if v != nil {
				ok, err := s.members.IsMember(ctx, teamID, *v)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, ErrBadRequest
				}
			}
			parsed[key] = v
		case "due_date":
			var v *string
			if err := json.Unmarshal(val, &v); err != nil {
				return nil, ErrBadRequest
			}
			if v == nil {
				parsed[key] = (*time.Time)(nil)
				continue
			}
			tm, err := time.Parse("2006-01-02", *v)
			if err != nil {
				return nil, ErrBadRequest
			}
			parsed[key] = &tm
		}
	}
	if len(parsed) == 0 {
		return nil, ErrBadRequest
	}
	return parsed, nil
}

func buildTaskDiffEntries(task repository.Task, userID int64, parsed map[string]any) (map[string]any, []repository.TaskHistoryCreate) {
	updates := make(map[string]any)
	entries := make([]repository.TaskHistoryCreate, 0, len(parsed))

	for key, val := range parsed {
		switch key {
		case "title":
			newValue := val.(string)
			if task.Title == newValue {
				continue
			}
			updates[key] = newValue
			entries = append(entries, taskHistoryEntry(task.ID, userID, key, task.Title, newValue))
		case "description":
			var oldValue any
			if task.Description.Valid {
				oldValue = task.Description.String
			}
			newPtr := val.(*string)
			if newPtr == nil {
				if !task.Description.Valid {
					continue
				}
				updates[key] = nil
				entries = append(entries, taskHistoryEntry(task.ID, userID, key, oldValue, nil))
				continue
			}
			if task.Description.Valid && task.Description.String == *newPtr {
				continue
			}
			updates[key] = *newPtr
			entries = append(entries, taskHistoryEntry(task.ID, userID, key, oldValue, *newPtr))
		case "status":
			newValue := val.(string)
			if task.Status == newValue {
				continue
			}
			updates[key] = newValue
			entries = append(entries, taskHistoryEntry(task.ID, userID, key, task.Status, newValue))
		case "priority":
			newValue := val.(string)
			if task.Priority == newValue {
				continue
			}
			updates[key] = newValue
			entries = append(entries, taskHistoryEntry(task.ID, userID, key, task.Priority, newValue))
		case "assignee_id":
			var oldValue any
			if task.AssigneeID.Valid {
				oldValue = task.AssigneeID.Int64
			}
			newPtr := val.(*int64)
			if newPtr == nil {
				if !task.AssigneeID.Valid {
					continue
				}
				updates[key] = nil
				entries = append(entries, taskHistoryEntry(task.ID, userID, key, oldValue, nil))
				continue
			}
			if task.AssigneeID.Valid && task.AssigneeID.Int64 == *newPtr {
				continue
			}
			updates[key] = *newPtr
			entries = append(entries, taskHistoryEntry(task.ID, userID, key, oldValue, *newPtr))
		case "due_date":
			var oldValue any
			if task.DueDate.Valid {
				oldValue = task.DueDate.Time.Format("2006-01-02")
			}
			newPtr := val.(*time.Time)
			if newPtr == nil {
				if !task.DueDate.Valid {
					continue
				}
				updates[key] = nil
				entries = append(entries, taskHistoryEntry(task.ID, userID, key, oldValue, nil))
				continue
			}
			newDate := newPtr.Format("2006-01-02")
			if task.DueDate.Valid && task.DueDate.Time.Format("2006-01-02") == newDate {
				continue
			}
			updates[key] = *newPtr
			entries = append(entries, taskHistoryEntry(task.ID, userID, key, oldValue, newDate))
		}
	}

	return updates, entries
}

func taskHistoryEntry(taskID, userID int64, field string, oldValue, newValue any) repository.TaskHistoryCreate {
	oldJSON := mustJSON(oldValue)
	newJSON := mustJSON(newValue)
	return repository.TaskHistoryCreate{
		TaskID:    taskID,
		ChangedBy: &userID,
		FieldName: field,
		OldValue:  oldJSON,
		NewValue:  newJSON,
	}
}

func taskDeleteSnapshot(task repository.Task) (*json.RawMessage, error) {
	payload := map[string]any{
		"id":          task.ID,
		"title":       task.Title,
		"description": nil,
		"status":      task.Status,
		"assignee_id": nil,
		"priority":    task.Priority,
		"due_date":    nil,
	}
	if task.Description.Valid {
		payload["description"] = task.Description.String
	}
	if task.AssigneeID.Valid {
		payload["assignee_id"] = task.AssigneeID.Int64
	}
	if task.DueDate.Valid {
		payload["due_date"] = task.DueDate.Time.Format("2006-01-02")
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	raw := json.RawMessage(b)
	return &raw, nil
}

func mustJSON(v any) *json.RawMessage {
	b, _ := json.Marshal(v)
	raw := json.RawMessage(b)
	return &raw
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
