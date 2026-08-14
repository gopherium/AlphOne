// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gopherium/alphone/internal/version"
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
	return filepath.Join(bindir, "alphone"), append(env, "GOCOVERDIR="+gocoverdir)
}

func TestMainBinaryRequiresDatabaseURL(t *testing.T) {
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
		t.Fatalf("alphone without configuration: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "ALPHONE_DATABASE_URL is required") {
		t.Errorf("stderr = %q, want it to report the missing database URL", stderr.String())
	}
}

func TestMainBinaryPrintsHelp(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	var stdout bytes.Buffer
	cmd := exec.Command(binary, "--help")
	cmd.Dir = t.TempDir()
	cmd.Env = env
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("alphone --help: %v, want it to succeed", err)
	}
	if !strings.Contains(stdout.String(), "createadmin") {
		t.Errorf("stdout = %q, want the subcommands listed", stdout.String())
	}
}

func TestMainBinaryRefusesAnUnknownArgument(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	var stderr bytes.Buffer
	cmd := exec.Command(binary, "not-a-subcommand")
	cmd.Dir = t.TempDir()
	cmd.Env = env
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("alphone with an unknown argument: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Errorf("stderr = %q, want the argument refused", stderr.String())
	}
	if strings.Contains(stderr.String(), "ALPHONE_DATABASE_URL is required") {
		t.Errorf("stderr = %q, want the server never started", stderr.String())
	}
}

