// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// unnamedFile is the name a download saves under when nothing of its file name is safe to keep.
const unnamedFile = "attachment"

// downloads serves the kept files of mail to the workspace each request serves.
type downloads struct {
	// db reads the row that names each kept file.
	db rowQuerier
	// store opens each kept file.
	store sdk.FileStore
}

// keptDownload is one kept file of a mail as a download serves it.
type keptDownload struct {
	// fileID names the file in the file store.
	fileID uuid.UUID
	// name is the name a browser saves the file under.
	name string
	// contentType is the media type the file is served under.
	contentType string
	// sha256 is the hash of the file.
	sha256 []byte
	// storedAt is when the row naming the file was written.
	storedAt time.Time
}

// downloadFinder reads the kept file id names in the workspace tenant, wrapping pgx.ErrNoRows when it keeps none.
type downloadFinder func(ctx context.Context, db rowQuerier, tenant, id uuid.UUID) (keptDownload, error)

// routes returns the router that serves an attachment and the original of a mail by their ids.
func (d *downloads) routes() http.Handler {
	router := chi.NewRouter()
	router.Get("/attachments/{id}", d.serveWith(readAttachmentDownload))
	router.Get("/messages/{id}/original", d.serveWith(readOriginalDownload))
	return router
}

// serveWith returns the handler that serves the kept file find reads for the id the path names.
func (d *downloads) serveWith(find downloadFinder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		tenant := sdk.TenantOrDefault(r.Context())
		file, err := find(r.Context(), d.db, tenant, id)
		if err != nil {
			w.WriteHeader(failureStatus(err))
			return
		}
		content, err := d.store.Open(sdk.WithTenant(r.Context(), tenant), file.fileID)
		if err != nil {
			w.WriteHeader(failureStatus(err))
			return
		}
		defer func() { _ = content.Close() }()
		sdk.ServeDownload(w, r, sdk.Download{
			Content: content, ContentType: file.contentType, Filename: file.name,
			SHA256: hex.EncodeToString(file.sha256), ModTime: file.storedAt,
		})
	}
}

// failureStatus returns the status a failed read or open answers, not found for a file the workspace does not keep.
func failureStatus(err error) int {
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sdk.ErrFileNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// readAttachmentDownload reads the kept attachment id names in the workspace tenant, under its safe name.
func readAttachmentDownload(ctx context.Context, db rowQuerier, tenant, id uuid.UUID) (keptDownload, error) {
	var file keptDownload
	var filename string
	err := db.QueryRow(ctx, `SELECT file_id, filename, content_type, sha256, created_at
		FROM plugin_email.attachments
		WHERE tenant_id = $1 AND id = $2 AND state = 'stored' AND file_id IS NOT NULL`,
		tenant, id).Scan(&file.fileID, &filename, &file.contentType, &file.sha256, &file.storedAt)
	if err != nil {
		return keptDownload{}, fmt.Errorf("email: read an attachment to download: %w", err)
	}
	file.name = downloadName(filename)
	return file, nil
}

// readOriginalDownload reads the kept original of the mail id names in the workspace tenant, under a name of its id.
func readOriginalDownload(ctx context.Context, db rowQuerier, tenant, id uuid.UUID) (keptDownload, error) {
	file := keptDownload{name: "original-" + id.String() + ".eml", contentType: rawMailType}
	err := db.QueryRow(ctx, `SELECT raw_file_id, raw_sha256, created_at FROM plugin_email.messages
		WHERE tenant_id = $1 AND id = $2 AND raw_state = 'stored' AND raw_file_id IS NOT NULL`,
		tenant, id).Scan(&file.fileID, &file.sha256, &file.storedAt)
	if err != nil {
		return keptDownload{}, fmt.Errorf("email: read an original to download: %w", err)
	}
	return file, nil
}

// downloadName returns the last path part of filename with no control or format character and no end dot or blank.
func downloadName(filename string) string {
	last := filename[strings.LastIndexAny(filename, `/\`)+1:]
	visible := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, last)
	trimmed := strings.TrimFunc(visible, func(r rune) bool { return r == '.' || unicode.IsSpace(r) })
	if trimmed == "" {
		return unnamedFile
	}
	return trimmed
}
