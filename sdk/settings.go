// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"context"
	"errors"
)

// ErrSecretSetting reports a plain read of a key that holds a secret.
var ErrSecretSetting = errors.New("sdk: the setting holds a secret")

// ErrPlainSetting reports a secret read of a key that holds a plain value.
var ErrPlainSetting = errors.New("sdk: the setting holds a plain value")

// ErrNoEncryptionKey reports a secret written or read while the host holds no encryption key.
var ErrNoEncryptionKey = errors.New("sdk: no encryption key, set ALPHONE_ENCRYPTION_KEY")

// ErrCannotDecrypt reports a value no encryption key opens for the calling workspace, plugin and purpose.
var ErrCannotDecrypt = errors.New("sdk: the value does not decrypt here")

// SettingStore keeps one plugin's settings for the workspace each call serves.
type SettingStore interface {
	// Lookup answers the plain value under key and whether it is set, ErrSecretSetting when it holds a secret.
	Lookup(ctx context.Context, key string) (string, bool, error)
	// Save stores every plain value or none of them.
	Save(ctx context.Context, values map[string]string) error
	// LookupSecret answers the secret under key and whether it is set, ErrPlainSetting when it holds a plain value.
	LookupSecret(ctx context.Context, key string) (string, bool, error)
	// SaveSecrets encrypts and stores every secret or none of them.
	SaveSecrets(ctx context.Context, secrets map[string]string) error
	// HasSecret answers whether key holds a secret that is not empty, without handing it out.
	HasSecret(ctx context.Context, key string) (bool, error)
	// LookupPrefix answers the plain values whose keys start with prefix, leaving secrets out.
	LookupPrefix(ctx context.Context, prefix string) (map[string]string, error)
}

// SettingStoreConsumer receives the host's settings store for the plugin.
type SettingStoreConsumer interface {
	UseSettingStore(store SettingStore)
}
