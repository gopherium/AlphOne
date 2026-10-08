// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/server"
)

// smallWindowConn dials the address with a tiny receive buffer and small segments.
func smallWindowConn(t *testing.T, addr string) net.Conn {
	t.Helper()
	dialer := net.Dialer{Control: func(_, _ string, raw syscall.RawConn) error {
		var set error
		if err := raw.Control(func(fd uintptr) {
			set = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 1024)
			if set == nil {
				set = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_MAXSEG, 536)
			}
		}); err != nil {
			return err
		}
		return set
	}}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dialing: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// unreadLargeAnswer asks over a connection with a small receive buffer for a large answer it never reads.
func unreadLargeAnswer(t *testing.T, addr string, cookie *http.Cookie) {
	t.Helper()
	conn := smallWindowConn(t, addr)
	var aliases strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&aliases, "a%0490d: version ", i)
	}
	body, err := json.Marshal(map[string]string{"query": "{ " + aliases.String() + "}"})
	if err != nil {
		t.Fatalf("encoding the query: %v", err)
	}
	head := "POST /api/graphql HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\n" +
		"Cookie: %s\r\nContent-Length: %d\r\n\r\n"
	if _, err := fmt.Fprintf(conn, head, addr, cookie.String(), len(body)); err != nil {
		t.Fatalf("writing the request head: %v", err)
	}
	if _, err := conn.Write(body); err != nil {
		t.Fatalf("writing the request body: %v", err)
	}
}

// awaitVersionStatus asks for the version until it answers the status or the wait runs out, reporting whether it did.
func awaitVersionStatus(
	t *testing.T, client *http.Client, addr string, cookie *http.Cookie, status int, wait time.Duration,
) bool {
	t.Helper()
	for deadline := time.Now().Add(wait); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if versionStatus(t, client, addr, cookie) == status {
			return true
		}
	}
	return false
}

func TestAnUnreadAnswerFreesItsSlotAtTheLifetime(t *testing.T) {
	t.Parallel()

	addr := servedOverTCP(t, server.GraphBounds{OperationsPerUser: 1, OperationTimeout: time.Second})
	cookie := signInCookie(t, addr)
	unreadLargeAnswer(t, addr, cookie)
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	if !awaitVersionStatus(t, client, addr, cookie, http.StatusTooManyRequests, 5*time.Second) {
		t.Fatal("the unread answer never held the only slot, want it held while unread")
	}

	if !awaitVersionStatus(t, client, addr, cookie, http.StatusOK, 8*time.Second) {
		t.Error("the slot of an unread answer was still held 8s on, want it freed at the 1s lifetime")
	}
}
