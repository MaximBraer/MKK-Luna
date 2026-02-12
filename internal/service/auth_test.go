package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"MKK-Luna/internal/config"
	"MKK-Luna/internal/repository"
)

type fakeUsers struct {
	user *repository.User
	id   int64
}

func (f *fakeUsers) Create(ctx context.Context, email, username, passwordHash string) (int64, error) {
	f.id = 1
	f.user = &repository.User{ID: 1, Email: email, Username: username, PasswordHash: passwordHash}
	return f.id, nil
}

func (f *fakeUsers) GetByEmail(ctx context.Context, email string) (*repository.User, error) {
	if f.user != nil && f.user.Email == email {
		return f.user, nil
	}
	return nil, nil
}

func (f *fakeUsers) GetByUsername(ctx context.Context, username string) (*repository.User, error) {
	if f.user != nil && f.user.Username == username {
		return f.user, nil
	}
	return nil, nil
}

type fakeSessions struct {
	sessions map[string]*repository.Session
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{sessions: make(map[string]*repository.Session)}
}

func (f *fakeSessions) Create(ctx context.Context, s *repository.Session) (int64, error) {
	f.sessions[s.TokenHash] = s
	return 1, nil
}

func (f *fakeSessions) CreateWithTx(ctx context.Context, _ *sqlx.Tx, s *repository.Session) (int64, error) {
	return f.Create(ctx, s)
}

func (f *fakeSessions) GetByTokenHash(ctx context.Context, tokenHash string) (*repository.Session, error) {
	return f.sessions[tokenHash], nil
}

func (f *fakeSessions) GetByTokenHashForUpdate(ctx context.Context, _ *sqlx.Tx, tokenHash string) (*repository.Session, error) {
	return f.sessions[tokenHash], nil
}

func (f *fakeSessions) Revoke(ctx context.Context, tokenHash string, revokedAt time.Time) error {
	if s := f.sessions[tokenHash]; s != nil {
		s.RevokedAt = &revokedAt
	}
	return nil
}

func (f *fakeSessions) RevokeWithTx(ctx context.Context, _ *sqlx.Tx, tokenHash string, revokedAt time.Time) error {
	return f.Revoke(ctx, tokenHash, revokedAt)
}

func (f *fakeSessions) UpdateLastUsed(ctx context.Context, tokenHash string, ts time.Time) error {
	if s := f.sessions[tokenHash]; s != nil {
		s.LastUsedAt = &ts
	}
	return nil
}

func (f *fakeSessions) RevokeAllByUser(ctx context.Context, userID int64, revokedAt time.Time) error {
	for _, s := range f.sessions {
		if s.UserID == userID && s.RevokedAt == nil {
			s.RevokedAt = &revokedAt
		}
	}
	return nil
}

func (f *fakeSessions) GetActiveSessionsByUser(ctx context.Context, userID int64) ([]repository.Session, error) {
	var out []repository.Session
	for _, s := range f.sessions {
		if s.UserID == userID && s.RevokedAt == nil {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (f *fakeSessions) WithTx(ctx context.Context, fn func(*sqlx.Tx) error) error {
	return fn(nil)
}

func baseConfig() config.Config {
	cfg := config.Config{}
	cfg.JWT.Secret = "change-me-please-change-me-please-32"
	cfg.JWT.AccessTTL = 15 * time.Minute
	cfg.JWT.RefreshTTL = 30 * 24 * time.Hour
	cfg.JWT.Issuer = "task-service"
	cfg.JWT.ClockSkew = time.Minute
	cfg.Auth.BcryptCost = 12
	return cfg
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "short", password: "short1", valid: false},
		{name: "no digit", password: "longpassword", valid: false},
		{name: "no letter", password: "1234567890", valid: false},
		{name: "valid", password: "Password123", valid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if tt.valid && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !tt.valid && err == nil {
				t.Fatalf("expected invalid password")
			}
		})
	}
}

func TestRegisterValidation(t *testing.T) {
	cfg := baseConfig()
	auth, _ := NewAuthService(&fakeUsers{}, newFakeSessions(), cfg, nil)

	tests := []struct {
		name     string
		email    string
		username string
		password string
		wantErr  bool
	}{
		{name: "bad email", email: "bad", username: "user1", password: "Password123", wantErr: true},
		{name: "bad username", email: "u@test.com", username: "bad name", password: "Password123", wantErr: true},
		{name: "bad password", email: "u@test.com", username: "user1", password: "short1", wantErr: true},
		{name: "ok", email: "u@test.com", username: "user1", password: "Password123", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.Register(context.Background(), tt.email, tt.username, tt.password)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	cfg := baseConfig()
	users := &fakeUsers{}
	sessions := newFakeSessions()
	auth, _ := NewAuthService(users, sessions, cfg, nil)

	cases := []struct {
		name  string
		login string
		pass  string
	}{
		{name: "unknown user", login: "u@test.com", pass: "Password123"},
		{name: "unknown username", login: "user1", pass: "Password123"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.Login(context.Background(), tt.login, tt.pass, "1.2.3.4", "ua")
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("expected invalid credentials")
			}
		})
	}
}

