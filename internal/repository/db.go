package repository

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/go-sql-driver/mysql"

	"MKK-Luna/internal/config"
)

func NewMySQL(cfg config.MySQLConfig) (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
	)

	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.MaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.MaxLifetime)
	} else {
		db.SetConnMaxLifetime(30 * time.Minute)
	}

	return db, nil
}
