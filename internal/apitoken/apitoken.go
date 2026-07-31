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
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// Minted pairs a stored [Token] with the secret shown to its creator once.
type Minted struct {
	Token  Token
	Secret string
}

// Mint returns a new token for userID, named name.
func Mint(userID uuid.UUID, name string) (Minted, error) {
	trimmedName, err := ValidateName(name)
	if err != nil {
		return Minted{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Minted{}, fmt.Errorf("apitoken: generate id: %w", err)
	}
	entropy := make([]byte, secretBytes)
	if _, err := randRead(entropy); err != nil {
		return Minted{}, fmt.Errorf("apitoken: draw secret: %w", err)
	}
	secret := Prefix + base64.RawURLEncoding.EncodeToString(entropy)
	return Minted{
		Token: Token{
			ID:        id,
			UserID:    userID,
			Name:      trimmedName,
			Hash:      HashSecret(secret),
			CreatedAt: time.Now().UTC(),
		},
		Secret: secret,
	}, nil
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
