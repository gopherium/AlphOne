// SPDX-License-Identifier: Elastic-2.0

package apitoken_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/apitoken"
)

func TestMintBuildsATokenCarryingItsSecretOnce(t *testing.T) {
	t.Parallel()

	userID := uuid.Must(uuid.NewV7())

	minted, err := apitoken.Mint(userID, "  n8n production  ", apitoken.Full(), apitoken.Never)

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

	minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n", apitoken.Full(), apitoken.Never)
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
		minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n", apitoken.Full(), apitoken.Never)
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

	minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n", apitoken.Full(), apitoken.Never)
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

	_, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "   ", apitoken.Full(), apitoken.Never)

	if !errors.Is(err, apitoken.ErrEmptyName) {
		t.Errorf("Mint() error = %v, want %v", err, apitoken.ErrEmptyName)
	}
}

func TestMintCarriesTheGrantedScopes(t *testing.T) {
	t.Parallel()

	minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n", apitoken.ParseScopes("tasks:write"), apitoken.Never)

	if err != nil {
		t.Fatalf("Mint() error = %v, want nil", err)
	}
	if got, want := minted.Token.Scopes.String(), "tasks:write"; got != want {
		t.Errorf("Scopes = %q, want %q", got, want)
	}
}

func TestMintCanonicalisesTheScopesItIsHanded(t *testing.T) {
	t.Parallel()

	granted := apitoken.Scopes{"tasks:write", "contacts:read", "tasks:write"}

	minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n", granted, apitoken.Never)

	if err != nil {
		t.Fatalf("Mint() error = %v, want nil", err)
	}
	if got, want := minted.Token.Scopes.String(), "contacts:read tasks:write"; got != want {
		t.Errorf("Scopes = %q, want %q, sorted without repeats", got, want)
	}
	if got, want := granted.String(), "tasks:write contacts:read tasks:write"; got != want {
		t.Errorf("the caller's scopes = %q, want %q, Mint must not reorder them", got, want)
	}
}

func TestMintEndsTheTokenAfterItsLifetime(t *testing.T) {
	t.Parallel()

	minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n", apitoken.Full(), 30*24*time.Hour)

	if err != nil {
		t.Fatalf("Mint() error = %v, want nil", err)
	}
	if want := minted.Token.CreatedAt.Add(30 * 24 * time.Hour); !minted.Token.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v, the mint moment plus the lifetime", minted.Token.ExpiresAt, want)
	}
}

func TestMintLeavesTheExpiryOpenWhenAskedForNever(t *testing.T) {
	t.Parallel()

	minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n", apitoken.Full(), apitoken.Never)

	if err != nil {
		t.Fatalf("Mint() error = %v, want nil", err)
	}
	if !minted.Token.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero, meaning the token never expires", minted.Token.ExpiresAt)
	}
}

func TestMintRejectsAMalformedScope(t *testing.T) {
	t.Parallel()

	_, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n", apitoken.ParseScopes("tasks:admin"), apitoken.Never)

	if !errors.Is(err, apitoken.ErrMalformedScope) {
		t.Errorf("Mint() error = %v, want %v", err, apitoken.ErrMalformedScope)
	}
}

func TestMintRejectsATokenGrantedNothing(t *testing.T) {
	t.Parallel()

	_, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n", nil, apitoken.Never)

	if !errors.Is(err, apitoken.ErrNoScopes) {
		t.Errorf("Mint() error = %v, want %v", err, apitoken.ErrNoScopes)
	}
}

func TestMintRejectsALifetimeAlreadySpent(t *testing.T) {
	t.Parallel()

	_, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "n8n", apitoken.Full(), -time.Hour)

	if !errors.Is(err, apitoken.ErrNegativeLifetime) {
		t.Errorf("Mint() error = %v, want %v", err, apitoken.ErrNegativeLifetime)
	}
}

func TestLifetimeOfDaysReadsTheDaysAsked(t *testing.T) {
	t.Parallel()

	got, err := apitoken.LifetimeOfDays(30)

	if err != nil {
		t.Fatalf("LifetimeOfDays() error = %v, want nil", err)
	}
	if want := 30 * 24 * time.Hour; got != want {
		t.Errorf("LifetimeOfDays(30) = %v, want %v", got, want)
	}
}

func TestLifetimeOfDaysAcceptsTheLongestLifetime(t *testing.T) {
	t.Parallel()

	got, err := apitoken.LifetimeOfDays(apitoken.MaxLifetimeDays)

	if err != nil {
		t.Fatalf("LifetimeOfDays(max) error = %v, want nil", err)
	}
	if got <= 0 {
		t.Errorf("LifetimeOfDays(max) = %v, want a positive lifetime", got)
	}
}

func TestLifetimeOfDaysRefusesALifetimeThatWouldOverflow(t *testing.T) {
	t.Parallel()

	for _, days := range []int{apitoken.MaxLifetimeDays + 1, 213504, 999999999} {
		got, err := apitoken.LifetimeOfDays(days)
		if !errors.Is(err, apitoken.ErrLifetimeTooLong) {
			t.Errorf("LifetimeOfDays(%d) error = %v, want %v", days, err, apitoken.ErrLifetimeTooLong)
		}
		if got != 0 {
			t.Errorf("LifetimeOfDays(%d) = %v, want no lifetime", days, got)
		}
	}
}

func TestLifetimeOfDaysRefusesALifetimeAlreadySpent(t *testing.T) {
	t.Parallel()

	if _, err := apitoken.LifetimeOfDays(-1); !errors.Is(err, apitoken.ErrNegativeLifetime) {
		t.Errorf("LifetimeOfDays(-1) error = %v, want %v", err, apitoken.ErrNegativeLifetime)
	}
}

func TestExpiredReadsTheMomentTheTokenEnds(t *testing.T) {
	t.Parallel()

	ends := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	token := apitoken.Token{ExpiresAt: ends}

	if token.Expired(ends.Add(-time.Second)) {
		t.Error("Expired(before) = true, want false")
	}
	if !token.Expired(ends) {
		t.Error("Expired(at the moment it ends) = false, want true")
	}
	if !token.Expired(ends.Add(time.Second)) {
		t.Error("Expired(after) = false, want true")
	}
}

func TestExpiredIsNeverTrueForATokenWithoutAnEnd(t *testing.T) {
	t.Parallel()

	var token apitoken.Token

	if token.Expired(time.Now().AddDate(100, 0, 0)) {
		t.Error("Expired() = true, want false, a zero expiry means never")
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
