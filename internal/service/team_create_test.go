package service

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestTeamService_CreateTeam_BeginTxError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	beginErr := errors.New("begin failed")
	mock.ExpectBegin().WillReturnError(beginErr)

	svc := NewTeamService(sqlx.NewDb(db, "sqlmock"), &fakeTeamStore{}, &fakeTeamMemberStore{}, &fakeUserStore{}, nil)
	_, err = svc.CreateTeam(context.Background(), 1, "team")
	if err == nil || err.Error() != beginErr.Error() {
		t.Fatalf("expected begin error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestTeamService_CreateTeam_CreateTxError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	createErr := errors.New("create tx failed")
	svc := NewTeamService(
		sqlx.NewDb(db, "sqlmock"),
		&fakeTeamStore{
			createTx: func(context.Context, *sqlx.Tx, string, int64) (int64, error) {
				return 0, createErr
			},
		},
		&fakeTeamMemberStore{},
		&fakeUserStore{},
		nil,
	)
	_, err = svc.CreateTeam(context.Background(), 1, "team")
	if err == nil || err.Error() != createErr.Error() {
		t.Fatalf("expected create tx error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestTeamService_CreateTeam_AddMemberTxError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	addErr := errors.New("add member failed")
	svc := NewTeamService(
		sqlx.NewDb(db, "sqlmock"),
		&fakeTeamStore{
			createTx: func(context.Context, *sqlx.Tx, string, int64) (int64, error) {
				return 42, nil
			},
		},
		&fakeTeamMemberStore{
			addTx: func(context.Context, *sqlx.Tx, int64, int64, string) error {
				return addErr
			},
		},
		&fakeUserStore{},
		nil,
	)
	_, err = svc.CreateTeam(context.Background(), 1, "team")
	if err == nil || err.Error() != addErr.Error() {
		t.Fatalf("expected add tx error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestTeamService_CreateTeam_CommitError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	commitErr := errors.New("commit failed")
	mock.ExpectCommit().WillReturnError(commitErr)

	svc := NewTeamService(
		sqlx.NewDb(db, "sqlmock"),
		&fakeTeamStore{
			createTx: func(context.Context, *sqlx.Tx, string, int64) (int64, error) {
				return 42, nil
			},
		},
		&fakeTeamMemberStore{
			addTx: func(context.Context, *sqlx.Tx, int64, int64, string) error {
				return nil
			},
		},
		&fakeUserStore{},
		nil,
	)
	_, err = svc.CreateTeam(context.Background(), 1, "team")
	if err == nil || err.Error() != commitErr.Error() {
		t.Fatalf("expected commit error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
