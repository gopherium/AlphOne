// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// coverBinary returns the path of the schemagen cover binary and the environment to run it with.
func coverBinary(t *testing.T) (string, []string) {
	t.Helper()
	bindir := os.Getenv("ALPHONE_COVER_BINDIR")
	gocoverdir := os.Getenv("ALPHONE_COVER_GOCOVERDIR")
	if bindir == "" || gocoverdir == "" {
		t.Skip("skipping binary test: run via make cover")
	}
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "ALPHONE_") && !strings.HasPrefix(entry, "GOCOVERDIR=") {
			env = append(env, entry)
		}
	}
	return filepath.Join(bindir, "schemagen"), append(env, "GOCOVERDIR="+gocoverdir)
}

func TestMainBinaryFailsWithoutTheConfig(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
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
