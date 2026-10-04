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
	env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	provision := exec.Command(binary,
		"account:create-admin", "-email", "admin@example.com", "-name", "Admin", "-role", "admin")
	provision.Dir = t.TempDir()
	provision.Env = env
	provision.Stdin = strings.NewReader(typedPassword + "\n")
	if err := provision.Run(); err != nil {
		t.Fatalf("account:create-admin: %v", err)
	}
	holding := storeRoleless(t, databaseURL, "none@example.com")
	var stdout, stderr bytes.Buffer
	granting := exec.Command(binary, "account:grant-role", "-role", "member", "-yes", "-as", "admin@example.com")
	granting.Dir = t.TempDir()
	granting.Env = env
	granting.Stdout = &stdout
	granting.Stderr = &stderr

	if err := granting.Run(); err != nil {
		t.Fatalf("account:grant-role: %v, stderr %s", err, stderr.String())
	}

	users := authkitpg.NewUserStore(testPool(t, databaseURL))
	held, err := users.UserByID(t.Context(), holding.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if held.Role != role.Member.String() {
		t.Errorf("role = %q, want %q written by the running binary", held.Role, role.Member.String())
	}
	if !strings.Contains(stdout.String(), "granted member to 1 account") {
		t.Errorf("output = %q, want it to count the account that took the role", stdout.String())
	}
	if kept := roleOf(t, databaseURL, "admin@example.com"); kept != role.Admin.String() {
		t.Errorf("admin@example.com holds %q, want the acting admin kept", kept)
	}
	records := recordsOf(t, map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	if len(records) != 1 || !strings.Contains(records[0], "admin@example.com  account:grant-role  -role member") {
		t.Errorf("records = %q, want the one grant admin@example.com applied", records)
	}
}
