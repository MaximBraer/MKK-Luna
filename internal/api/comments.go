package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"MKK-Luna/internal/api/middleware"
	"MKK-Luna/internal/service"
	"MKK-Luna/pkg/api/response"
)

type CommentHandler struct {
	tasks *service.TaskService
}

func NewCommentHandler(tasks *service.TaskService) *CommentHandler {
	return &CommentHandler{tasks: tasks}
}

type commentRequest struct {
	Body string `json:"body"`
}

type commentResponse struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"task_id"`
	UserID    int64  `json:"user_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Create godoc
// @Summary Create comment
// @Tags comments
// @Accept json
// @Produce json
// @Param id path int true "Task ID"
// @Param request body commentRequest true "Comment payload"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/tasks/{id}/comments [post]
func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req commentRequest
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Body) == "" {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	id, err := h.tasks.CreateComment(ctx, userID, taskID, strings.TrimSpace(req.Body))
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.JSON(w, http.StatusCreated, map[string]any{"status": "ok", "id": id})
}

// ListByTask godoc
// @Summary List comments by task
// @Tags comments
// @Produce json
// @Param id path int true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/tasks/{id}/comments [get]
func (h *CommentHandler) ListByTask(w http.ResponseWriter, r *http.Request) {
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

	items, err := h.tasks.ListComments(ctx, userID, taskID)
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]commentResponse, 0, len(items))
	for _, c := range items {
		resp = append(resp, commentResponse{
			ID:        c.ID,
			TaskID:    c.TaskID,
			UserID:    c.UserID,
			Body:      c.Body,
			CreatedAt: c.CreatedAt.Format(time.RFC3339Nano),
			UpdatedAt: c.UpdatedAt.Format(time.RFC3339Nano),
		})
	}
	response.JSON(w, http.StatusOK, map[string]any{"comments": resp})
}

// Update godoc
// @Summary Patch comment
// @Tags comments
// @Accept json
// @Produce json
// @Param id path int true "Comment ID"
// @Param request body commentRequest true "Comment payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/comments/{id} [patch]
func (h *CommentHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	commentID, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil || commentID <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	var req commentRequest
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Body) == "" {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.tasks.UpdateComment(ctx, userID, commentID, strings.TrimSpace(req.Body)); err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// Delete godoc
// @Summary Delete comment
// @Tags comments
// @Produce json
// @Param id path int true "Comment ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/comments/{id} [delete]
func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	commentID, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil || commentID <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.tasks.DeleteComment(ctx, userID, commentID); err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
