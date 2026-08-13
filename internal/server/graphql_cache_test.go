// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"os"
	"strings"
	"testing"
)

func TestGraphHandlerNeverCachesQueries(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("graphql.go")
	if err != nil {
		t.Fatalf("reading the handler source: %v", err)
	}

	if strings.Contains(string(source), "SetQueryCache") {
		t.Error("graphql.go wires SetQueryCache, want no query cache because" +
			" runtime defined fields revalidate every request against the live schema")
	}
}
