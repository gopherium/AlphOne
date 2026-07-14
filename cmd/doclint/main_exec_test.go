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
	return filepath.Join(bindir, "doclint"), append(env, "GOCOVERDIR="+gocoverdir)
}

func writeFixture(t *testing.T, dir, name, source string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func TestMainBinaryPassesOnDocumentedTree(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	dir := t.TempDir()
	writeFixture(t, dir, "documented.go", "package fixture\n\n// Documented does nothing.\nfunc Documented() {}\n")
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("doclint on a documented tree: %v, stderr: %s", err, stderr.String())
	}
}

func TestMainBinaryFailsOnUndocumentedFunction(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	dir := t.TempDir()
	writeFixture(t, dir, "undocumented.go", "package fixture\n\nfunc Undocumented() {}\n")
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("doclint on an undocumented tree: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "missing a doc comment") {
		t.Errorf("stderr = %q, want it to report the missing doc comment", stderr.String())
	}
}
