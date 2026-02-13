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

type TeamHandler struct {
	teams *service.TeamService
}

func NewTeamHandler(teams *service.TeamService) *TeamHandler {
	return &TeamHandler{teams: teams}
}

type createTeamRequest struct {
	Name string `json:"name"`
}

type teamResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Create godoc
// @Summary Create team
// @Tags teams
// @Accept json
// @Produce json
// @Param Idempotency-Key header string false "Idempotency key for safe retries"
// @Param request body createTeamRequest true "Create team"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Router /api/v1/teams [post]
func (h *TeamHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createTeamRequest
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	id, err := h.teams.CreateTeam(ctx, userID, strings.TrimSpace(req.Name))
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	response.JSON(w, http.StatusCreated, map[string]any{"status": "ok", "id": id})
}

// List godoc
// @Summary List teams
// @Tags teams
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/teams [get]
func (h *TeamHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	items, err := h.teams.ListTeams(ctx, userID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]teamResponse, 0, len(items))
	for _, t := range items {
		resp = append(resp, teamResponse{ID: t.ID, Name: t.Name})
	}
	response.JSON(w, http.StatusOK, map[string]any{"teams": resp})
}

// Invite godoc
// @Summary Invite user by email
// @Tags teams
// @Accept json
// @Produce json
// @Param id path int true "Team ID"
// @Param Idempotency-Key header string false "Idempotency key for safe retries"
// @Param request body inviteRequest true "Invite request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Failure 429 {object} response.ErrorResponse
// @Router /api/v1/teams/{id}/invite [post]
func (h *TeamHandler) Invite(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	teamID, err := parseInt64(chi.URLParam(r, "id"))
	if err != nil || teamID <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	var req inviteRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = service.RoleMember
	}
	if role != service.RoleMember && role != service.RoleAdmin {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.teams.InviteByEmail(ctx, userID, teamID, email, role); err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
