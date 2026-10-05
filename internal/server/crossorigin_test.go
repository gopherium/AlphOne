// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// crmOrigin is the address the guarded test server answers at.
const crmOrigin = "https://crm.example.com"

// otherSiteOrigin is a page on another site.
const otherSiteOrigin = "https://pages.example.net"

// siblingSiteOrigin is a page on a sibling subdomain of the server's site.
const siblingSiteOrigin = "https://shop.example.com"

// guardedProxyRange is the reverse proxy range the guarded server trusts for client addresses.
const guardedProxyRange = "10.42.0.0/16"

// guardedProxyPeer is the address of the trusted proxy relaying to the guarded server.
const guardedProxyPeer = "10.42.0.7:51000"

// whatsappAppSecret signs the webhook bodies the guarded server's WhatsApp plugin accepts.
const whatsappAppSecret = "app-secret"

// createContactOperations creates the contact the cross-origin tests look for.
const createContactOperations = `{"query":"mutation { createContact(name: \"Maria Perez\") { id name } }"}`

// guardedServer is a test server whose cross-origin guard the tests drive, with what they read back.
type guardedServer struct {
	handler  http.Handler
	contacts *fakeContactStore
	logged   *bytes.Buffer
	secret   string
	cookie   *http.Cookie
	reached  int
}

// newGuardedServer returns a server with a counting plugin, the WhatsApp plugin, a token and a signed in cookie.
func newGuardedServer(t *testing.T) *guardedServer {
	t.Helper()
	users := newFakeUserStore()
	ada := addAda(t, users)
	tokens := newFakeTokenStore()
	minted, err := apitoken.Mint(ada.ID, "n8n production", apitoken.Full(), apitoken.Never)
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	tokens.tokens[minted.Token.Hash] = minted.Token
	messages := signedWhatsApp(t)
	guarded := &guardedServer{contacts: newFakeContactStore(), logged: &bytes.Buffer{}, secret: minted.Secret}
	guarded.handler = newGraphServer(t, graphConfig{
		Contacts: guarded.contacts,
		Tasks:    newFakeTaskStore(),
		Users:    users,
		Tokens:   tokens,
		Version:  "9.9.9",
		Plugins: map[string]http.Handler{
			"echo":     guarded.countingPlugin(),
			"whatsapp": messages.Routes(),
		},
		PluginPublicPaths: map[string][]string{"echo": {"/hook"}, "whatsapp": messages.PublicPaths()},
		TrustedProxies:    []string{guardedProxyRange},
		Logger:            slog.New(slog.NewJSONHandler(guarded.logged, nil)),
		Web:               fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>AlphOne</title>")}},
	})
	guarded.cookie = loginCookie(t, guarded.handler)
	return guarded
}

// countingPlugin returns a plugin handler counting every request it serves.
func (g *guardedServer) countingPlugin() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		g.reached++
		_, _ = w.Write([]byte("plugin says hi"))
	})
}

// serve answers one request against the guarded server.
func (g *guardedServer) serve(request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	g.handler.ServeHTTP(recorder, request)
	return recorder
}

