// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveByDefault is the Dockerfile line that runs serve when the image is started with no command.
const serveByDefault = `CMD ["serve"]`

// entrypointLines returns the last ENTRYPOINT line of the Dockerfile, the line right after it and the lines below that.
func entrypointLines(t *testing.T) (entrypoint, next string, below []string) {
	t.Helper()
	dockerfile, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	if err != nil {
		t.Fatalf("reading the Dockerfile: %v", err)
	}
	lines := strings.Split(string(dockerfile), "\n")
	at := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "ENTRYPOINT ") {
			at = i
		}
	}
	if at < 0 {
		t.Fatal("the Dockerfile names no ENTRYPOINT, want the binary as the entrypoint")
	}
	if at+1 < len(lines) {
		next = strings.TrimSpace(lines[at+1])
		below = lines[at+2:]
	}
	return strings.TrimSpace(lines[at]), next, below
}

func TestTheImageServesWhenNoCommandIsNamed(t *testing.T) {
	t.Parallel()

	entrypoint, next, below := entrypointLines(t)

	if next != serveByDefault {
		t.Errorf("the line after %s is %q, want %s, or a container started with no command lists the commands",
			entrypoint, next, serveByDefault)
	}
	for _, line := range below {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == "CMD" {
			t.Errorf("%q follows %s, want no later CMD, or it replaces serve", strings.TrimSpace(line), serveByDefault)
		}
	}
}