func TestRegisterLoginRefresh(t *testing.T) {
	cfg := baseConfig()
	users := &fakeUsers{}
	sessions := newFakeSessions()

	auth, err := NewAuthService(users, sessions, cfg, nil)
	if err != nil {
		t.Fatalf("auth init: %v", err)
	}

	_, err = auth.Register(context.Background(), "u@test.com", "user1", "Password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	pair, err := auth.Login(context.Background(), "u@test.com", "Password123", "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	_, err = auth.Refresh(context.Background(), pair.RefreshToken, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
}

func TestRefreshScenarios(t *testing.T) {
	cfg := baseConfig()
	users := &fakeUsers{}
	sessions := newFakeSessions()
	auth, _ := NewAuthService(users, sessions, cfg, nil)

	pair, err := auth.newTokenPair(1)
	if err != nil {
		t.Fatalf("token pair: %v", err)
	}

	revoked := time.Now().Add(-time.Minute)

	cases := []struct {
		name      string
		setup     func()
		expected  error
	}{
		{
			name: "reuse",
			setup: func() {
				sessions.sessions[hashToken(pair.RefreshToken)] = &repository.Session{
					UserID:    1,
					TokenHash: hashToken(pair.RefreshToken),
					ExpiresAt: time.Now().Add(time.Hour),
					RevokedAt: &revoked,
				}
			},
			expected: ErrTokenReuse,
		},
		{
			name: "expired",
			setup: func() {
				sessions.sessions[hashToken(pair.RefreshToken)] = &repository.Session{
					UserID:    1,
					TokenHash: hashToken(pair.RefreshToken),
					ExpiresAt: time.Now().Add(-time.Minute),
				}
			},
			expected: ErrInvalidToken,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			sessions.sessions = make(map[string]*repository.Session)
			tt.setup()
			_, err := auth.Refresh(context.Background(), pair.RefreshToken, "", "")
			if !errors.Is(err, tt.expected) {
				t.Fatalf("expected %v, got %v", tt.expected, err)
			}
		})
	}
}

func TestParseAccessTokenScenarios(t *testing.T) {
	cfg := baseConfig()
	auth, _ := NewAuthService(&fakeUsers{}, newFakeSessions(), cfg, nil)

	cases := []struct {
		name     string
		buildTok func() string
		wantErr  bool
	}{
		{
			name: "wrong type",
			buildTok: func() string {
				refresh, _ := auth.newToken(1, TokenTypeRefresh, time.Minute)
				return refresh
			},
			wantErr: true,
		},
		{
			name: "wrong issuer",
			buildTok: func() string {
				claims := TokenClaims{
					Type: TokenTypeAccess,
					RegisteredClaims: jwt.RegisteredClaims{
						Subject:   "1",
						Issuer:    "other",
						IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
						ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Minute)),
						ID:        uuid.NewString(),
					},
				}
				tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				str, _ := tok.SignedString([]byte(cfg.JWT.Secret))
				return str
			},
			wantErr: true,
		},
		{
			name: "invalid alg",
			buildTok: func() string {
				claims := TokenClaims{
					Type: TokenTypeAccess,
					RegisteredClaims: jwt.RegisteredClaims{
						Subject:   "1",
						Issuer:    cfg.JWT.Issuer,
						IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
						ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Minute)),
						ID:        uuid.NewString(),
					},
				}
				tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
				str, _ := tok.SignedString([]byte("bad"))
				return str
			},
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			str := tt.buildTok()
			_, err := auth.ParseAccessToken(str)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRevokeAllByUser(t *testing.T) {
	cases := []struct {
		name      string
		userID    int64
		otherID   int64
	}{
		{name: "revoke target", userID: 1, otherID: 2},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			sessions := newFakeSessions()
			now := time.Now()
			sessions.sessions["a"] = &repository.Session{UserID: tt.userID}
			sessions.sessions["b"] = &repository.Session{UserID: tt.userID}
			sessions.sessions["c"] = &repository.Session{UserID: tt.otherID}

			if err := sessions.RevokeAllByUser(context.Background(), tt.userID, now); err != nil {
				t.Fatalf("revoke all: %v", err)
			}
			if sessions.sessions["a"].RevokedAt == nil || sessions.sessions["b"].RevokedAt == nil {
				t.Fatalf("expected target sessions revoked")
			}
			if sessions.sessions["c"].RevokedAt != nil {
				t.Fatalf("expected other user session intact")
			}
		})
	}
}
