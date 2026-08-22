// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/gopherium/gouncer"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/role"
)

// storeRoleless stores one account holding no role and returns it.
func storeRoleless(t *testing.T, databaseURL, email string) gouncer.User {
	t.Helper()
	held, err := gouncer.NewUser(email, "Maria Perez", "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	if err := authkitpg.NewUserStore(testPool(t, databaseURL)).CreateUser(t.Context(), held); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return held
}

func TestGrantRoleReachesEveryAccountHoldingNone(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	holding := storeRoleless(t, databaseURL, "none@example.com")
	var stdout strings.Builder

	if err := grantRole(t.Context(), getenv, []string{"-role", "member"}, &stdout); err != nil {
		t.Fatalf("grantRole() error = %v, want nil", err)
	}

	users := authkitpg.NewUserStore(testPool(t, databaseURL))
	held, err := users.UserByID(t.Context(), holding.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if held.Role != role.Member.String() {
		t.Errorf("role = %q, want %q", held.Role, role.Member.String())
	}
	if !strings.Contains(stdout.String(), "1") {
		t.Errorf("output = %q, want it to count the account that took the role", stdout.String())
	}
}

func TestGrantRoleLeavesAnAccountThatHoldsOne(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	standing := storeRoleless(t, databaseURL, "standing@example.com")
	if err := grantRole(t.Context(), getenv, []string{"-role", "admin"}, &strings.Builder{}); err != nil {
		t.Fatalf("first grantRole() error = %v, want nil", err)
	}

	if err := grantRole(t.Context(), getenv, []string{"-role", "member"}, &strings.Builder{}); err != nil {
		t.Fatalf("second grantRole() error = %v, want nil", err)
	}

	users := authkitpg.NewUserStore(testPool(t, databaseURL))
	held, err := users.UserByID(t.Context(), standing.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if held.Role != role.Admin.String() {
		t.Errorf("role = %q, want %q, a second run leaves an account that holds one", held.Role, role.Admin.String())
	}
}

func TestGrantRoleRefusesARoleTheRegistryDoesNotKnow(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": testDatabaseURL(t)})

	err := grantRole(t.Context(), getenv, []string{"-role", "superadmin"}, &strings.Builder{})

	if !errors.Is(err, role.ErrUnknownTier) {
		t.Errorf("grantRole() error = %v, want a role no plugin declared refused", err)
	}
}

func TestGrantRoleNamesTheMissingDatabaseBeforeTheRole(t *testing.T) {
	t.Parallel()

	err := grantRole(t.Context(), testGetenv(nil), nil, &strings.Builder{})

	if err == nil || errors.Is(err, role.ErrUnknownTier) {
		t.Errorf("grantRole() error = %v, want the database url named first", err)
	}
}

func TestGrantRoleRefusesAFlagItDoesNotKnow(t *testing.T) {
	t.Parallel()

	err := grantRole(t.Context(), testGetenv(nil), []string{"-bogus"}, &strings.Builder{})

	if err == nil {
		t.Error("grantRole() error = nil, want the unknown flag refused")
	}
}

func TestGrantRoleReportsADatabaseItCannotReach(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL})

	err := grantRole(t.Context(), getenv, []string{"-role", "member"}, &strings.Builder{})

	if err == nil {
		t.Error("grantRole() error = nil, want the unreachable database reported")
	}
}

func TestGrantRolePrintsItsFlags(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder

	err := grantRole(t.Context(), testGetenv(nil), []string{"-h"}, &stdout)

	if err != nil {
		t.Fatalf("grantRole() error = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "-role") {
		t.Errorf("output = %q, want the flags listed", stdout.String())
	}
}
