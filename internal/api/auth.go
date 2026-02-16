package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"MKK-Luna/internal/domain/ratelimit"
	authinfra "MKK-Luna/internal/infra/auth"
	"MKK-Luna/internal/service"
	"MKK-Luna/pkg/api/response"
)

type AuthHandler struct {
	auth           *service.AuthService
	loginLimiter   ratelimit.Limiter
	refreshLimiter ratelimit.Limiter
	lockout        *authinfra.Lockout
}

type registerRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type registerResponse struct {
	Status string `json:"status"`
	ID     int64  `json:"id"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func NewAuthHandler(auth *service.AuthService, loginLimiter, refreshLimiter ratelimit.Limiter, lockout *authinfra.Lockout) *AuthHandler {
	return &AuthHandler{auth: auth, loginLimiter: loginLimiter, refreshLimiter: refreshLimiter, lockout: lockout}
}

// Register godoc
// @Summary Register user
// @Description Creates a new user account.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body registerRequest true "Register request"
// @Success 201 {object} registerResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 409 {object} response.ErrorResponse
// @Router /api/v1/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	id, err := h.auth.Register(ctx, strings.TrimSpace(req.Email), strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		if isDuplicate(err) {
			response.Error(w, http.StatusConflict, "conflict")
			return
		}
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	w.Header().Set("Location", "/api/v1/users/"+intToString(id))
	response.JSON(w, http.StatusCreated, registerResponse{Status: "ok", ID: id})
}

// Login godoc
// @Summary Login
// @Description Returns access and refresh tokens.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body loginRequest true "Login request"
// @Success 200 {object} tokenResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 429 {object} response.ErrorResponse
// @Router /api/v1/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if ok, retry := h.loginLimiter.Allow(r.Context(), ip); !ok {
		setRetryAfter(w, retry)
		response.Error(w, http.StatusTooManyRequests, "too many requests")
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	normalizedLogin := strings.TrimSpace(req.Login)
	if h.lockout != nil {
		normalized, err := h.lockout.Normalize(normalizedLogin)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid request")
			return
		}
		if locked, retry, err := h.lockout.IsLocked(ctx, normalized); err == nil && locked {
			setRetryAfter(w, retry)
			response.Error(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		normalizedLogin = normalized
	}

	pair, err := h.auth.Login(ctx, normalizedLogin, req.Password, ip, r.UserAgent())
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) && h.lockout != nil {
			if locked, retry, e := h.lockout.OnFailure(ctx, normalizedLogin); e == nil && locked {
				setRetryAfter(w, retry)
				response.Error(w, http.StatusTooManyRequests, "too many requests")
				return
			}
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			response.Error(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if h.lockout != nil {
		_ = h.lockout.OnSuccess(ctx, normalizedLogin)
	}

	response.JSON(w, http.StatusOK, tokenResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken})
}

// Refresh godoc
// @Summary Refresh tokens
// @Description Rotates refresh token and returns new access and refresh tokens.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body refreshRequest true "Refresh request"
// @Success 200 {object} tokenResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 429 {object} response.ErrorResponse
// @Router /api/v1/refresh [post]
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		response.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	userID, err := h.auth.ParseRefreshTokenUserID(req.RefreshToken)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}

	key := intToString(userID)
	if ok, retry := h.refreshLimiter.Allow(r.Context(), key); !ok {
		setRetryAfter(w, retry)
		response.Error(w, http.StatusTooManyRequests, "too many requests")
		return
	}

	pair, err := h.auth.Refresh(ctx, req.RefreshToken, clientIP(r), r.UserAgent())
	if err != nil {
		if errors.Is(err, service.ErrTokenReuse) || errors.Is(err, service.ErrInvalidToken) {
			response.Error(w, http.StatusUnauthorized, "invalid token")
			return
		}
		response.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}

	response.JSON(w, http.StatusOK, tokenResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken})
}

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1062
	}
	return false
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func setRetryAfter(w http.ResponseWriter, d time.Duration) {
	if d <= 0 {
		return
	}
	secs := int(math.Ceil(d.Seconds()))
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", intToString(int64(secs)))
}

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

func intToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
