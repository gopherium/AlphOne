// SPDX-License-Identifier: Elastic-2.0

package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"strings"
	"time"

	"github.com/google/uuid"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"

	"github.com/gopherium/alphone/sdk"
)

// octetStream is the content type of a file saved without one.
const octetStream = "application/octet-stream"

// errNoSubtype reports a content type that names a type without a subtype.
var errNoSubtype = errors.New("want a type and a subtype, such as text/plain")

// fileStore keeps the files of one plugin under keys of their own for each workspace.
type fileStore struct {
	// bucket returns the bucket the files live in.
	bucket func(context.Context) (*blob.Bucket, error)
	// plugin is the id of the plugin the files belong to.
	plugin string
	// maxBytes is the most bytes one file holds.
	maxBytes int64
}

// NewFileStore returns the file store of the plugin pluginID over bucket, each file held to maxBytes.
func NewFileStore(bucket *blob.Bucket, pluginID string, maxBytes int64) sdk.FileStore {
	return fileStore{
		bucket:   func(context.Context) (*blob.Bucket, error) { return bucket, nil },
		plugin:   pluginID,
		maxBytes: maxBytes,
	}
}

// key returns the key of the file id of the workspace ctx serves.
func (s fileStore) key(ctx context.Context, id uuid.UUID) string {
	return "t/" + sdk.TenantOrDefault(ctx).String() + "/" + s.plugin + "/" + id.String()
}

// Save streams body under a new id of the workspace ctx serves.
func (s fileStore) Save(ctx context.Context, contentType string, body io.Reader) (sdk.StoredFile, error) {
	contentType, err := checkedType(contentType)
	if err != nil {
		return sdk.StoredFile{}, err
	}
	bucket, err := s.bucket(ctx)
	if err != nil {
		return sdk.StoredFile{}, err
	}
	id := uuid.Must(uuid.NewV7())
	writeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	writer, err := bucket.NewWriter(writeCtx, s.key(ctx, id), &blob.WriterOptions{ContentType: contentType})
	if err != nil {
		return sdk.StoredFile{}, fmt.Errorf("storage: open the writer: %w", err)
	}
	size, sha, err := s.write(writer, cancel, body)
	if err != nil {
		return sdk.StoredFile{}, err
	}
	return sdk.StoredFile{ID: id, Size: size, ContentType: contentType, SHA256: sha, StoredAt: time.Now().UTC()}, nil
}

// checkedType returns the content type a file is saved under in standard form, application/octet-stream when empty.
func checkedType(contentType string) (string, error) {
	if contentType == "" {
		return octetStream, nil
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err == nil && !strings.Contains(mediaType, "/") {
		err = errNoSubtype
	}
	if err != nil {
		return "", fmt.Errorf("storage: the content type %q: %w", contentType, err)
	}
	return mime.FormatMediaType(mediaType, params), nil
}

// write streams body into writer through one byte past the cap and returns its size and hex SHA-256, or aborts.
func (s fileStore) write(writer *blob.Writer, abort context.CancelFunc, body io.Reader) (int64, string, error) {
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(writer, hash), io.LimitReader(body, min(s.maxBytes, math.MaxInt64-1)+1))
	if err == nil && size > s.maxBytes {
		err = sdk.ErrFileTooLarge
	}
	if err != nil {
		abort()
		_ = writer.Close()
		return 0, "", copyFailure(err)
	}
	if err := writer.Close(); err != nil {
		return 0, "", fmt.Errorf("storage: store the file: %w", err)
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

// copyFailure returns ErrFileTooLarge for a body past the cap, and err wrapped otherwise.
func copyFailure(err error) error {
	if errors.Is(err, sdk.ErrFileTooLarge) {
		return err
	}
	return fmt.Errorf("storage: write the file: %w", err)
}

// Open returns the file under id of the workspace ctx serves.
func (s fileStore) Open(ctx context.Context, id uuid.UUID) (io.ReadSeekCloser, error) {
	bucket, err := s.bucket(ctx)
	if err != nil {
		return nil, err
	}
	reader, err := bucket.NewReader(ctx, s.key(ctx, id), nil)
	if err != nil {
		return nil, missingOr(err)
	}
	return reader, nil
}

// Delete removes the file under id of the workspace ctx serves.
func (s fileStore) Delete(ctx context.Context, id uuid.UUID) error {
	bucket, err := s.bucket(ctx)
	if err != nil {
		return err
	}
	return missingOr(bucket.Delete(ctx, s.key(ctx, id)))
}

// missingOr returns nil for nil, ErrFileNotFound for a key the bucket lacks, and err wrapped otherwise.
func missingOr(err error) error {
	switch {
	case err == nil:
		return nil
	case gcerrors.Code(err) == gcerrors.NotFound:
		return sdk.ErrFileNotFound
	}
	return fmt.Errorf("storage: %w", err)
}
