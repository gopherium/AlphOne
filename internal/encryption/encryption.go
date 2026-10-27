// SPDX-License-Identifier: Elastic-2.0

// Package encryption encrypts and decrypts small values with AES-256-GCM under a current key and any previous ones.
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// KeySize is the length of an AES-256 key in bytes.
const KeySize = 32

// formatVersion is the first byte of every envelope.
const formatVersion = 1

// headerSize counts the version byte and the key id byte that lead every envelope.
const headerSize = 2

// Encryption errors.
var (
	// ErrNoKey reports a call made while no current key is set.
	ErrNoKey = errors.New("encryption: no key is set")
	// ErrCannotDecrypt reports a value no key opens with the given associated data.
	ErrCannotDecrypt = errors.New("encryption: cannot decrypt")
	// ErrUnknownKey reports a value whose key id matches no key that is set.
	ErrUnknownKey = fmt.Errorf("%w: no key that is set has the id", ErrCannotDecrypt)
)

// key is one key that is set beside the id its envelopes carry.
type key struct {
	id   byte
	aead cipher.AEAD
}

// Keys encrypts with the current key and decrypts with the current or any previous key.
type Keys struct {
	keys []key
}

// New answers the keys that encrypt with current and decrypt with current or any of previous.
func New(current []byte, previous [][]byte) (*Keys, error) {
	if len(current) == 0 && len(previous) > 0 {
		return nil, errors.New("encryption: previous keys need a current key")
	}
	if len(current) == 0 {
		return &Keys{}, nil
	}
	keys := &Keys{keys: make([]key, 0, 1+len(previous))}
	for i, raw := range append([][]byte{current}, previous...) {
		if len(raw) != KeySize {
			return nil, fmt.Errorf("encryption: %s: must hold %d bytes, got %d", keyName(i), KeySize, len(raw))
		}
		keys.keys = append(keys.keys, newKey(raw))
	}
	return keys, nil
}

// keyName names the key at index i of the current key followed by the previous ones.
func keyName(i int) string {
	if i == 0 {
		return "current key"
	}
	return fmt.Sprintf("previous key %d", i)
}

// newKey answers the AES-256-GCM cipher over a 32 byte raw key beside the id the key itself gives.
func newKey(raw []byte) key {
	block, _ := aes.NewCipher(raw)
	aead, _ := cipher.NewGCMWithRandomNonce(block)
	sum := sha256.Sum256(raw)
	return key{id: sum[0], aead: aead}
}

// Encrypt answers plaintext encrypted under the current key and bound to additionalData.
func (k *Keys) Encrypt(plaintext []byte, additionalData ...[]byte) ([]byte, error) {
	if len(k.keys) == 0 {
		return nil, ErrNoKey
	}
	current := k.keys[0]
	header := []byte{formatVersion, current.id}
	envelope := make([]byte, 0, headerSize+current.aead.Overhead()+len(plaintext))
	return current.aead.Seal(append(envelope, header...), nil, plaintext, associated(header, additionalData)), nil
}

// Decrypt answers the plaintext of ciphertext, ErrCannotDecrypt when no key opens it with additionalData.
func (k *Keys) Decrypt(ciphertext []byte, additionalData ...[]byte) ([]byte, error) {
	if len(k.keys) == 0 {
		return nil, ErrNoKey
	}
	if len(ciphertext) < headerSize {
		return nil, fmt.Errorf("%w: %d bytes is shorter than the header", ErrCannotDecrypt, len(ciphertext))
	}
	if ciphertext[0] != formatVersion {
		return nil, fmt.Errorf("%w: unknown format version %d", ErrCannotDecrypt, ciphertext[0])
	}
	return k.open(ciphertext[1], ciphertext[headerSize:], associated(ciphertext[:headerSize], additionalData))
}

// open answers what encrypted opens to under the first key with the id that opens it with additionalData.
func (k *Keys) open(id byte, encrypted, additionalData []byte) ([]byte, error) {
	matched := false
	for _, candidate := range k.keys {
		if candidate.id != id {
			continue
		}
		matched = true
		if plaintext, err := candidate.aead.Open(nil, nil, encrypted, additionalData); err == nil {
			return plaintext, nil
		}
	}
	if !matched {
		return nil, fmt.Errorf("%w 0x%02x", ErrUnknownKey, id)
	}
	return nil, fmt.Errorf("%w: the key with the id 0x%02x does not open it with this associated data",
		ErrCannotDecrypt, id)
}

// associated answers the header followed by every part, each part led by its length as a uvarint.
func associated(header []byte, parts [][]byte) []byte {
	joined := append([]byte(nil), header...)
	for _, part := range parts {
		joined = binary.AppendUvarint(joined, uint64(len(part)))
		joined = append(joined, part...)
	}
	return joined
}
