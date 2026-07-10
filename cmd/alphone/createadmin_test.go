// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/postgres"
)

const unreachableDatabaseURL = "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1"

type failingStdin struct{}

func (failingStdin) Read([]byte) (int, error) {
	return 0, errors.New("stdin exploded")
}

func TestCreateAdminProvisionsAUser(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	var stdout strings.Builder

	err := createAdmin(
		t.Context(),
		getenv,
		[]string{"-email", " Admin@Example.com ", "-name", "Admin"},
		strings.NewReader("correct horse battery\n"),
		&stdout,
	)

	if err != nil {
		t.Fatalf("createAdmin() error = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "admin@example.com") {
		t.Errorf("output = %q, want it to name the created user", stdout.String())
	}

	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	created, err := postgres.NewUserStore(pool).UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the created user", err)
	}
	if !gouncer.VerifyPassword(created.PasswordHash, "correct horse battery") {
		t.Error("stored password hash does not verify against the entered password")
	}
}

func TestCreateAdminRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	args := []string{"-email", "admin@example.com", "-name", "Admin"}

	if err := createAdmin(
		t.Context(), getenv, args, strings.NewReader("correct horse battery\n"), io.Discard,
	); err != nil {
		t.Fatalf("first createAdmin() error = %v, want nil", err)
	}

	err := createAdmin(
		t.Context(), getenv, args, strings.NewReader("correct horse battery\n"), io.Discard,
	)

	if !errors.Is(err, gouncer.ErrEmailTaken) {
		t.Errorf("createAdmin() error = %v, want %v", err, gouncer.ErrEmailTaken)
	}
}

func TestCreateAdminValidatesItsInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		env   map[string]string
		args  []string
		stdin io.Reader
	}{
		"missing database url": {
			env:   nil,
			args:  []string{"-email", "admin@example.com", "-name", "Admin"},
			stdin: strings.NewReader("correct horse battery\n"),
		},
		"unknown flag": {
			env:   map[string]string{"ALPHONE_DATABASE_URL": "postgres://localhost/db"},
			args:  []string{"-bogus"},
			stdin: strings.NewReader("correct horse battery\n"),
		},
		"invalid email": {
			env:   map[string]string{"ALPHONE_DATABASE_URL": "postgres://localhost/db"},
			args:  []string{"-email", "not-an-email", "-name", "Admin"},
			stdin: strings.NewReader("correct horse battery\n"),
		},
		"short password": {
			env:   map[string]string{"ALPHONE_DATABASE_URL": "postgres://localhost/db"},
			args:  []string{"-email", "admin@example.com", "-name", "Admin"},
			stdin: strings.NewReader("short\n"),
		},
		"no password input": {
			env:   map[string]string{"ALPHONE_DATABASE_URL": "postgres://localhost/db"},
			args:  []string{"-email", "admin@example.com", "-name", "Admin"},
			stdin: strings.NewReader(""),
		},
		"failing stdin": {
			env:   map[string]string{"ALPHONE_DATABASE_URL": "postgres://localhost/db"},
			args:  []string{"-email", "admin@example.com", "-name", "Admin"},
			stdin: failingStdin{},
		},
		"malformed database url": {
			env:   map[string]string{"ALPHONE_DATABASE_URL": "not a url \x00"},
			args:  []string{"-email", "admin@example.com", "-name", "Admin"},
			stdin: strings.NewReader("correct horse battery\n"),
		},
		"unreachable database": {
			env:   map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL},
			args:  []string{"-email", "admin@example.com", "-name", "Admin"},
			stdin: strings.NewReader("correct horse battery\n"),
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			err := createAdmin(t.Context(), testGetenv(tc.env), tc.args, tc.stdin, io.Discard)

			if err == nil {
				t.Fatal("createAdmin() error = nil, want a failure")
			}
		})
	}
}
