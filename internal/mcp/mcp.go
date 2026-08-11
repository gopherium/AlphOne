// SPDX-License-Identifier: Elastic-2.0

// Package mcp serves AlphOne's read only tools over the Model Context Protocol.
package mcp

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverName identifies AlphOne to a connecting agent.
const serverName = "alphone"

// readOnly describes a tool that only reads, repeatably, within AlphOne alone.
func readOnly() *mcp.ToolAnnotations {
	closed := false
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   true,
		IdempotentHint: true,
		OpenWorldHint:  &closed,
	}
}

// NewServer returns the MCP server exposing AlphOne's tools over the graph.
func NewServer(graph http.Handler, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: version}, nil)
	register(server, &tools{graph: graph})
	return server
}

// Handler returns the Streamable HTTP handler serving the MCP server.
func Handler(graph http.Handler, version string) http.Handler {
	server := NewServer(graph, version)
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		nil,
	)
}
