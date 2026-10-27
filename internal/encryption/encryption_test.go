// SPDX-License-Identifier: Elastic-2.0

//go:debug fips140=only

package encryption_test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/fips140"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gopherium/alphone/internal/encryption"
)

// workspaceSize is the length of a workspace id in bytes.
const workspaceSize = 16

// storedKey is the full key of a secret setting the tests encrypt.
const storedKey = "plugin.email.imap.password"

// testKey answers a 32 byte key filled with b.
func testKey(b byte) []byte {
	return bytes.Repeat([]byte{b}, encryption.KeySize)
}

// keyID answers the id an envelope carries for raw.
func keyID(raw []byte) byte {
	sum := sha256.Sum256(raw)
	return sum[0]
}

// mustKeys answers the keys over current and previous, failing the test on an error.
func mustKeys(t *testing.T, current []byte, previous ...[]byte) *encryption.Keys {
	t.Helper()
	keys, err := encryption.New(current, previous)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	return keys
}

// mustEncrypt answers plaintext encrypted under keys and bound to additionalData, failing the test on an error.
func mustEncrypt(t *testing.T, keys *encryption.Keys, plaintext string, additionalData ...[]byte) []byte {
	t.Helper()
	ciphertext, err := keys.Encrypt([]byte(plaintext), additionalData...)
	if err != nil {
		t.Fatalf("Encrypt() error = %v, want nil", err)
	}
	return ciphertext
}

// mustDecrypt answers what ciphertext opens to under keys and additionalData, failing the test on an error.
func mustDecrypt(t *testing.T, keys *encryption.Keys, ciphertext []byte, additionalData ...[]byte) string {
	t.Helper()
	plaintext, err := keys.Decrypt(ciphertext, additionalData...)
	if err != nil {
		t.Fatalf("Decrypt() error = %v, want nil", err)
	}
	return string(plaintext)
}

// settingBinding answers the associated data of a stored secret in the workspace filled with workspace.
func settingBinding(workspace byte, key string) [][]byte {
	return [][]byte{[]byte("setting"), bytes.Repeat([]byte{workspace}, workspaceSize), []byte(key)}
}

// pluginBinding answers the associated data of a plugin value in the workspace filled with workspace.
func pluginBinding(workspace byte, plugin, purpose string) [][]byte {
	return [][]byte{
		[]byte("plugin"), bytes.Repeat([]byte{workspace}, workspaceSize), []byte(plugin), []byte(purpose),
	}
}

// wantCannotDecrypt fails the test unless err reports a value that does not open.
func wantCannotDecrypt(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, encryption.ErrCannotDecrypt) {
		t.Errorf("Decrypt() error = %v, want %v", err, encryption.ErrCannotDecrypt)
	}
}

func TestEveryEncryptionTestRunsInFIPSOnlyMode(t *testing.T) {
	t.Parallel()

	if !fips140.Enforced() {
		t.Fatal("fips140.Enforced() = false, want the tests run under fips140=only")
	}
	block, err := aes.NewCipher(testKey(1))
	if err != nil {
		t.Fatalf("aes.NewCipher() error = %v, want nil", err)
	}
	if _, err := cipher.NewGCM(block); err == nil {
		t.Error("cipher.NewGCM() built a cipher, want FIPS 140-only mode to turn down a caller nonce")
	}
}

func TestAValueRoundTripsUnderTheCurrentKey(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, testKey(1))
	binding := settingBinding(1, storedKey)

	ciphertext := mustEncrypt(t, keys, "correct horse battery", binding...)

	if got := mustDecrypt(t, keys, ciphertext, binding...); got != "correct horse battery" {
		t.Errorf("Decrypt() = %q, want the value encrypted", got)
	}
}

func TestAnEmptyValueRoundTrips(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, testKey(1))

	if got := mustDecrypt(t, keys, mustEncrypt(t, keys, "")); got != "" {
		t.Errorf("Decrypt() = %q, want the empty value encrypted", got)
	}
}

func TestTheEnvelopeCarriesTheVersionTheKeyIDTheNonceAndTheTag(t *testing.T) {
	t.Parallel()

	ciphertext := mustEncrypt(t, mustKeys(t, testKey(1)), "abc")

	if ciphertext[0] != 1 || ciphertext[1] != keyID(testKey(1)) {
		t.Errorf("header = %x, want 01%02x", ciphertext[:2], keyID(testKey(1)))
	}
	if want := 2 + 12 + len("abc") + 16; len(ciphertext) != want {
		t.Errorf("len(ciphertext) = %d, want %d", len(ciphertext), want)
	}
}

