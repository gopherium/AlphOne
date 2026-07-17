// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/server"
)

const streamLogin = `{"email":"ada@example.com","password":"correct horse battery"}`

func blockingPlugin(entered chan<- struct{}) http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-r.Context().Done()
	})
}

func newStreamServer(users *testkit.Store, entered chan struct{}, lifetime time.Duration, perUser int) http.Handler {
	return server.NewServer(server.Config{
		Contacts:          newFakeContactStore(),
		Users:             users,
		Plugins:           map[string]http.Handler{"stub": blockingPlugin(entered)},
		PluginPublicPaths: map[string][]string{"stub": {"/webhook"}},
		MaxStreamLifetime: lifetime,
		MaxStreamsPerUser: perUser,
	})
}

func openPluginRequest(
	handler http.Handler,
	path string,
	cookie *http.Cookie,
) (context.CancelFunc, chan *httptest.ResponseRecorder) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
		if cookie != nil {
			request.AddCookie(cookie)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		done <- recorder
	}()
	return cancel, done
}

func TestHostBoundsPluginStreamLifetime(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	entered := make(chan struct{}, 8)
	handler := newStreamServer(users, entered, 30*time.Millisecond, 5)
	cookie := sessionCookie(t, doLogin(t, handler, streamLogin))

	cancel, done := openPluginRequest(handler, "/api/plugins/stub/events", cookie)
	defer cancel()

	<-entered
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the host never closed a plugin stream that outlived MaxStreamLifetime")
	}
}

func TestHostCapsConcurrentPluginStreamsPerUser(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	users.AddUser(t, "grace@example.com", "Grace Hopper", testPassword)
	entered := make(chan struct{}, 8)
	handler := newStreamServer(users, entered, 2*time.Second, 2)
	ada := sessionCookie(t, doLogin(t, handler, streamLogin))
	grace := sessionCookie(t, doLogin(t, handler,
		`{"email":"grace@example.com","password":"correct horse battery"}`))

	cancelFirst, doneFirst := openPluginRequest(handler, "/api/plugins/stub/events", ada)
	cancelSecond, doneSecond := openPluginRequest(handler, "/api/plugins/stub/events", ada)
	<-entered
	<-entered

	cancelOver, doneOver := openPluginRequest(handler, "/api/plugins/stub/events", ada)
	defer cancelOver()
	var over *httptest.ResponseRecorder
	select {
	case over = <-doneOver:
	case <-time.After(2 * time.Second):
		t.Fatal("the over-cap stream was admitted instead of rejected")
	}
	if over.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d for a stream over the per-user cap", over.Code, http.StatusTooManyRequests)
	}
	if got := over.Header().Get("Retry-After"); got != "2" {
		t.Errorf("Retry-After = %q, want %q", got, "2")
	}
	if body := decodeBody[errorBody](t, over); body.Error == "" {
		t.Error("over-cap rejection carries no JSON error message")
	}

	cancelGrace, doneGrace := openPluginRequest(handler, "/api/plugins/stub/events", grace)
	<-entered
	cancelGrace()
	<-doneGrace

	cancelFirst()
	<-doneFirst
	cancelFreed, doneFreed := openPluginRequest(handler, "/api/plugins/stub/events", ada)
	<-entered
	cancelFreed()
	<-doneFreed

	cancelSecond()
	<-doneSecond
}

func TestHostLeavesPublicPluginPathsUnbounded(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	entered := make(chan struct{}, 8)
	handler := newStreamServer(users, entered, 20*time.Millisecond, 1)

	cancelFirst, doneFirst := openPluginRequest(handler, "/api/plugins/stub/webhook", nil)
	cancelSecond, doneSecond := openPluginRequest(handler, "/api/plugins/stub/webhook", nil)
	<-entered
	<-entered

	select {
	case <-doneFirst:
		t.Fatal("a public plugin path was closed by the host stream bound")
	case <-doneSecond:
		t.Fatal("a public plugin path was capped by the host stream limiter")
	case <-time.After(100 * time.Millisecond):
	}

	cancelFirst()
	cancelSecond()
	<-doneFirst
	<-doneSecond
}
