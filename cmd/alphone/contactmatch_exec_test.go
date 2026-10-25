// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/gopherium/framework/gonsole/testkit"
)

func TestMainBinaryRefusesAContactMatchBoundOfZero(t *testing.T) {
	t.Parallel()

	binary, env := testkit.CoverBinary(t, "ALPHONE_", "alphone")
	var stderr bytes.Buffer
	cmd := exec.Command(binary, "serve")
	cmd.Dir = t.TempDir()
	cmd.Env = append(env, "ALPHONE_DATABASE_URL="+unreachableDatabaseURL, "ALPHONE_CONTACT_MATCH_MAX=0")
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("alphone serve with a match bound of zero: %v, want exit code 1", err)
	}
	if want := `ALPHONE_CONTACT_MATCH_MAX: must stand above zero, got "0"`; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to name %s", stderr.String(), want)
	}
}