func TestTheAssociatedDataIsTheHeaderThenEachPartLedByItsLength(t *testing.T) {
	t.Parallel()

	ciphertext := mustEncrypt(t, mustKeys(t, testKey(1)), "correct horse battery", []byte("setting"), []byte("ab"))
	block, err := aes.NewCipher(testKey(1))
	if err != nil {
		t.Fatalf("aes.NewCipher() error = %v, want nil", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		t.Fatalf("cipher.NewGCMWithRandomNonce() error = %v, want nil", err)
	}
	joined := slices.Concat(ciphertext[:2], []byte{7}, []byte("setting"), []byte{2}, []byte("ab"))

	if plaintext, err := aead.Open(nil, nil, ciphertext[2:], joined); err != nil ||
		string(plaintext) != "correct horse battery" {
		t.Errorf("Open() over the header and the parts = %q, %v, want the value encrypted", plaintext, err)
	}
	if _, err := aead.Open(nil, nil, ciphertext[2:], joined[2:]); err == nil {
		t.Error("Open() over the parts alone opened the value, want the header bound too")
	}
}

func TestTwoEncryptionsOfOneValueDiffer(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, testKey(1))

	if bytes.Equal(mustEncrypt(t, keys, "same"), mustEncrypt(t, keys, "same")) {
		t.Error("two encryptions of one value are equal, want a new nonce each time")
	}
}

func TestDifferentAssociatedDataDoesNotOpen(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, testKey(1))
	ciphertext := mustEncrypt(t, keys, "correct horse battery", settingBinding(1, storedKey)...)
	tests := map[string][][]byte{
		"another workspace":     settingBinding(2, storedKey),
		"another setting":       settingBinding(1, "plugin.email.smtp.password"),
		"a plugin value":        pluginBinding(1, "email", "imap.password"),
		"no associated data":    nil,
		"one part left out":     settingBinding(1, storedKey)[:2],
		"one part added at end": append(settingBinding(1, storedKey), []byte("extra")),
	}

	for name, binding := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := keys.Decrypt(ciphertext, binding...)

			wantCannotDecrypt(t, err)
		})
	}
}

func TestAnotherPurposeOfOnePluginDoesNotOpen(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, testKey(1))
	ciphertext := mustEncrypt(t, keys, "token", pluginBinding(1, "whatsapp", "access_token")...)

	_, err := keys.Decrypt(ciphertext, pluginBinding(1, "whatsapp", "refresh_token")...)

	wantCannotDecrypt(t, err)
}

func TestPartsJoinedDifferentlyDoNotOpen(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, testKey(1))
	ciphertext := mustEncrypt(t, keys, "correct horse battery", []byte("a.b"), []byte("c"))

	for name, binding := range map[string][][]byte{
		"split after a":  {[]byte("a"), []byte("b.c")},
		"joined as one":  {[]byte("a.bc")},
		"joined plainly": {[]byte("a.b" + "c")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := keys.Decrypt(ciphertext, binding...)

			wantCannotDecrypt(t, err)
		})
	}
}

func TestOneChangedByteDoesNotOpen(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, testKey(1))
	binding := settingBinding(1, storedKey)
	ciphertext := mustEncrypt(t, keys, "correct horse battery", binding...)

	for at := range ciphertext {
		changed := bytes.Clone(ciphertext)
		changed[at] ^= 0x01

		if _, err := keys.Decrypt(changed, binding...); !errors.Is(err, encryption.ErrCannotDecrypt) {
			t.Errorf("byte %d changed: Decrypt() error = %v, want %v", at, err, encryption.ErrCannotDecrypt)
		}
	}
}

func TestAShortInputDoesNotOpen(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, testKey(1))
	ciphertext := mustEncrypt(t, keys, "correct horse battery")

	for _, length := range []int{0, 1, 2, 13, 29, len(ciphertext) - 1} {
		if _, err := keys.Decrypt(ciphertext[:length]); !errors.Is(err, encryption.ErrCannotDecrypt) {
			t.Errorf("%d bytes: Decrypt() error = %v, want %v", length, err, encryption.ErrCannotDecrypt)
		}
	}
}

func TestAnUnknownVersionDoesNotOpenAndIsNamed(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, testKey(1))
	ciphertext := mustEncrypt(t, keys, "correct horse battery")
	ciphertext[0] = 2

	_, err := keys.Decrypt(ciphertext)

	wantCannotDecrypt(t, err)
	if err == nil || !strings.Contains(err.Error(), "unknown format version 2") {
		t.Errorf("Decrypt() error = %v, want the version 2 named", err)
	}
}

func TestAValueOpensAfterItsKeyMovesToThePreviousList(t *testing.T) {
	t.Parallel()

	ciphertext := mustEncrypt(t, mustKeys(t, testKey(1)), "older value")

	rotated := mustKeys(t, testKey(2), testKey(1))

	if got := mustDecrypt(t, rotated, ciphertext); got != "older value" {
		t.Errorf("Decrypt() = %q, want the value encrypted before the rotation", got)
	}
}

