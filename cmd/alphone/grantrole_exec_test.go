// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/role"
)

func TestMainBinaryGrantsARoleToEveryAccountHoldingNone(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	databaseURL := testDatabaseURL(t)
	holding := storeRoleless(t, databaseURL, "none@example.com")
	var stdout bytes.Buffer
	granting := exec.Command(binary, "grantrole", "-role", "member")
	granting.Dir = t.TempDir()
	granting.Env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	granting.Stdout = &stdout

	if err := granting.Run(); err != nil {
		t.Fatalf("grantrole: %v, answered %s", err, stdout.String())
	}

	users := authkitpg.NewUserStore(testPool(t, databaseURL))
	held, err := users.UserByID(t.Context(), holding.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if held.Role != role.Member.String() {
		t.Errorf("role = %q, want %q written by the running binary", held.Role, role.Member.String())
	}
	if !strings.Contains(stdout.String(), "1") {
		t.Errorf("output = %q, want it to count the account that took the role", stdout.String())
	}
}
