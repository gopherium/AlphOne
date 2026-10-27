// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/gopherium/framework/gonsole"

	"github.com/gopherium/alphone/internal/encryption"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/sdk"
)

// encryptionKeySetting names the setting holding the key new secrets are encrypted with, past the program prefix.
const encryptionKeySetting = "ENCRYPTION_KEY"

// previousEncryptionKeysSetting names the setting listing the keys older secrets still open with.
const previousEncryptionKeysSetting = "ENCRYPTION_PREVIOUS_KEYS"

// pluginPurpose leads the associated data every value a plugin encrypts is bound to.
const pluginPurpose = "plugin"

// loadEncryptionKeys reads the key new secrets are encrypted with and the keys older ones still open with.
func loadEncryptionKeys(env gonsole.Env) (*encryption.Keys, error) {
	current, err := gonsole.Parse(env, encryptionKeySetting, nil, parseEncryptionKey)
	if err != nil {
		return nil, err
	}
	previous, err := gonsole.Parse(env, previousEncryptionKeysSetting, nil, parseEncryptionKeys)
	if err != nil {
		return nil, err
	}
	if current == nil && previous != nil {
		return nil, fmt.Errorf("%s is set but %s is not",
			env.Key(previousEncryptionKeysSetting), env.Key(encryptionKeySetting))
	}
	return encryption.New(current, previous)
}

// parseEncryptionKey reads one key written as 64 hex characters, leaving the value out of every error.
func parseEncryptionKey(value string) ([]byte, error) {
	key, err := hex.DecodeString(value)
	if err != nil {
		return nil, errors.New("must be written in hex, as openssl rand -hex 32 prints it")
	}
	if len(key) != encryption.KeySize {
		return nil, fmt.Errorf("must hold %d bytes, got %d", encryption.KeySize, len(key))
	}
	return key, nil
}

// parseEncryptionKeys reads a comma list of keys, each written as 64 hex characters.
func parseEncryptionKeys(value string) ([][]byte, error) {
	var keys [][]byte
	for at, entry := range strings.Split(value, ",") {
		key, err := parseEncryptionKey(strings.TrimSpace(entry))
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", at+1, err)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// wireSettingStores hands every plugin taking a settings store a store of its own over settings.
func wireSettingStores(registered []sdk.Plugin, settings *postgres.SettingStore) {
	for _, plugin := range registered {
		if consumer, ok := plugin.(sdk.SettingStoreConsumer); ok {
			consumer.UseSettingStore(settings.ForPlugin(plugin.ID()))
		}
	}
}

// wireEncrypters hands every plugin taking an encrypter one bound to the plugin over keys.
func wireEncrypters(registered []sdk.Plugin, keys *encryption.Keys) {
	for _, plugin := range registered {
		if consumer, ok := plugin.(sdk.EncrypterConsumer); ok {
			consumer.UseEncrypter(pluginEncrypter{keys: keys, plugin: plugin.ID()})
		}
	}
}

// pluginEncrypter encrypts one plugin's own values for the workspace each call serves.
type pluginEncrypter struct {
	keys   *encryption.Keys
	plugin string
}

// Encrypt answers plaintext encrypted for the calling workspace, this plugin and purpose.
func (e pluginEncrypter) Encrypt(ctx context.Context, purpose string, plaintext []byte) ([]byte, error) {
	ciphertext, err := e.keys.Encrypt(plaintext, e.binding(ctx, purpose)...)
	if err != nil {
		return nil, sdk.ErrNoEncryptionKey
	}
	return ciphertext, nil
}

// Decrypt answers what Encrypt took, ErrCannotDecrypt for another workspace, plugin or purpose.
func (e pluginEncrypter) Decrypt(ctx context.Context, purpose string, ciphertext []byte) ([]byte, error) {
	plaintext, err := e.keys.Decrypt(ciphertext, e.binding(ctx, purpose)...)
	if errors.Is(err, encryption.ErrNoKey) {
		return nil, sdk.ErrNoEncryptionKey
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", sdk.ErrCannotDecrypt, err)
	}
	return plaintext, nil
}

// binding answers the associated data a value of the calling workspace, this plugin and purpose is bound to.
func (e pluginEncrypter) binding(ctx context.Context, purpose string) [][]byte {
	tenant := sdk.TenantOrDefault(ctx)
	return [][]byte{[]byte(pluginPurpose), tenant[:], []byte(e.plugin), []byte(purpose)}
}
