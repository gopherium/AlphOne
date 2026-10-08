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

func TestMainBinaryFailsWithoutTheConfig(t *testing.T) {
	t.Parallel()

	binary, env := testkit.CoverBinary(t, "ALPHONE_", "schemagen")
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir()
	cmd.Env = env
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("schemagen without a config: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "load config") {
		t.Errorf("stderr = %q, want it to report the missing config", stderr.String())
	}
}
