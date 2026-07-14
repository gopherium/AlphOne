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

func coverBinary(t *testing.T, name string) (string, []string) {
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
	return filepath.Join(bindir, name), append(env, "GOCOVERDIR="+gocoverdir)
}

func TestMainBinaryGeneratesWiring(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t, "pluginwire")
	root := t.TempDir()
	writePlugin(t, root, "demo", `{
		"id": "demo",
		"name": "Demo",
		"backend": "github.com/gopherium/alphone/plugins/demo"
	}`)
	for _, dir := range []string{"cmd/alphone", "frontend/src/plugins"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
	}
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("pluginwire on a valid tree: %v, stderr: %s", err, stderr.String())
	}

	for _, generated := range []string{"cmd/alphone/plugins_gen.go", "frontend/src/plugins/index.ts"} {
		if _, err := os.Stat(filepath.Join(root, generated)); err != nil {
			t.Errorf("expected generated file %s: %v", generated, err)
		}
	}
}

func TestMainBinaryFailsWithoutPluginsDirectory(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t, "pluginwire")
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir()
	cmd.Env = env
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("pluginwire without a plugins directory: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "reading plugins directory") {
		t.Errorf("stderr = %q, want it to report the missing plugins directory", stderr.String())
	}
}
