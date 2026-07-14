// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
)

func TestUserStoreSetUserDisabledReportsQueryFailure(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v, want nil", err)
	}
	defer mock.Close()
	id := uuid.Must(uuid.NewV7())
	backend := errors.New("backend unavailable")
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE core.users").WithArgs(id, true).WillReturnError(backend)
	mock.ExpectRollback()

	err = newUserStore(mock).SetUserDisabled(t.Context(), id, true)

	if !errors.Is(err, backend) {
		t.Errorf("SetUserDisabled() error = %v, want it to wrap %v", err, backend)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUserStoreSetUserDisabledReportsSessionRevokeFailure(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v, want nil", err)
	}
	defer mock.Close()
	id := uuid.Must(uuid.NewV7())
	backend := errors.New("backend unavailable")
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE core.users").WithArgs(id, true).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM core.sessions").WithArgs(id).WillReturnError(backend)
	mock.ExpectRollback()

	err = newUserStore(mock).SetUserDisabled(t.Context(), id, true)

	if !errors.Is(err, backend) {
		t.Errorf("SetUserDisabled() error = %v, want it to wrap %v", err, backend)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUserStoreSetUserDisabledReportsCommitFailure(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v, want nil", err)
	}
	defer mock.Close()
	id := uuid.Must(uuid.NewV7())
	backend := errors.New("backend unavailable")
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE core.users").WithArgs(id, false).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit().WillReturnError(backend)
	mock.ExpectRollback()

	err = newUserStore(mock).SetUserDisabled(t.Context(), id, false)

	if !errors.Is(err, backend) {
		t.Errorf("SetUserDisabled() error = %v, want it to wrap %v", err, backend)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
