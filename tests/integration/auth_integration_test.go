//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/testcontainers/testcontainers-go/modules/mysql"

	"MKK-Luna/internal/config"
	"MKK-Luna/internal/repository"
	"MKK-Luna/internal/service"
)

func TestAuthIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run")
	}

	ctx := context.Background()
	mysqlC, err := mysql.RunContainer(ctx,
		mysql.WithDatabase("mkk_luna_test"),
		mysql.WithUsername("root"),
		mysql.WithPassword("root"),
	)
	if err != nil {
		t.Fatalf("mysql container: %v", err)
	}
	defer mysqlC.Terminate(ctx)

	host, err := mysqlC.Host(ctx)
	if err != nil {
		t.Fatalf("mysql host: %v", err)
	}
	port, err := mysqlC.MappedPort(ctx, "3306")
	if err != nil {
		t.Fatalf("mysql port: %v", err)
	}

	dsn := "root:root@tcp(" + host + ":" + port.Port() + ")/mkk_luna_test?parseTime=true&multiStatements=true"

	migrationsPath, err := findMigrationsPath()
	if err != nil {
		t.Fatalf("migrations path: %v", err)
	}

	m, err := migrate.New(toFileURL(migrationsPath), "mysql://"+dsn)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate.Up: %v", err)
	}

	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sqlx open: %v", err)
	}
	defer db.Close()

	cfg := config.Config{}
	cfg.JWT.Secret = "change-me-please-change-me-please-32"
	cfg.JWT.AccessTTL = 15 * time.Minute
	cfg.JWT.RefreshTTL = 30 * 24 * time.Hour
	cfg.JWT.Issuer = "task-service"
	cfg.JWT.ClockSkew = time.Minute
	cfg.Auth.BcryptCost = 12

	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	auth, err := service.NewAuthService(users, sessions, cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("auth init: %v", err)
	}

	_, err = auth.Register(ctx, "u@test.com", "user1", "Password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	pair, err := auth.Login(ctx, "u@test.com", "Password123", "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	_, err = auth.Refresh(ctx, pair.RefreshToken, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// reuse refresh should fail
	_, err = auth.Refresh(ctx, pair.RefreshToken, "1.2.3.4", "ua")
	if err == nil {
		t.Fatalf("expected reuse error")
	}
}

func findMigrationsPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(wd, "migrations")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return "", os.ErrNotExist
}

func toFileURL(path string) string {
	p := filepath.ToSlash(path)
	return "file://" + p
}