// signedWhatsApp registers the WhatsApp plugin holding the test app secret over a database it never reaches.
func signedWhatsApp(t *testing.T) *whatsapp.Plugin {
	t.Helper()
	held := map[string]string{"ALPHONE_WHATSAPP_APP_SECRET": whatsappAppSecret}
	plugin, err := whatsapp.Register(sdk.Deps{
		DatabaseURL: "postgres://graph:graph@localhost:1/graph",
		Env:         sdk.Env{Prefix: "ALPHONE_", Getenv: func(name string) string { return held[name] }},
	})
	if err != nil {
		t.Fatalf("whatsapp.Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = plugin.Stop(context.Background()) })
	return plugin
}

// formPart is one field or file of a multipart graph post.
type formPart struct {
	name     string
	filename string
	value    string
}

// graphForm returns a multipart post to target carrying parts in order, the shape a plain HTML form sends.
func graphForm(t *testing.T, target string, parts ...formPart) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, part := range parts {
		var field io.Writer
		var err error
		if part.filename == "" {
			field, err = form.CreateFormField(part.name)
		} else {
			field, err = form.CreateFormFile(part.name, part.filename)
		}
		if err != nil {
			t.Fatalf("creating the %s part: %v", part.name, err)
		}
		if _, err := io.WriteString(field, part.value); err != nil {
			t.Fatalf("writing the %s part: %v", part.name, err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatalf("closing the form: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	return request
}

// operationsForm returns a multipart graph post carrying operations then an empty map.
func operationsForm(t *testing.T, operations string) *http.Request {
	t.Helper()
	return graphForm(t, crmOrigin+"/api/graphql",
		formPart{name: "operations", value: operations}, formPart{name: "map", value: "{}"})
}

// uploadOperations stages the attached file as an import.
const uploadOperations = `{"query":"mutation ($file: Upload!) { importUpload(file: $file) { id } }",` +
	`"variables":{"file":null}}`

// uploadForm returns the importer's multipart upload carrying a file the importer cannot read.
func uploadForm(t *testing.T) *http.Request {
	t.Helper()
	return graphForm(t, crmOrigin+"/api/graphql",
		formPart{name: "operations", value: uploadOperations},
		formPart{name: "map", value: `{"0":["variables.file"]}`},
		formPart{name: "0", filename: "contacts.csv", value: "\x00\x01\x02"},
	)
}

// fromBrowser returns request as a browser sends it from a page at origin, naming fetchSite when it is set.
func fromBrowser(request *http.Request, origin, fetchSite string, cookie *http.Cookie) *http.Request {
	request.Header.Set("Origin", origin)
	if fetchSite != "" {
		request.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

// refusalBody is the JSON answer a refused cross-origin write carries.
type refusalBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// assertRefusedCrossOrigin fails unless recorder holds the cross-origin refusal.
func assertRefusedCrossOrigin(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if body := decodeBody[refusalBody](t, recorder); body.Code != "request_cross_origin" {
		t.Errorf("code = %q, want request_cross_origin", body.Code)
	}
}

// assertContactsStored fails unless the server stored count contacts.
func (g *guardedServer) assertContactsStored(t *testing.T, count int) {
	t.Helper()
	if stored := len(g.contacts.contacts); stored != count {
		t.Errorf("stored %d contacts, want %d", stored, count)
	}
}

func TestACrossSiteMultipartGraphWriteIsRefusedBeforeItRuns(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := guarded.serve(fromBrowser(
		operationsForm(t, createContactOperations), otherSiteOrigin, "cross-site", guarded.cookie))

	assertRefusedCrossOrigin(t, recorder)
	guarded.assertContactsStored(t, 0)
}

func TestASameSiteMultipartGraphWriteIsRefused(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := guarded.serve(fromBrowser(
		operationsForm(t, createContactOperations), siblingSiteOrigin, "same-site", guarded.cookie))

	assertRefusedCrossOrigin(t, recorder)
	guarded.assertContactsStored(t, 0)
}

func TestAnOlderBrowserGraphWriteFromAnotherSiteIsRefused(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := guarded.serve(fromBrowser(
		operationsForm(t, createContactOperations), otherSiteOrigin, "", guarded.cookie))

	assertRefusedCrossOrigin(t, recorder)
	guarded.assertContactsStored(t, 0)
}

func TestACrossSiteMultipartLoginIsRefused(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)
	login := operationsForm(t, `{"query":"mutation { login(email: \"ada@example.com\", password: \"`+
		testPassword+`\") { me { id } } }"}`)

	recorder := guarded.serve(fromBrowser(login, otherSiteOrigin, "cross-site", nil))

	assertRefusedCrossOrigin(t, recorder)
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == server.SessionCookieName {
			t.Errorf("the refused login set the session cookie %q", cookie.Value)
		}
	}
}

func TestACrossSiteSignOutLeavesTheSessionAlive(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := guarded.serve(fromBrowser(
		operationsForm(t, `{"query":"mutation { logout }"}`), otherSiteOrigin, "cross-site", guarded.cookie))

	assertRefusedCrossOrigin(t, recorder)
	if me := postGraphQL(t, guarded.handler, `{"query":"{ me { email } }"}`, guarded.cookie); !strings.Contains(
		me.Body.String(), `"email":"ada@example.com"`) {
		t.Errorf("me = %s, want the session still signed in", me.Body.String())
	}
}

func TestASameOriginMultipartGraphWriteRuns(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := guarded.serve(fromBrowser(
		operationsForm(t, createContactOperations), crmOrigin, "same-origin", guarded.cookie))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	guarded.assertContactsStored(t, 1)
}

func TestAnOlderBrowserGraphWriteFromTheSameOriginRuns(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := guarded.serve(fromBrowser(
		operationsForm(t, createContactOperations), crmOrigin, "", guarded.cookie))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	guarded.assertContactsStored(t, 1)
}

func TestACrossOriginReadPasses(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	for _, path := range []string{"/api/plugins/echo/anything", "/contacts"} {
		recorder := guarded.serve(fromBrowser(
			httptest.NewRequest(http.MethodGet, crmOrigin+path, nil), otherSiteOrigin, "cross-site", guarded.cookie))

		if recorder.Code != http.StatusOK {
			t.Errorf("GET %s from another site = %d, want %d", path, recorder.Code, http.StatusOK)
		}
	}
}

func TestATokenWriteWithoutBrowserHeadersPasses(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := postGraphWithBearer(guarded.handler, createContactOperations, guarded.secret)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	guarded.assertContactsStored(t, 1)
}

// whatsappDelivery is a webhook body carrying no message.
const whatsappDelivery = `{"object":"whatsapp_business_account","entry":[]}`

// signedWebhook returns the signed post Meta sends to the WhatsApp webhook.
func signedWebhook() *http.Request {
	mac := hmac.New(sha256.New, []byte(whatsappAppSecret))
	mac.Write([]byte(whatsappDelivery))
	request := httptest.NewRequest(http.MethodPost, crmOrigin+"/api/plugins/whatsapp/webhook",
		strings.NewReader(whatsappDelivery))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return request
}

func TestTheSignedWhatsAppWebhookPasses(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := guarded.serve(signedWebhook())

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

// mcpHandshake is the first message an agent posts to the MCP endpoint.
const mcpHandshake = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18",` +
	`"capabilities":{},"clientInfo":{"name":"n8n","version":"1.0.0"}}}`

// mcpPost returns an agent's MCP handshake carrying the bearer secret.
func mcpPost(secret string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, crmOrigin+"/api/mcp", strings.NewReader(mcpHandshake))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+secret)
	return request
}

func TestAnMCPPostWithATokenPasses(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := guarded.serve(mcpPost(guarded.secret))

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestTheImporterUploadFromTheSameOriginReachesTheImporter(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := guarded.serve(fromBrowser(uploadForm(t), crmOrigin, "same-origin", guarded.cookie))

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"reason":"file_unreadable"`) {
		t.Errorf("answer = %d %s, want the importer to read the upload and refuse its contents",
			recorder.Code, recorder.Body.String())
	}
}

func TestTheImporterUploadFromAnotherSiteIsRefused(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	recorder := guarded.serve(fromBrowser(uploadForm(t), otherSiteOrigin, "cross-site", guarded.cookie))

	assertRefusedCrossOrigin(t, recorder)
}

func TestEveryUnsafeRouteRefusesACrossSiteWrite(t *testing.T) {
	t.Parallel()

	routes := map[string]func(*guardedServer) *http.Request{
		"graph json post": func(*guardedServer) *http.Request {
			request := httptest.NewRequest(http.MethodPost, crmOrigin+"/api/graphql",
				strings.NewReader(createContactOperations))
			request.Header.Set("Content-Type", "application/json")
			return request
		},
		"graph put": func(*guardedServer) *http.Request {
			return httptest.NewRequest(http.MethodPut, crmOrigin+"/api/graphql", strings.NewReader("{}"))
		},
		"mcp post":         func(g *guardedServer) *http.Request { return mcpPost(g.secret) },
		"plugin post":      routeRequest(http.MethodPost, "/api/plugins/echo/anything"),
		"plugin put":       routeRequest(http.MethodPut, "/api/plugins/echo/anything"),
		"plugin patch":     routeRequest(http.MethodPatch, "/api/plugins/echo/anything"),
		"plugin delete":    routeRequest(http.MethodDelete, "/api/plugins/echo/anything"),
		"public path post": routeRequest(http.MethodPost, "/api/plugins/echo/hook"),
		"whatsapp webhook": func(*guardedServer) *http.Request { return signedWebhook() },
		"app page post":    routeRequest(http.MethodPost, "/contacts"),
	}
	guarded := newGuardedServer(t)

	for name, build := range routes {
		t.Run(name, func(t *testing.T) {
			recorder := guarded.serve(fromBrowser(build(guarded), otherSiteOrigin, "cross-site", guarded.cookie))

			assertRefusedCrossOrigin(t, recorder)
		})
	}
	if guarded.reached != 0 {
		t.Errorf("the plugin served %d refused writes, want none", guarded.reached)
	}
	guarded.assertContactsStored(t, 0)
}

// routeRequest returns a builder of one request to path with method.
func routeRequest(method, path string) func(*guardedServer) *http.Request {
	return func(*guardedServer) *http.Request {
		return httptest.NewRequest(method, crmOrigin+path, strings.NewReader("{}"))
	}
}

// relayedWrite returns an older browser's write from origin the trusted proxy relays to host, naming forwardedHost.
func relayedWrite(t *testing.T, host, origin, forwardedHost string, cookie *http.Cookie) *http.Request {
	t.Helper()
	request := graphForm(t, "http://"+host+"/api/graphql",
		formPart{name: "operations", value: createContactOperations}, formPart{name: "map", value: "{}"})
	request.RemoteAddr = guardedProxyPeer
	request.Header.Set("X-Forwarded-Host", forwardedHost)
	request.Header.Set("X-Forwarded-Proto", "https")
	return fromBrowser(request, origin, "", cookie)
}

func TestAnOlderBrowserWriteIsJudgedOnTheHostHeaderWhateverATrustedProxyForwards(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	refused := map[string]*http.Request{
		"a proxy renaming the host": relayedWrite(t, "alphone:8080", crmOrigin, "crm.example.com", guarded.cookie),
		"a forwarded host naming the other site": relayedWrite(
			t, "crm.example.com", otherSiteOrigin, "pages.example.net", guarded.cookie),
	}
	for name, request := range refused {
		t.Run(name, func(t *testing.T) {
			assertRefusedCrossOrigin(t, guarded.serve(request))
		})
	}
	kept := guarded.serve(relayedWrite(t, "crm.example.com", crmOrigin, "pages.example.net", guarded.cookie))
	if kept.Code != http.StatusOK {
		t.Errorf("a proxy keeping the host = %d, want %d: %s", kept.Code, http.StatusOK, kept.Body.String())
	}
	guarded.assertContactsStored(t, 1)
}

// refusalLine is the log line a refused write leaves.
type refusalLine struct {
	Message   string `json:"msg"`
	Reason    string `json:"reason"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Host      string `json:"host"`
	Origin    string `json:"origin"`
	FetchSite string `json:"fetch_site"`
}

// refusedGraphPost returns the log line a refused graph post leaves with reason, host, origin and fetchSite.
func refusedGraphPost(reason, host, origin, fetchSite string) refusalLine {
	return refusalLine{
		Message: "write refused", Reason: reason, Method: http.MethodPost, Path: "/api/graphql",
		Host: host, Origin: origin, FetchSite: fetchSite,
	}
}

func TestARefusedWriteIsLoggedWithItsReason(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)
	cases := []struct {
		name    string
		request *http.Request
		want    refusalLine
	}{
		{
			name: "a browser naming another site",
			request: fromBrowser(
				operationsForm(t, createContactOperations), otherSiteOrigin, "cross-site", guarded.cookie),
			want: refusedGraphPost("fetch-site", "crm.example.com", otherSiteOrigin, "cross-site"),
		},
		{
			name:    "an older browser on another host",
			request: fromBrowser(operationsForm(t, createContactOperations), otherSiteOrigin, "", guarded.cookie),
			want:    refusedGraphPost("origin", "crm.example.com", otherSiteOrigin, ""),
		},
		{
			name:    "an older browser behind a proxy renaming the host",
			request: relayedWrite(t, "alphone:8080", crmOrigin, "crm.example.com", guarded.cookie),
			want:    refusedGraphPost("origin", "alphone:8080", crmOrigin, ""),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guarded.logged.Reset()

			guarded.serve(tc.request)

			var line refusalLine
			if err := json.Unmarshal(guarded.logged.Bytes(), &line); err != nil {
				t.Fatalf("decoding the log line %q: %v", guarded.logged.String(), err)
			}
			if line != tc.want {
				t.Errorf("log line = %+v, want %+v", line, tc.want)
			}
		})
	}
}
