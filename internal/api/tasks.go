package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"MKK-Luna/internal/api/middleware"
	"MKK-Luna/internal/domain/cache"
	"MKK-Luna/internal/repository"
	"MKK-Luna/internal/service"
	"MKK-Luna/pkg/api/response"
)

type TaskHandler struct {
	tasks *service.TaskService
	teams *service.TeamService
	cache cache.TaskCache
}

func NewTaskHandler(tasks *service.TaskService, teams *service.TeamService, cache cache.TaskCache) *TaskHandler {
	return &TaskHandler{tasks: tasks, teams: teams, cache: cache}
}

type createTaskRequest struct {
	TeamID      int64   `json:"team_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	Priority    string  `json:"priority"`
	AssigneeID  *int64  `json:"assignee_id"`
	DueDate     *string `json:"due_date"`
}

type taskResponse struct {
	ID          int64   `json:"id"`
	TeamID      int64   `json:"team_id"`
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	Status      string  `json:"status"`
	Priority    string  `json:"priority"`
	AssigneeID  *int64  `json:"assignee_id,omitempty"`
	CreatedBy   *int64  `json:"created_by,omitempty"`
	DueDate     *string `json:"due_date,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type listTasksResponse struct {
	Items  []taskResponse `json:"items"`
	Total  int64          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type taskHistoryItemResponse struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"task_id"`
	ChangedBy *int64 `json:"changed_by,omitempty"`
	FieldName string `json:"field_name"`
	OldValue  any    `json:"old_value,omitempty"`
	NewValue  any    `json:"new_value,omitempty"`
	CreatedAt string `json:"created_at"`
}

type listTaskHistoryResponse struct {
	Items  []taskHistoryItemResponse `json:"items"`
	Total  int64                     `json:"total"`
	Limit  int                       `json:"limit"`
	Offset int                       `json:"offset"`
}

// Create godoc
// @Summary Create task
// @Tags tasks
// @Accept json
// @Produce json
// @Param Idempotency-Key header string false "Idempotency key for safe retries"
// @Param request body createTaskRequest true "Create task"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Router /api/v1/tasks [post]
func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.TeamID <= 0 || strings.TrimSpace(req.Title) == "" {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	var due *time.Time
	if req.DueDate != nil {
		tm, err := time.Parse("2006-01-02", *req.DueDate)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid request")
			return
		}
		due = &tm
	}

	id, err := h.tasks.CreateTask(ctx, userID, service.CreateTaskInput{
		TeamID:      req.TeamID,
		Title:       strings.TrimSpace(req.Title),
		Description: req.Description,
		Status:      strings.TrimSpace(req.Status),
		Priority:    strings.TrimSpace(req.Priority),
		AssigneeID:  req.AssigneeID,
		DueDate:     due,
	})
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.cache != nil {
		_ = h.cache.InvalidateTeam(ctx, req.TeamID)
	}
	response.JSON(w, http.StatusCreated, map[string]any{"status": "ok", "id": id})
}

// List godoc
// @Summary List tasks
// @Tags tasks
// @Produce json
// @Param team_id query int true "Team ID"
// @Param status query string false "Status"
// @Param assignee_id query int false "Assignee ID"
// @Param limit query int false "Limit (max 100)"
// @Param offset query int false "Offset"
// @Success 200 {object} listTasksResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/tasks [get]
func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	teamID, err := parseInt64(r.URL.Query().Get("team_id"))
	if err != nil || teamID <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	_, err = h.teams.EnsureMemberRole(ctx, teamID, userID)
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	limit := parseQueryInt(r.URL.Query().Get("limit"), 20)
	if limit > 100 {
		limit = 100
	}
	offset := parseQueryInt(r.URL.Query().Get("offset"), 0)

	filters := map[string]string{
		"status":      r.URL.Query().Get("status"),
		"assignee_id": r.URL.Query().Get("assignee_id"),
		"limit":       strconv.Itoa(limit),
		"offset":      strconv.Itoa(offset),
	}

	if h.cache != nil {
		if data, ok, err := h.cache.GetList(ctx, teamID, filters); err == nil && ok {
			writeJSONBytes(w, http.StatusOK, data)
			return
		}
	}

	var status *string
	if v := strings.TrimSpace(r.URL.Query().Get("status")); v != "" {
		status = &v
	}
	var assigneeID *int64
	if v := strings.TrimSpace(r.URL.Query().Get("assignee_id")); v != "" {
		id, err := parseInt64(v)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid request")
			return
		}
		assigneeID = &id
	}

	items, total, err := h.tasks.ListTasks(ctx, userID, service.TaskListInput{
		TeamID:     teamID,
		Status:     status,
		AssigneeID: assigneeID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := listTasksResponse{
		Items:  make([]taskResponse, 0, len(items)),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	for _, t := range items {
		resp.Items = append(resp.Items, toTaskResponse(t))
	}

	data, _ := json.Marshal(resp)
	if h.cache != nil {
		_ = h.cache.SetList(ctx, teamID, filters, data)
	}
	writeJSONBytes(w, http.StatusOK, data)
}

// Get godoc
// @Summary Get task by id
// @Tags tasks
// @Produce json
// @Param id path int true "Task ID"
// @Success 200 {object} taskResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/tasks/{id} [get]
func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	taskID, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil || taskID <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	task, err := h.tasks.GetTask(ctx, userID, taskID)
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.JSON(w, http.StatusOK, toTaskResponse(*task))
}

// Update godoc
// @Summary Patch task
// @Tags tasks
// @Accept json
// @Produce json
// @Param id path int true "Task ID"
// @Param Idempotency-Key header string false "Idempotency key for safe retries"
// @Param request body map[string]interface{} true "Patch payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Router /api/v1/tasks/{id} [put]
func (h *TaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	taskID, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil || taskID <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	var raw map[string]json.RawMessage
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&raw); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if len(raw) == 0 {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	teamID, err := h.tasks.UpdateTask(ctx, userID, taskID, raw)
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.cache != nil {
		_ = h.cache.InvalidateTeam(ctx, teamID)
	}
	response.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// Delete godoc
// @Summary Delete task
// @Tags tasks
// @Produce json
// @Param id path int true "Task ID"
// @Param Idempotency-Key header string false "Idempotency key for safe retries"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Router /api/v1/tasks/{id} [delete]
func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	taskID, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil || taskID <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	teamID, err := h.tasks.DeleteTask(ctx, userID, taskID)
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if h.cache != nil {
		_ = h.cache.InvalidateTeam(ctx, teamID)
	}
	response.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// History godoc
// @Summary Get task history
// @Tags tasks
// @Produce json
// @Param id path int true "Task ID"
// @Param limit query int false "Limit (1..100)"
// @Param offset query int false "Offset (>=0)"
// @Success 200 {object} listTaskHistoryResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/tasks/{id}/history [get]
func (h *TaskHandler) History(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	taskID, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil || taskID <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	limit, err := parseStrictPositiveInt(r.URL.Query().Get("limit"), 20, 100)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	offset, err := parseStrictNonNegativeInt(r.URL.Query().Get("offset"), 0)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	items, total, err := h.tasks.GetTaskHistory(ctx, userID, taskID, limit, offset)
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := listTaskHistoryResponse{
		Items:  make([]taskHistoryItemResponse, 0, len(items)),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	for _, item := range items {
		resp.Items = append(resp.Items, toTaskHistoryResponse(item))
	}
	response.JSON(w, http.StatusOK, resp)
}

func parseQueryInt(v string, def int) int {
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func parseStrictPositiveInt(v string, def, max int) (int, error) {
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 || n > max {
		return 0, strconv.ErrSyntax
	}
	return n, nil
}

func parseStrictNonNegativeInt(v string, def int) (int, error) {
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, strconv.ErrSyntax
	}
	return n, nil
}

func toTaskResponse(t repository.Task) taskResponse {
	var desc *string
	if t.Description.Valid {
		desc = &t.Description.String
	}
	var assignee *int64
	if t.AssigneeID.Valid {
		assignee = &t.AssigneeID.Int64
	}
	var createdBy *int64
	if t.CreatedBy.Valid {
		createdBy = &t.CreatedBy.Int64
	}
	var due *string
	if t.DueDate.Valid {
		s := t.DueDate.Time.Format("2006-01-02")
		due = &s
	}
	return taskResponse{
		ID:          t.ID,
		TeamID:      t.TeamID,
		Title:       t.Title,
		Description: desc,
		Status:      t.Status,
		Priority:    t.Priority,
		AssigneeID:  assignee,
		CreatedBy:   createdBy,
		DueDate:     due,
		CreatedAt:   t.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:   t.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func toTaskHistoryResponse(h repository.TaskHistory) taskHistoryItemResponse {
	var changedBy *int64
	if h.ChangedBy.Valid {
		changedBy = &h.ChangedBy.Int64
	}

	var oldValue any
	if len(h.OldValue) > 0 {
		_ = json.Unmarshal(h.OldValue, &oldValue)
	}
	var newValue any
	if len(h.NewValue) > 0 {
		_ = json.Unmarshal(h.NewValue, &newValue)
	}

	return taskHistoryItemResponse{
		ID:        h.ID,
		TaskID:    h.TaskID,
		ChangedBy: changedBy,
		FieldName: h.FieldName,
		OldValue:  oldValue,
		NewValue:  newValue,
		CreatedAt: h.CreatedAt.Format(time.RFC3339Nano),
	}
}
