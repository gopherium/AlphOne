// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestMainBinaryRequiresDatabaseURL(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t, "alphone")
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir()
	cmd.Env = env
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("alphone without configuration: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "ALPHONE_DATABASE_URL is required") {
		t.Errorf("stderr = %q, want it to report the missing database URL", stderr.String())
	}
}

func TestMainBinaryCreateAdminReportsFailure(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t, "alphone")
	var stderr bytes.Buffer
	cmd := exec.Command(binary, "createadmin")
	cmd.Dir = t.TempDir()
	cmd.Env = env
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("createadmin without configuration: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "ALPHONE_DATABASE_URL is required") {
		t.Errorf("stderr = %q, want it to report the missing database URL", stderr.String())
	}
}

func TestMainBinaryCreateAdminCreatesUser(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t, "alphone")
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(binary, "createadmin", "-email", "admin@example.com", "-name", "Admin")
	cmd.Dir = t.TempDir()
	cmd.Env = append(env, "ALPHONE_DATABASE_URL="+testDatabaseURL(t))
	cmd.Stdin = strings.NewReader("correct horse battery\n")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("createadmin: %v, stderr: %s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "created user admin@example.com") {
		t.Errorf("stdout = %q, want it to confirm the created user", stdout.String())
	}
}

func TestMainBinaryServesUntilSignalled(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t, "alphone")
	addr := freeAddr(t)
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir()
	cmd.Env = append(env,
		"ALPHONE_DATABASE_URL="+testDatabaseURL(t),
		"ALPHONE_ADDR="+addr,
		"ALPHONE_WHATSAPP_VERIFY_TOKEN=e2e-secret",
		"ALPHONE_WHATSAPP_APP_SECRET=e2e-app-secret",
	)
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting alphone: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	waitForServer(t, "http://"+addr+"/api/contacts/"+uuid.Must(uuid.NewV7()).String())
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signalling alphone: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("alphone exited with %v, stderr: %s", err, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("alphone did not shut down after SIGTERM")
	}
	for _, message := range []string{"listening", "shutting down"} {
		if !strings.Contains(stderr.String(), message) {
			t.Errorf("stderr = %q, want it to log %q", stderr.String(), message)
		}
	}
}
