// SPDX-License-Identifier: Elastic-2.0

package sdk

import "context"

// Encrypter encrypts a plugin's own values for the workspace each call serves.
type Encrypter interface {
	// Encrypt answers plaintext encrypted for the calling workspace, this plugin and purpose.
	Encrypt(ctx context.Context, purpose string, plaintext []byte) ([]byte, error)
	// Decrypt answers what Encrypt took, ErrCannotDecrypt for another workspace, plugin or purpose.
	Decrypt(ctx context.Context, purpose string, ciphertext []byte) ([]byte, error)
}

// EncrypterConsumer receives the host's encrypter for the plugin.
type EncrypterConsumer interface {
	UseEncrypter(encrypter Encrypter)
}
