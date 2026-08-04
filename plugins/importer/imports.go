// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/gopherium/alphone/sdk"
)

// uploadField is the multipart field carrying the uploaded file.
const uploadField = "file"

// errNoFilePart reports a multipart body without the upload field.
var errNoFilePart = errors.New("the request carries no file part")

// errTooLarge reports an upload beyond the size cap.
var errTooLarge = errors.New("the file is larger than the importer accepts")

// Routes returns the plugin's HTTP endpoints, served relative to its namespace.
func (p *Plugin) Routes() http.Handler {
	router := chi.NewRouter()
	router.Get("/fields", p.handleFields())
	router.Post("/imports", p.handleUpload())
	router.Get("/imports", p.handleImportList())
	router.Get("/imports/{id}", p.handleImportGet())
	router.Put("/imports/{id}/mapping", p.handleMappingPut())
	router.Get("/imports/{id}/rows", p.handleRowList())
	router.Get("/imports/{id}/contacts", p.handleImportContacts())
	router.Post("/imports/{id}/commit", p.handleCommit())
	return router
}

// handleUpload returns a handler storing an uploaded file as a staged import.
func (p *Plugin) handleUpload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uploader, ok := sdk.UserFromContext(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "the request carries no user")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1)
		filename, data, err := readUpload(r)
		if err != nil {
			respondUploadError(w, err)
			return
		}
		parsed, err := Parse(data)
		if err != nil {
			respondError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		stored, err := p.store.insertImport(r.Context(), uploader, filename, parsed)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "the import could not be stored")
			return
		}
		respondJSON(w, http.StatusCreated, importDetail{
			importSummary: newImportSummary(stored),
			Columns:       stored.Columns,
			Mapping:       stored.Mapping,
		})
	}
}

// readUpload returns the name and bytes of the uploaded file part.
func readUpload(r *http.Request) (string, []byte, error) {
	parts, err := r.MultipartReader()
	if err != nil {
		return "", nil, err
	}
	part, err := filePart(parts)
	if err != nil {
		return "", nil, err
	}
	data, err := io.ReadAll(part)
	return filepath.Base(part.FileName()), data, err
}

// filePart advances the reader to the part carrying the uploaded file.
func filePart(parts *multipart.Reader) (*multipart.Part, error) {
	for {
		part, err := parts.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, errNoFilePart
		}
		if err != nil {
			return nil, err
		}
		if part.FormName() == uploadField {
			return part, nil
		}
	}
}

// respondUploadError answers the status the upload failure maps to.
func respondUploadError(w http.ResponseWriter, err error) {
	var oversized *http.MaxBytesError
	if errors.Is(err, errTooLarge) || errors.As(err, &oversized) {
		respondError(w, http.StatusUnprocessableEntity, errTooLarge.Error())
		return
	}
	respondError(w, http.StatusBadRequest, err.Error())
}

// respondError writes an error message as a JSON response body.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// respondJSON writes v as a JSON response body with the given status code.
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