func TestMainBinaryCreateAdminReportsFailure(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
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

	binary, env := coverBinary(t)
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

func TestMainBinarySeedReportsFailure(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	var stderr bytes.Buffer
	cmd := exec.Command(binary, "seed")
	cmd.Dir = t.TempDir()
	cmd.Env = env
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("seed without configuration: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "ALPHONE_DATABASE_URL is required") {
		t.Errorf("stderr = %q, want it to report the missing database URL", stderr.String())
	}
}

func TestMainBinarySeedStoresDemoData(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(binary, "seed")
	cmd.Dir = t.TempDir()
	cmd.Env = append(env, "ALPHONE_DATABASE_URL="+testDatabaseURL(t))
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("seed: %v, stderr: %s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "admin@example.com / password1234") {
		t.Errorf("stdout = %q, want it to print the demo credentials", stdout.String())
	}
}

func TestMainBinarySeedFillsTheDemoImportField(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	databaseURL := testDatabaseURL(t)
	var stderr bytes.Buffer
	seedCmd := exec.Command(binary, "seed")
	seedCmd.Dir = t.TempDir()
	seedCmd.Env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	seedCmd.Stderr = &stderr
	if err := seedCmd.Run(); err != nil {
		t.Fatalf("seed: %v, stderr: %s", err, stderr.String())
	}
	addr, secret := servedSeededBinary(t, databaseURL)

	read := postGraph(t, addr, secret,
		`{"query":"{ contacts(first: 50) { edges { node { name birthDate } } } }"}`)

	if read.Data.Contacts == nil {
		t.Fatal("the read answered no contacts, want the seeded demo import")
	}
	var found bool
	for _, edge := range read.Data.Contacts.Edges {
		if edge.Node["name"] != "Grace Hopper" {
			continue
		}
		found = true
		if edge.Node["birthDate"] != "1906-12-09" {
			t.Errorf("birthDate = %#v, want the value the seed path's providers wrote",
				edge.Node["birthDate"])
		}
	}
	if !found {
		t.Error("the seeded demo import listed no Grace Hopper, want the imported contact")
	}
}

func TestMainBinaryServesUntilSignalled(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
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

	waitForServer(t, "http://"+addr)
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

// bearerTransport sends every request with the given bearer secret.
type bearerTransport struct {
	secret string
}

// RoundTrip adds the Authorization header and forwards the request.
func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	cloned := r.Clone(r.Context())
	cloned.Header.Set("Authorization", "Bearer "+b.secret)
	return http.DefaultTransport.RoundTrip(cloned)
}

// tokenSecret pulls the a1_ secret out of the token create output.
func tokenSecret(t *testing.T, stdout string) string {
	t.Helper()
	start := strings.Index(stdout, "a1_")
	if start < 0 {
		t.Fatalf("stdout = %q, want it to carry the secret", stdout)
	}
	secret := stdout[start:]
	if end := strings.IndexAny(secret, " \n"); end >= 0 {
		secret = secret[:end]
	}
	return secret
}

// graphAnswer is the envelope the exec tests read one operation back through.
type graphAnswer struct {
	Data struct {
		DefineField struct {
			ID string `json:"id"`
		} `json:"defineField"`
		CreateContact struct {
			ID string `json:"id"`
		} `json:"createContact"`
		Tenant struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"tenant"`
		ImportUpload struct {
			ID string `json:"id"`
		} `json:"importUpload"`
		ImportCommit struct {
			Imported int `json:"imported"`
			Failed   int `json:"failed"`
		} `json:"importCommit"`
		ImportFields []struct {
			Name string `json:"name"`
		} `json:"importFields"`
		Contacts *struct {
			Edges []struct {
				Node map[string]any `json:"node"`
			} `json:"edges"`
		} `json:"contacts"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// postGraph runs one graph operation against the running binary, refusing any failure.
func postGraph(t *testing.T, addr, secret, body string) graphAnswer {
	t.Helper()
	request, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, "http://"+addr+"/api/graphql", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the graph request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("posting the graph request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading the graph answer: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d, answered %s", response.StatusCode, http.StatusOK, answered)
	}
	var envelope graphAnswer
	if err := json.Unmarshal(answered, &envelope); err != nil {
		t.Fatalf("decoding %s: %v", answered, err)
	}
	if len(envelope.Errors) > 0 {
		t.Fatalf("the graph refused the operation: %s", answered)
	}
	return envelope
}

// uploadCSV posts one CSV file through the graph multipart upload protocol.
func uploadCSV(t *testing.T, addr, secret, content string) graphAnswer {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	const operations = `{"query":"mutation($file: Upload!) { importUpload(file: $file) { id } }",` +
		`"variables":{"file":null}}`
	for field, value := range map[string]string{
		"operations": operations,
		"map":        `{"0":["variables.file"]}`,
	} {
		if err := form.WriteField(field, value); err != nil {
			t.Fatalf("writing the %s part: %v", field, err)
		}
	}
	part, err := form.CreateFormFile("0", "leads.csv")
	if err != nil {
		t.Fatalf("creating the file part: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("writing the file part: %v", err)
	}
	if err := form.Close(); err != nil {
		t.Fatalf("closing the form: %v", err)
	}
	return postForm(t, addr, secret, form.FormDataContentType(), &body)
}

// postForm posts a multipart body to the graph, refusing any failure.
func postForm(t *testing.T, addr, secret, contentType string, body io.Reader) graphAnswer {
	t.Helper()
	request, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, "http://"+addr+"/api/graphql", body)
	if err != nil {
		t.Fatalf("building the upload request: %v", err)
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("posting the upload: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading the upload answer: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d, answered %s", response.StatusCode, http.StatusOK, answered)
	}
	var envelope graphAnswer
	if err := json.Unmarshal(answered, &envelope); err != nil {
		t.Fatalf("decoding %s: %v", answered, err)
	}
	if len(envelope.Errors) > 0 {
		t.Fatalf("the graph refused the upload: %s", answered)
	}
	return envelope
}

// servedBinary starts the real binary on its own database, answering its address and token.
func servedBinary(t *testing.T, databaseURL string) (string, string) {
	t.Helper()
	binary, env := coverBinary(t)
	createUser := exec.Command(binary, "createadmin", "-email", "admin@example.com", "-name", "Admin")
	createUser.Dir = t.TempDir()
	createUser.Env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	createUser.Stdin = strings.NewReader("correct horse battery\n")
	if err := createUser.Run(); err != nil {
		t.Fatalf("createadmin: %v", err)
	}
	return servedSeededBinary(t, databaseURL)
}

// servedSeededBinary starts the real binary on a database already holding the admin.
func servedSeededBinary(t *testing.T, databaseURL string) (string, string) {
	t.Helper()
	binary, env := coverBinary(t)
	var minted bytes.Buffer
	token := exec.Command(binary, "token", "create", "-email", "admin@example.com", "-name", "exec")
	token.Dir = t.TempDir()
	token.Env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	token.Stdout = &minted
	if err := token.Run(); err != nil {
		t.Fatalf("token create: %v", err)
	}
	addr := freeAddr(t)
	serve := exec.Command(binary)
	serve.Dir = t.TempDir()
	serve.Env = append(env,
		"ALPHONE_DATABASE_URL="+databaseURL,
		"ALPHONE_ADDR="+addr,
		"ALPHONE_WHATSAPP_VERIFY_TOKEN=e2e-secret",
		"ALPHONE_WHATSAPP_APP_SECRET=e2e-app-secret",
	)
	if err := serve.Start(); err != nil {
		t.Fatalf("starting alphone: %v", err)
	}
	t.Cleanup(func() { _ = serve.Process.Kill() })
	waitForServer(t, "http://"+addr)
	return addr, tokenSecret(t, minted.String())
}

func TestMainBinaryServesARuntimeDefinedField(t *testing.T) {
	t.Parallel()

	addr, secret := servedBinary(t, testDatabaseURL(t))

	defined := postGraph(t, addr, secret,
		`{"query":"mutation { defineField(name: \"birthDate\", label: \"Birth date\", kind: DATE) { id } }"}`)
	if defined.Data.DefineField.ID == "" {
		t.Fatal("defineField answered no id, want the definition stored")
	}
	created := postGraph(t, addr, secret,
		`{"query":"mutation { createContact(name: \"Maria Perez\") { id } }"}`)
	if created.Data.CreateContact.ID == "" {
		t.Fatal("createContact answered no id, want a contact to read the field back from")
	}

	read := postGraph(t, addr, secret,
		`{"query":"{ contacts(first: 1) { edges { node { name birthDate } } } }"}`)

	if read.Data.Contacts == nil {
		t.Fatal("the read answered no contacts, want the connection served")
	}
	if len(read.Data.Contacts.Edges) == 0 {
		t.Fatal("the read answered no contact, want the seeded contact")
	}
	node := read.Data.Contacts.Edges[0].Node
	if _, selected := node["birthDate"]; !selected {
		t.Errorf("node = %v, want birthDate answered by the running binary's widened schema", node)
	}
}

func TestMainBinaryAnswersTheCallersTenant(t *testing.T) {
	t.Parallel()

	addr, secret := servedBinary(t, testDatabaseURL(t))

	read := postGraph(t, addr, secret, `{"query":"{ tenant { id name } }"}`)

	if read.Data.Tenant.Name != "Default" {
		t.Errorf("tenant = %q, want Default answered by the running binary", read.Data.Tenant.Name)
	}
	if read.Data.Tenant.ID == "" {
		t.Error("the tenant carries no id, want the fixed default identifier")
	}
}

func TestMainBinaryImportsASpreadsheetColumnIntoAField(t *testing.T) {
	t.Parallel()

	addr, secret := servedBinary(t, testDatabaseURL(t))
	postGraph(t, addr, secret,
		`{"query":"mutation { defineField(name: \"birthDate\", label: \"Birth date\", kind: DATE) { id } }"}`)

	listed := postGraph(t, addr, secret, `{"query":"{ importFields { name } }"}`)
	var mappable bool
	for _, field := range listed.Data.ImportFields {
		mappable = mappable || field.Name == "birthDate"
	}
	if !mappable {
		t.Fatalf("importFields = %v, want the defined field mappable in the real binary", listed.Data.ImportFields)
	}

	uploaded := uploadCSV(t, addr, secret,
		"Name,Email,Birth date\nMaria Perez,maria@example.com,1990-04-17\n")
	if uploaded.Data.ImportUpload.ID == "" {
		t.Fatal("importUpload answered no id, want the spreadsheet staged")
	}
	postGraph(t, addr, secret, `{"query":"mutation { importSetMapping(id: \"`+uploaded.Data.ImportUpload.ID+
		`\", assignments: [{column: 0, field: \"name\"}, {column: 1, field: \"email\"},`+
		` {column: 2, field: \"birthDate\"}]) { id } }"}`)
	committed := postGraph(t, addr, secret,
		`{"query":"mutation { importCommit(id: \"`+uploaded.Data.ImportUpload.ID+
			`\") { imported failed } }"}`)

	if committed.Data.ImportCommit.Imported != 1 {
		t.Fatalf("imported = %d, failed = %d, want the row imported",
			committed.Data.ImportCommit.Imported, committed.Data.ImportCommit.Failed)
	}
	read := postGraph(t, addr, secret,
		`{"query":"{ contacts(first: 5) { edges { node { name birthDate } } } }"}`)
	if read.Data.Contacts == nil || len(read.Data.Contacts.Edges) == 0 {
		t.Fatal("the read answered no contacts, want the imported contact")
	}
	node := read.Data.Contacts.Edges[0].Node
	if node["birthDate"] != "1990-04-17" {
		t.Errorf("birthDate = %#v, want the imported cell served by the real binary", node["birthDate"])
	}
}

func TestMainBinaryAdvertisesTheBuildVersionOverMCP(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	databaseURL := testDatabaseURL(t)
	createUser := exec.Command(binary, "createadmin", "-email", "admin@example.com", "-name", "Admin")
	createUser.Dir = t.TempDir()
	createUser.Env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	createUser.Stdin = strings.NewReader("correct horse battery\n")
	if err := createUser.Run(); err != nil {
		t.Fatalf("createadmin: %v", err)
	}
	var stdout bytes.Buffer
	mint := exec.Command(binary, "token", "create", "-email", "admin@example.com", "-name", "agent")
	mint.Dir = t.TempDir()
	mint.Env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	mint.Stdout = &stdout
	if err := mint.Run(); err != nil {
		t.Fatalf("token create: %v", err)
	}
	addr := freeAddr(t)
	serve := exec.Command(binary)
	serve.Dir = t.TempDir()
	serve.Env = append(env,
		"ALPHONE_DATABASE_URL="+databaseURL,
		"ALPHONE_ADDR="+addr,
		"ALPHONE_WHATSAPP_VERIFY_TOKEN=e2e-secret",
		"ALPHONE_WHATSAPP_APP_SECRET=e2e-app-secret",
	)
	if err := serve.Start(); err != nil {
		t.Fatalf("starting alphone: %v", err)
	}
	t.Cleanup(func() { _ = serve.Process.Kill() })
	waitForServer(t, "http://"+addr)

	client := mcp.NewClient(&mcp.Implementation{Name: "exec-test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:   "http://" + addr + "/api/mcp",
		HTTPClient: &http.Client{Transport: bearerTransport{secret: tokenSecret(t, stdout.String())}},
	}, nil)
	if err != nil {
		t.Fatalf("connecting over MCP: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	served := session.InitializeResult().ServerInfo.Version
	if served != version.Version() {
		t.Errorf("advertised version = %q, want %q", served, version.Version())
	}
}

func TestMainBinaryTokenReportsFailure(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	var stderr bytes.Buffer
	cmd := exec.Command(binary, "token", "list", "-email", "admin@example.com")
	cmd.Dir = t.TempDir()
	cmd.Env = env
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("token without configuration: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "ALPHONE_DATABASE_URL is required") {
		t.Errorf("stderr = %q, want it to report the missing database URL", stderr.String())
	}
}

func TestMainBinaryTokenCreatesAToken(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	databaseURL := testDatabaseURL(t)
	createUser := exec.Command(binary, "createadmin", "-email", "admin@example.com", "-name", "Admin")
	createUser.Dir = t.TempDir()
	createUser.Env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	createUser.Stdin = strings.NewReader("correct horse battery\n")
	if err := createUser.Run(); err != nil {
		t.Fatalf("createadmin: %v", err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(binary, "token", "create", "-email", "admin@example.com", "-name", "n8n")
	cmd.Dir = t.TempDir()
	cmd.Env = append(env, "ALPHONE_DATABASE_URL="+databaseURL)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("token create: %v, stderr: %s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "a1_") {
		t.Errorf("stdout = %q, want it to print the secret", stdout.String())
	}
}
