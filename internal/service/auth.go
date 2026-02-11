package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"

	"MKK-Luna/internal/config"
	"MKK-Luna/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenReuse         = errors.New("token reuse")
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type AuthService struct {
	users    UserStore
	sessions SessionStore
	cfg      config.Config
	logger   *slog.Logger
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type TokenClaims struct {
	Type string `json:"type"`
	jwt.RegisteredClaims
}

type UserStore interface {
	Create(ctx context.Context, email, username, passwordHash string) (int64, error)
	GetByEmail(ctx context.Context, email string) (*repository.User, error)
	GetByUsername(ctx context.Context, username string) (*repository.User, error)
}

type SessionStore interface {
	Create(ctx context.Context, s *repository.Session) (int64, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*repository.Session, error)
	GetByTokenHashForUpdate(ctx context.Context, tx *sqlx.Tx, tokenHash string) (*repository.Session, error)
	Revoke(ctx context.Context, tokenHash string, revokedAt time.Time) error
	UpdateLastUsed(ctx context.Context, tokenHash string, ts time.Time) error
	RevokeAllByUser(ctx context.Context, userID int64, revokedAt time.Time) error
	GetActiveSessionsByUser(ctx context.Context, userID int64) ([]repository.Session, error)
	WithTx(ctx context.Context, fn func(*sqlx.Tx) error) error
	CreateWithTx(ctx context.Context, tx *sqlx.Tx, s *repository.Session) (int64, error)
	RevokeWithTx(ctx context.Context, tx *sqlx.Tx, tokenHash string, revokedAt time.Time) error
}

func NewAuthService(users UserStore, sessions SessionStore, cfg config.Config, logger *slog.Logger) (*AuthService, error) {
	if len(cfg.JWT.Secret) < 32 {
		return nil, errors.New("jwt secret must be at least 32 bytes")
	}
	if cfg.Auth.BcryptCost < 10 || cfg.Auth.BcryptCost > 14 {
		return nil, errors.New("bcrypt cost must be between 10 and 14")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AuthService{users: users, sessions: sessions, cfg: cfg, logger: logger}, nil
}

func (s *AuthService) Register(ctx context.Context, email, username, password string) (int64, error) {
	if err := validateEmail(email); err != nil {
		return 0, err
	}
	if err := validateUsername(username); err != nil {
		return 0, err
	}
	if err := validatePassword(password); err != nil {
		return 0, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cfg.Auth.BcryptCost)
	if err != nil {
		return 0, err
	}
	return s.users.Create(ctx, email, username, string(hash))
}

func (s *AuthService) Login(ctx context.Context, login, password, ip, userAgent string) (*TokenPair, error) {
	var user *repository.User
	var err error

	if strings.Contains(login, "@") {
		user, err = s.users.GetByEmail(ctx, login)
	} else {
		user, err = s.users.GetByUsername(ctx, login)
	}
	if err != nil || user == nil {
		return nil, ErrInvalidCredentials
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}

	pair, err := s.newTokenPair(user.ID)
	if err != nil {
		return nil, err
	}

	if err := s.createSession(ctx, user.ID, pair.RefreshToken, ip, userAgent); err != nil {
		return nil, err
	}

	s.logger.Info("auth_event",
		"event", "login",
		"user_id", user.ID,
		"ip", ip,
		"user_agent", userAgent,
	)

	return pair, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken, ip, userAgent string) (*TokenPair, error) {
	claims, err := s.parseToken(refreshToken, TokenTypeRefresh)
	if err != nil {
		return nil, ErrInvalidToken
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return nil, ErrInvalidToken
	}

	hash := hashToken(refreshToken)
	now := time.Now().UTC()

	var newPair *TokenPair
	var newID int64

	err = s.sessions.WithTx(ctx, func(tx *sqlx.Tx) error {
		session, err := s.sessions.GetByTokenHashForUpdate(ctx, tx, hash)
		if err != nil {
			return err
		}
		if session == nil {
			return ErrInvalidToken
		}
		if session.RevokedAt != nil {
			return ErrTokenReuse
		}
		if session.ExpiresAt.Before(now) {
			return ErrInvalidToken
		}

		newPair, err = s.newTokenPair(userID)
		if err != nil {
			return err
		}

		err = s.sessions.RevokeWithTx(ctx, tx, hash, now)
		if err != nil {
			return err
		}

		ua := nullableString(userAgent)
		ipPtr := nullableString(ip)

		newSession := &repository.Session{
			UserID:    userID,
			TokenHash: hashToken(newPair.RefreshToken),
			ExpiresAt: now.Add(s.cfg.JWT.RefreshTTL),
			UserAgent: ua,
			IP:        ipPtr,
		}
		newID, err := s.sessions.CreateWithTx(ctx, tx, newSession)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.logger.Debug("auth_event",
		"event", "refresh",
		"user_id", userID,
		"ip", ip,
		"user_agent", userAgent,
	)
	s.logger.Debug("auth_event",
		"event", "revoke",
		"reason", "refresh_rotation",
		"session_id", newID,
		"user_id", userID,
		"ip", ip,
		"user_agent", userAgent,
	)

	return newPair, nil
}

func (s *AuthService) newTokenPair(userID int64) (*TokenPair, error) {
	access, err := s.newToken(userID, TokenTypeAccess, s.cfg.JWT.AccessTTL)
	if err != nil {
		return nil, err
	}
	refresh, err := s.newToken(userID, TokenTypeRefresh, s.cfg.JWT.RefreshTTL)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

func (s *AuthService) newToken(userID int64, typ string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := TokenClaims{
		Type: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			Issuer:    s.cfg.JWT.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        uuid.NewString(),
		},
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(s.cfg.JWT.Secret))
}

func (s *AuthService) parseToken(tokenString, expectedType string) (*TokenClaims, error) {
	parser := jwt.NewParser(jwt.WithLeeway(s.cfg.JWT.ClockSkew))
	claims := &TokenClaims{}

	tok, err := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, ErrInvalidToken
		}
		return []byte(s.cfg.JWT.Secret), nil
	})
	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}
	if claims.Type != expectedType {
		return nil, ErrInvalidToken
	}
	if claims.Issuer != s.cfg.JWT.Issuer {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (s *AuthService) ParseAccessToken(tokenString string) (int64, error) {
	claims, err := s.parseToken(tokenString, TokenTypeAccess)
	if err != nil {
		return 0, ErrInvalidToken
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return userID, nil
}

func (s *AuthService) ParseRefreshTokenUserID(tokenString string) (int64, error) {
	claims, err := s.parseToken(tokenString, TokenTypeRefresh)
	if err != nil {
		return 0, ErrInvalidToken
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return userID, nil
}

func (s *AuthService) createSession(ctx context.Context, userID int64, refreshToken, ip, userAgent string) error {
	expiresAt := time.Now().UTC().Add(s.cfg.JWT.RefreshTTL)

	session := &repository.Session{
		UserID:    userID,
		TokenHash: hashToken(refreshToken),
		ExpiresAt: expiresAt,
		UserAgent: nullableString(userAgent),
		IP:        nullableString(ip),
	}
	id, err := s.sessions.Create(ctx, session)
	if err != nil {
		return err
	}
	s.logger.Debug("auth_event",
		"event", "session_created",
		"session_id", id,
		"user_id", userID,
		"ip", ip,
		"user_agent", userAgent,
	)
	return nil
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

var (
	emailRegex    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,100}$`)
)

func validateEmail(email string) error {
	if !emailRegex.MatchString(email) {
		return errors.New("invalid email")
	}
	return nil
}

func validateUsername(username string) error {
	if !usernameRegex.MatchString(username) {
		return errors.New("invalid username")
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 10 {
		return errors.New("invalid password")
	}
	hasLetter := false
	hasDigit := false
	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errors.New("invalid password")
	}
	return nil
}