func TestANewValueTakesTheCurrentKeyAndOpensWithItAlone(t *testing.T) {
	t.Parallel()

	ciphertext := mustEncrypt(t, mustKeys(t, testKey(2), testKey(1)), "newer value")

	if ciphertext[1] != keyID(testKey(2)) {
		t.Errorf("key id = %02x, want the current key's %02x", ciphertext[1], keyID(testKey(2)))
	}
	if got := mustDecrypt(t, mustKeys(t, testKey(2)), ciphertext); got != "newer value" {
		t.Errorf("Decrypt() with the current key alone = %q, want the newer value", got)
	}
}

func TestAReorderedPreviousListStillOpensEveryValue(t *testing.T) {
	t.Parallel()

	values := map[string][]byte{
		"first":  mustEncrypt(t, mustKeys(t, testKey(1)), "first"),
		"second": mustEncrypt(t, mustKeys(t, testKey(2)), "second"),
	}

	for _, keys := range []*encryption.Keys{
		mustKeys(t, testKey(3), testKey(1), testKey(2)),
		mustKeys(t, testKey(3), testKey(2), testKey(1)),
	} {
		for want, ciphertext := range values {
			if got := mustDecrypt(t, keys, ciphertext); got != want {
				t.Errorf("Decrypt() = %q, want %q", got, want)
			}
		}
	}
}

func TestAValueUnderAKeyNoLongerListedNamesItsKeyID(t *testing.T) {
	t.Parallel()

	ciphertext := mustEncrypt(t, mustKeys(t, testKey(1)), "correct horse battery")

	_, err := mustKeys(t, testKey(2)).Decrypt(ciphertext)

	if !errors.Is(err, encryption.ErrUnknownKey) || !errors.Is(err, encryption.ErrCannotDecrypt) {
		t.Errorf("Decrypt() error = %v, want %v wrapping %v", err, encryption.ErrUnknownKey, encryption.ErrCannotDecrypt)
	}
	if want := fmt.Sprintf("0x%02x", keyID(testKey(1))); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Decrypt() error = %v, want the key id %s named", err, want)
	}
}

// keysSharingAnID answers two different keys whose ids are the same byte.
func keysSharingAnID(t *testing.T) ([]byte, []byte) {
	t.Helper()
	seen := map[byte][]byte{}
	for i := range 1000 {
		raw := sha256.Sum256(fmt.Appendf(nil, "key %d", i))
		if other, held := seen[keyID(raw[:])]; held {
			return other, raw[:]
		}
		seen[keyID(raw[:])] = raw[:]
	}
	t.Fatal("no two keys of the thousand share an id")
	return nil, nil
}

func TestEveryKeySharingTheIDOfAValueIsTried(t *testing.T) {
	t.Parallel()

	first, second := keysSharingAnID(t)
	ciphertext := mustEncrypt(t, mustKeys(t, second), "correct horse battery")

	if got := mustDecrypt(t, mustKeys(t, testKey(9), first, second), ciphertext); got != "correct horse battery" {
		t.Errorf("Decrypt() = %q, want the key listed after its namesake tried", got)
	}
}

func TestNewTurnsDownAKeyThatIsNot32Bytes(t *testing.T) {
	t.Parallel()

	for _, length := range []int{16, 24, 31, 33} {
		_, err := encryption.New(make([]byte, length), nil)
		if want := fmt.Sprintf("current key: must hold 32 bytes, got %d", length); err == nil ||
			!strings.Contains(err.Error(), want) {
			t.Errorf("New() of a %d byte key error = %v, want %q", length, err, want)
		}
	}
	_, err := encryption.New(testKey(1), [][]byte{testKey(2), make([]byte, 16)})
	if want := "previous key 2: must hold 32 bytes, got 16"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("New() of a short previous key error = %v, want %q", err, want)
	}
}

func TestPreviousKeysNeedACurrentKey(t *testing.T) {
	t.Parallel()

	if _, err := encryption.New(nil, [][]byte{testKey(1)}); err == nil {
		t.Error("New() of previous keys without a current key error = nil, want it turned down")
	}
}

func TestWithoutAKeyNothingEncryptsOrDecrypts(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, nil)
	ciphertext := mustEncrypt(t, mustKeys(t, testKey(1)), "correct horse battery")

	if _, err := keys.Encrypt([]byte("correct horse battery")); !errors.Is(err, encryption.ErrNoKey) {
		t.Errorf("Encrypt() error = %v, want %v", err, encryption.ErrNoKey)
	}
	if _, err := keys.Decrypt(ciphertext); !errors.Is(err, encryption.ErrNoKey) {
		t.Errorf("Decrypt() error = %v, want %v", err, encryption.ErrNoKey)
	}
}

func TestTheKeysServeConcurrentCalls(t *testing.T) {
	t.Parallel()

	keys := mustKeys(t, testKey(1))
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			binding := settingBinding(byte(i), storedKey)
			ciphertext, err := keys.Encrypt([]byte("correct horse battery"), binding...)
			if err == nil {
				_, err = keys.Decrypt(ciphertext, binding...)
			}
			if err != nil {
				t.Errorf("call %d: %v", i, err)
			}
		})
	}
	wg.Wait()
}
