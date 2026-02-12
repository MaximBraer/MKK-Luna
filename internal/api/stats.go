package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"MKK-Luna/internal/api/middleware"
	"MKK-Luna/internal/repository"
	"MKK-Luna/internal/service"
	"MKK-Luna/pkg/api/response"
)

type StatsHandler struct {
	stats *service.StatsService
}

func NewStatsHandler(stats *service.StatsService) *StatsHandler {
	return &StatsHandler{stats: stats}
}

type teamDoneStatResponse struct {
	TeamID       int64  `json:"team_id"`
	TeamName     string `json:"team_name"`
	MembersCount int64  `json:"members_count"`
	DoneCount    int64  `json:"done_count"`
}

type teamDoneStatsResponse struct {
	Items []teamDoneStatResponse `json:"items"`
}

type teamTopCreatorResponse struct {
	TeamID       int64 `json:"team_id"`
	UserID       int64 `json:"user_id"`
	CreatedCount int64 `json:"created_count"`
	Rank         int   `json:"rank"`
}

type teamTopCreatorsResponse struct {
	Items []teamTopCreatorResponse `json:"items"`
}

type taskIntegrityIssueResponse struct {
	TaskID     int64 `json:"task_id"`
	TeamID     int64 `json:"team_id"`
	AssigneeID int64 `json:"assignee_id"`
}

type taskIntegrityResponse struct {
	Items []taskIntegrityIssueResponse `json:"items"`
}

// TeamDoneStats godoc
// @Summary Team done stats
// @Description done_count = tasks with status=done and updated_at within the window
// @Tags stats
// @Produce json
// @Security BearerAuth
// @Param from query string true "RFC3339 UTC from"
// @Param to query string true "RFC3339 UTC to"
// @Success 200 {object} teamDoneStatsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/stats/teams/done [get]
func (h *StatsHandler) TeamDoneStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	from, to, err := parseFromToUTC(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	rows, err := h.stats.GetTeamDoneStats(ctx, userID, from, to)
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := teamDoneStatsResponse{Items: make([]teamDoneStatResponse, 0, len(rows))}
	for _, row := range rows {
		resp.Items = append(resp.Items, toTeamDoneStatResponse(row))
	}
	response.JSON(w, http.StatusOK, resp)
}

// TopCreators godoc
// @Summary Top creators by team
// @Tags stats
// @Produce json
// @Security BearerAuth
// @Param from query string true "RFC3339 UTC from"
// @Param to query string true "RFC3339 UTC to"
// @Param limit query int true "Limit (1..10)"
// @Success 200 {object} teamTopCreatorsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/stats/teams/top-creators [get]
func (h *StatsHandler) TopCreators(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	from, to, err := parseFromToUTC(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	limit, err := parseRequiredLimit(r.URL.Query().Get("limit"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	rows, err := h.stats.GetTopCreatorsByTeam(ctx, userID, from, to, limit)
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := teamTopCreatorsResponse{Items: make([]teamTopCreatorResponse, 0, len(rows))}
	for _, row := range rows {
		resp.Items = append(resp.Items, toTeamTopCreatorResponse(row))
	}
	response.JSON(w, http.StatusOK, resp)
}

// IntegrityTasks godoc
// @Summary Integrity issues for tasks
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} taskIntegrityResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/admin/integrity/tasks [get]
func (h *StatsHandler) IntegrityTasks(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.stats.FindTasksWithAssigneeNotMember(ctx, userID)
	if err != nil {
		if mapServiceError(w, err) {
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := taskIntegrityResponse{Items: make([]taskIntegrityIssueResponse, 0, len(rows))}
	for _, row := range rows {
		resp.Items = append(resp.Items, toTaskIntegrityResponse(row))
	}
	response.JSON(w, http.StatusOK, resp)
}

func parseFromToUTC(r *http.Request) (time.Time, time.Time, error) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		return time.Time{}, time.Time{}, errors.New("missing params")
	}
	from, err := parseRFC3339UTC(fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := parseRFC3339UTC(toStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to, nil
}

func parseRFC3339UTC(v string) (time.Time, error) {
	tm, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, err
	}
	if tm.Location() != time.UTC {
		return time.Time{}, errors.New("not utc")
	}
	return tm, nil
}

func parseRequiredLimit(v string) (int, error) {
	if v == "" {
		return 0, errors.New("missing limit")
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func toTeamDoneStatResponse(s repository.TeamDoneStat) teamDoneStatResponse {
	return teamDoneStatResponse{
		TeamID:       s.TeamID,
		TeamName:     s.TeamName,
		MembersCount: s.MembersCount,
		DoneCount:    s.DoneCount,
	}
}

func toTeamTopCreatorResponse(s repository.TeamTopCreator) teamTopCreatorResponse {
	return teamTopCreatorResponse{
		TeamID:       s.TeamID,
		UserID:       s.UserID,
		CreatedCount: s.CreatedCount,
		Rank:         s.Rank,
	}
}

func toTaskIntegrityResponse(s repository.TaskIntegrityIssue) taskIntegrityIssueResponse {
	return taskIntegrityIssueResponse{
		TaskID:     s.TaskID,
		TeamID:     s.TeamID,
		AssigneeID: s.AssigneeID,
	}
}
