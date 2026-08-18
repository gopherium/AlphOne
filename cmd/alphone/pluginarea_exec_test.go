// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// mintScopedSecret returns the secret of a token the real binary mints for the given scope.
func mintScopedSecret(t *testing.T, binary string, env []string, databaseURL, scope string) string {
	t.Helper()
	var stdout bytes.Buffer
	mint := exec.Command(binary, "token", "create",
		"-email", "admin@example.com", "-name", "narrow", "-scope", scope)
	mint.Dir = t.TempDir()
	mint.Env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	mint.Stdout = &stdout
	if err := mint.Run(); err != nil {
		t.Fatalf("token create: %v", err)
	}
	return tokenSecret(t, stdout.String())
}

// getWithSecret returns the status the running binary answers one bearer GET with.
func getWithSecret(t *testing.T, url, secret string) int {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("calling the plugin route: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode
}

func TestMainBinaryHoldsAPluginRouteToItsDeclaredArea(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	binary, env := coverBinary(t)
	createUser := exec.Command(binary, "createadmin", "-email", "admin@example.com", "-name", "Admin")
	createUser.Dir = t.TempDir()
	createUser.Env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	createUser.Stdin = strings.NewReader("correct horse battery\n")
	if err := createUser.Run(); err != nil {
		t.Fatalf("createadmin: %v", err)
	}
	narrow := mintScopedSecret(t, binary, env, databaseURL, "tasks:read")
	wide := mintScopedSecret(t, binary, env, databaseURL, "whatsapp:read")
	addr, _ := servedSeededBinary(t, databaseURL)
	media := "http://" + addr + "/api/plugins/whatsapp/conversations/" +
		uuid.Must(uuid.NewV7()).String() + "/messages/" + uuid.Must(uuid.NewV7()).String() + "/media"

	if got := getWithSecret(t, media, narrow); got != http.StatusForbidden {
		t.Errorf("status = %d, want %d, a tasks token does not download media", got, http.StatusForbidden)
	}
	if got := getWithSecret(t, media, wide); got == http.StatusForbidden {
		t.Errorf("status = %d, want the area token past the guard", got)
	}
}
