// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
)

// ErrFileNotFound reports an id the calling workspace and plugin hold no file under.
var ErrFileNotFound = errors.New("sdk: file not found")

// ErrFileTooLarge reports a file longer than the most bytes the host keeps for one file.
var ErrFileTooLarge = errors.New("sdk: file too large")

// StoredFile describes one file a plugin keeps.
type StoredFile struct {
	// ID names the file within the plugin and the workspace that saved it.
	ID uuid.UUID
	// Size is the length of the file in bytes.
	Size int64
	// ContentType is the media type the file was saved with.
	ContentType string
	// SHA256 is the SHA-256 of the file in lower case hex.
	SHA256 string
	// StoredAt is when the host stored the file.
	StoredAt time.Time
}

// FileStore keeps one plugin's files for the workspace each call serves.
type FileStore interface {
	// Save stores body under a new id, answering ErrFileTooLarge for a body over the host's cap.
	Save(ctx context.Context, contentType string, body io.Reader) (StoredFile, error)
	// Open returns the file under id, answering ErrFileNotFound when the workspace holds none.
	Open(ctx context.Context, id uuid.UUID) (io.ReadSeekCloser, error)
	// Delete removes the file under id, answering ErrFileNotFound when the workspace holds none.
	Delete(ctx context.Context, id uuid.UUID) error
}

// FileStoreConsumer receives the host's file store for the plugin.
type FileStoreConsumer interface {
	UseFileStore(store FileStore)
}
