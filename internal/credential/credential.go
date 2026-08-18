// SPDX-License-Identifier: Elastic-2.0

// Package credential carries the attribution of a request credential.
package credential

import (
	"context"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/role"
)

// tokenPrefix namespaces attribution stamped from an API token.
const tokenPrefix = "token:"

// tokenKey is the context key carrying the API token a request presented.
type tokenKey struct{}

// roleKey is the context key carrying the tier the caller stands in.
type roleKey struct{}

// WithRole returns ctx carrying the tier its caller stands in.
func WithRole(ctx context.Context, tier role.Role) context.Context {
	return context.WithValue(ctx, roleKey{}, tier)
}

// RoleOf returns the tier the caller stands in, member when nothing stamped one.
func RoleOf(ctx context.Context) role.Role {
	tier, ok := ctx.Value(roleKey{}).(role.Role)
	if !ok {
		return role.Member
	}
	return tier
}

// Token is the API token a request authenticated with.
type Token struct {
	ID     uuid.UUID
	Name   string
	Scopes apitoken.Scopes
}

// WithToken returns ctx carrying the API token the request presented.
func WithToken(ctx context.Context, token Token) context.Context {
	return context.WithValue(ctx, tokenKey{}, token)
}

// TokenOf returns the API token the request presented, reporting whether one was.
func TokenOf(ctx context.Context) (Token, bool) {
	token, ok := ctx.Value(tokenKey{}).(Token)
	return token, ok
}

// Origin returns the attribution of the request credential, or empty.
func Origin(ctx context.Context) string {
	token, ok := TokenOf(ctx)
	if !ok {
		return ""
	}
	return tokenPrefix + token.Name
}
