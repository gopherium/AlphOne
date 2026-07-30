// SPDX-License-Identifier: Elastic-2.0

package apitoken_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/apitoken"
)

func TestMintBuildsATokenCarryingItsSecretOnce(t *testing.T) {
	t.Parallel()

	userID := uuid.Must(uuid.NewV7())

	minted, err := apitoken.Mint(userID, "  n8n production  ")

	if err != nil {
		t.Fatalf("Mint() error = %v, want nil", err)
	}
	if !strings.HasPrefix(minted.Secret, apitoken.Prefix) {
		t.Errorf("Secret = %q, want prefix %q", minted.Secret, apitoken.Prefix)
	}
	if minted.Token.UserID != userID {
		t.Errorf("UserID = %v, want %v", minted.Token.UserID, userID)
	}
	if minted.Token.Name != "n8n production" {
		t.Errorf("Name = %q, want the name trimmed", minted.Token.Name)
	}
	if minted.Token.ID == uuid.Nil {
		t.Error("ID = zero, want a generated identifier")
	}
	if minted.Token.CreatedAt.IsZero() {
		t.Error("CreatedAt = zero, want the mint time")
	}
	if !minted.Token.LastUsedAt.IsZero() {
		t.Error("LastUsedAt = non-zero, want zero until the token is used")
	}
}

func TestMintStoresOnlyTheHashOfTheSecret(t *testing.T) {
	t.Parallel()

	minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n")
	if err != nil {
		t.Fatalf("Mint() error = %v, want nil", err)
	}

	if strings.Contains(minted.Token.Hash, minted.Secret) {
		t.Error("Hash contains the secret, want only its digest")
	}
	if got := apitoken.HashSecret(minted.Secret); got != minted.Token.Hash {
		t.Errorf("HashSecret(secret) = %q, want the stored hash %q", got, minted.Token.Hash)
	}
}

func TestMintDrawsADistinctSecretEveryTime(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool, 100)
	for range 100 {
		minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n")
		if err != nil {
			t.Fatalf("Mint() error = %v, want nil", err)
		}
		if seen[minted.Secret] {
			t.Fatalf("Mint() reused secret %q", minted.Secret)
		}
		seen[minted.Secret] = true
	}
}

func TestMintDrawsAFullyRandomSecret(t *testing.T) {
	t.Parallel()

	minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n")
	if err != nil {
		t.Fatalf("Mint() error = %v, want nil", err)
	}

	entropy, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(minted.Secret, apitoken.Prefix))
	if err != nil {
		t.Fatalf("decoding the secret: %v", err)
	}
	if len(entropy) != 32 {
		t.Errorf("secret carries %d bytes of entropy, want 32", len(entropy))
	}
}

func TestMintRejectsABlankName(t *testing.T) {
	t.Parallel()

	if _, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "   "); !errors.Is(err, apitoken.ErrEmptyName) {
		t.Errorf("Mint() error = %v, want %v", err, apitoken.ErrEmptyName)
	}
}

func TestHashSecretReturnsTheHexEncodedDigest(t *testing.T) {
	t.Parallel()

	digest := sha256.Sum256([]byte("a1_example"))

	if got, want := apitoken.HashSecret("a1_example"), hex.EncodeToString(digest[:]); got != want {
		t.Errorf("HashSecret() = %q, want %q", got, want)
	}
}

func TestValidateNameTrimsSurroundingWhitespace(t *testing.T) {
	t.Parallel()

	name, err := apitoken.ValidateName("  n8n production  ")

	if err != nil {
		t.Fatalf("ValidateName() error = %v, want nil", err)
	}
	if name != "n8n production" {
		t.Errorf("ValidateName() = %q, want %q", name, "n8n production")
	}
}
