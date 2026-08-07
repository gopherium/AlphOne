// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type importSummary struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	Filename      string    `json:"filename"`
	State         string    `json:"state"`
	RowCount      int       `json:"row_count"`
	ImportedCount int       `json:"imported_count"`
	SkippedCount  int       `json:"skipped_count"`
	FailedCount   int       `json:"failed_count"`
	CreatedAt     time.Time `json:"created_at"`
}

type importDetail struct {
	importSummary
	Columns []string `json:"columns"`
	Mapping mapping  `json:"mapping"`
}

type rowResponse struct {
	ID        uuid.UUID  `json:"id"`
	Position  int        `json:"position"`
	Cells     []string   `json:"cells"`
	Outcome   string     `json:"outcome"`
	Reason    *string    `json:"reason"`
	ContactID *uuid.UUID `json:"contact_id"`
}

type importContactResponse struct {
	ContactID uuid.UUID `json:"contact_id"`
	Name      string    `json:"name"`
	RowID     uuid.UUID `json:"row_id"`
}

type importContactsResponse struct {
	Contacts []importContactResponse `json:"contacts"`
}

// newImportSummary builds the history payload of one stored import.
func newImportSummary(stored importRow) importSummary {
	return importSummary{
		ID:            stored.ID,
		UserID:        stored.UserID,
		Filename:      stored.Filename,
		State:         stored.State,
		RowCount:      stored.RowCount,
		ImportedCount: stored.ImportedCount,
		SkippedCount:  stored.SkippedCount,
		FailedCount:   stored.FailedCount,
		CreatedAt:     stored.CreatedAt.UTC(),
	}
}

// handleImportList returns a handler listing every stored import, newest first.
func (p *Plugin) handleImportList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stored, err := p.store.listImports(r.Context())
		if err != nil {
			respondError(w, http.StatusInternalServerError, "the imports could not be read")
			return
		}
		summaries := make([]importSummary, 0, len(stored))
		for _, one := range stored {
			summaries = append(summaries, newImportSummary(one))
		}
		respondJSON(w, http.StatusOK, summaries)
	}
}

// handleImportGet returns a handler answering with one import, its columns, and its mapping.
func (p *Plugin) handleImportGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stored, ok := p.loadImport(w, r)
		if !ok {
			return
		}
		respondJSON(w, http.StatusOK, importDetail{
			importSummary: newImportSummary(stored),
			Columns:       stored.Columns,
			Mapping:       stored.Mapping,
		})
	}
}

// handleRowList returns a handler listing the staged rows of one import.
func (p *Plugin) handleRowList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stored, ok := p.loadImport(w, r)
		if !ok {
			return
		}
		staged, err := p.store.listRows(r.Context(), stored.ID, maxRows)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "the rows could not be read")
			return
		}
		rows := make([]rowResponse, 0, len(staged))
		for _, one := range staged {
			rows = append(rows, rowResponse(one))
		}
		respondJSON(w, http.StatusOK, rows)
	}
}

// handleImportContacts returns a handler listing the contacts one import created.
func (p *Plugin) handleImportContacts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stored, ok := p.loadImport(w, r)
		if !ok {
			return
		}
		linked, err := p.store.listImportContacts(r.Context(), stored.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "the contacts could not be read")
			return
		}
		contacts := make([]importContactResponse, 0, len(linked))
		for _, one := range linked {
			contacts = append(contacts, importContactResponse(one))
		}
		respondJSON(w, http.StatusOK, importContactsResponse{Contacts: contacts})
	}
}

// loadImport returns the import named in the request path, answering the caller itself when it cannot.
func (p *Plugin) loadImport(w http.ResponseWriter, r *http.Request) (importRow, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "the import id is malformed")
		return importRow{}, false
	}
	stored, err := p.store.importByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		respondError(w, http.StatusNotFound, "the import does not exist")
		return importRow{}, false
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "the import could not be read")
		return importRow{}, false
	}
	return stored, true
}
