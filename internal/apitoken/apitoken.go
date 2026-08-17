// SPDX-License-Identifier: Elastic-2.0

// Package apitoken defines the bearer credential a machine client
// authenticates with.
package apitoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrEmptyName reports that a token name is empty or only whitespace.
var ErrEmptyName = errors.New("apitoken: empty name")

// ErrNotFound reports that no token exists for the presented secret.
var ErrNotFound = errors.New("apitoken: not found")

// ErrExpired reports that the presented token has outlived its expiry.
var ErrExpired = errors.New("apitoken: expired")

// ErrNegativeLifetime reports a token asked to expire before it was minted.
var ErrNegativeLifetime = errors.New("apitoken: negative lifetime")

// ErrLifetimeTooLong reports a lifetime longer than a token may be given.
var ErrLifetimeTooLong = errors.New("apitoken: lifetime too long")

// Never is the lifetime of a token that does not expire.
const Never = time.Duration(0)

// MaxLifetimeDays is the longest lifetime a token may be given in whole days.
const MaxLifetimeDays = 106751

// LifetimeOfDays returns the lifetime the given whole days ask for.
func LifetimeOfDays(days int) (time.Duration, error) {
	if days < 0 {
		return 0, ErrNegativeLifetime
	}
	if days > MaxLifetimeDays {
		return 0, fmt.Errorf("%w: %d days", ErrLifetimeTooLong, days)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

// Prefix identifies an AlphOne API token.
const Prefix = "a1_"

// secretBytes is how much entropy backs the part of a secret after [Prefix].
const secretBytes = 32

// defaultRandRead is the entropy source a secret is built from.
var defaultRandRead = rand.Read

// randRead draws the entropy a secret is built from.
var randRead = defaultRandRead

// Token is a bearer credential acting as the user who created it.
type Token struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Name       string
	Hash       string
	Scopes     Scopes
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  time.Time
}

// Expired reports whether the token had outlived its expiry at the given moment.
func (t Token) Expired(at time.Time) bool {
	return !t.ExpiresAt.IsZero() && !at.Before(t.ExpiresAt)
}

// Minted pairs a stored [Token] with the secret shown to its creator once.
type Minted struct {
	Token  Token
	Secret string
}

// Mint returns a new token for userID, named name, granted scopes and lasting ttl.
func Mint(userID uuid.UUID, name string, scopes Scopes, ttl time.Duration) (Minted, error) {
	trimmedName, err := ValidateName(name)
	if err != nil {
		return Minted{}, err
	}
	if err := scopes.Validate(); err != nil {
		return Minted{}, err
	}
	if ttl < 0 {
		return Minted{}, ErrNegativeLifetime
	}
	granted := scopes.Canonical()
	id, err := uuid.NewV7()
	if err != nil {
		return Minted{}, fmt.Errorf("apitoken: generate id: %w", err)
	}
	entropy := make([]byte, secretBytes)
	if _, err := randRead(entropy); err != nil {
		return Minted{}, fmt.Errorf("apitoken: draw secret: %w", err)
	}
	secret := Prefix + base64.RawURLEncoding.EncodeToString(entropy)
	createdAt := time.Now().UTC()
	return Minted{
		Token: Token{
			ID:        id,
			UserID:    userID,
			Name:      trimmedName,
			Hash:      HashSecret(secret),
			Scopes:    granted,
			CreatedAt: createdAt,
			ExpiresAt: expiryAfter(createdAt, ttl),
		},
		Secret: secret,
	}, nil
}

// expiryAfter returns the moment a token minted at createdAt ends, zero for never.
func expiryAfter(createdAt time.Time, ttl time.Duration) time.Time {
	if ttl == Never {
		return time.Time{}
	}
	return createdAt.Add(ttl)
}

// HashSecret returns the digest under which secret is persisted and looked up.
func HashSecret(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

// ValidateName returns the name trimmed, or [ErrEmptyName] when blank.
func ValidateName(name string) (string, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return "", ErrEmptyName
	}
	return trimmedName, nil
}
