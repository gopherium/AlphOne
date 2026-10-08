// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gopherium/framework/gonsole"
)

// servedOverTCP starts the graph server on a loopback port under a read timeout outlasting the test.
func servedOverTCP(t *testing.T) string {
	t.Helper()
	users := newFakeUserStore()
	addAda(t, users)
	handler := newGraphServer(t, graphConfig{Contacts: newFakeContactStore(), Users: users, Version: "9.9.9"})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	srv := gonsole.NewServer("", handler, gonsole.Timeouts{
		ReadHeader: 10 * time.Second, Read: time.Minute, Idle: time.Minute,
	})
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Close() })
	return listener.Addr().String()
}

// silentBody opens a connection that announces a graph request body and never sends it.
func silentBody(t *testing.T, addr string) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dialing: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	head := "POST /api/graphql HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{"
	if _, err := fmt.Fprintf(conn, head, addr); err != nil {
		t.Fatalf("writing the request head: %v", err)
	}
}

// signInStatus posts the login mutation over a fresh connection and answers its status.
func signInStatus(t *testing.T, addr string) int {
	t.Helper()
	query := `mutation { login(email: "ada@example.com", password: "` + testPassword + `") { me { id } } }`
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("encoding the login: %v", err)
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Post("http://"+addr+"/api/graphql", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("posting the login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestSilentAnonymousBodiesNeverHoldOperationSlots(t *testing.T) {
	t.Parallel()

	addr := servedOverTCP(t)
	for range 25 {
		silentBody(t, addr)
	}
	time.Sleep(300 * time.Millisecond)

	for attempt := range 10 {
		if status := signInStatus(t, addr); status != http.StatusOK {
			t.Fatalf("sign in %d under 25 silent bodies answered %d, want 200 with no slot held by a body", attempt+1, status)
		}
	}
}
