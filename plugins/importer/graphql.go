// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/sdk"
)

// Graph resolver errors.
var (
	errImportNotFound  = errors.New("importer: import not found")
	errInvalidRowLimit = fmt.Errorf("importer: limit must be between 1 and %d", maxRows)
	errNoUploader      = errors.New("importer: the request carries no user")
)

// graphRowLimit resolves a rows limit argument against the staging cap.
func graphRowLimit(limit *int) (int, error) {
	if limit == nil {
		return maxRows, nil
	}
	if *limit < 1 || *limit > maxRows {
		return 0, sdk.GraphError{Code: "VALIDATION", Reason: "first_out_of_range",
			Meta: map[string]any{"min": 1, "max": maxRows}, Err: errInvalidRowLimit}
	}
	return *limit, nil
}

// toGraphImport maps a stored import onto its graph model.
func toGraphImport(stored importRow) *model.ImportJob {
	return &model.ImportJob{
		ID:            stored.ID,
		UserID:        stored.UserID,
		Filename:      stored.Filename,
		State:         stored.State,
		RowCount:      stored.RowCount,
		ImportedCount: stored.ImportedCount,
		SkippedCount:  stored.SkippedCount,
		FailedCount:   stored.FailedCount,
		CreatedAt:     stored.CreatedAt.UTC(),
		Columns:       stored.Columns,
		Mapping:       toGraphMapping(stored.Mapping),
	}
}

// toGraphMapping maps stored column assignments onto the graph list in column order.
func toGraphMapping(assigned mapping) []*model.ImportAssignment {
	list := make([]*model.ImportAssignment, 0, len(assigned))
	for index, field := range assigned {
		column, err := strconv.Atoi(index)
		if err != nil {
			continue
		}
		list = append(list, &model.ImportAssignment{Column: column, Field: string(field)})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Column < list[j].Column })
	return list
}

// toGraphRow maps a staged row onto its graph model.
func toGraphRow(staged stagedRow) *model.ImportRow {
	return &model.ImportRow{
		ID:        staged.ID,
		Position:  staged.Position,
		Cells:     staged.Cells,
		Outcome:   staged.Outcome,
		Reason:    staged.Reason,
		ContactID: staged.ContactID,
	}
}

// loadImportJob returns the stored import behind id or its classified absence.
func (p *Plugin) loadImportJob(ctx context.Context, id uuid.UUID) (importRow, error) {
	stored, err := p.store.importByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return importRow{}, sdk.GraphError{Code: "NOT_FOUND", Reason: "import_not_found", Err: errImportNotFound}
	}
	if err != nil {
		return importRow{}, err
	}
	return stored, nil
}

// QueryResolvers serves the plugin's Query root fields.
type QueryResolvers struct {
	plugin *Plugin
}

// QueryResolvers returns the plugin's Query resolver set.
func (p *Plugin) QueryResolvers() QueryResolvers {
	return QueryResolvers{plugin: p}
}

// Imports lists every stored import, newest first.
func (q QueryResolvers) Imports(ctx context.Context) ([]*model.ImportJob, error) {
	stored, err := q.plugin.store.listImports(ctx)
	if err != nil {
		return nil, err
	}
	imports := make([]*model.ImportJob, len(stored))
	for i, one := range stored {
		imports[i] = toGraphImport(one)
	}
	return imports, nil
}

// ImportJob reports one stored import.
func (q QueryResolvers) ImportJob(ctx context.Context, id uuid.UUID) (*model.ImportJob, error) {
	stored, err := q.plugin.loadImportJob(ctx, id)
	if err != nil {
		return nil, err
	}
	return toGraphImport(stored), nil
}

// ImportFields lists the fields a column may be mapped onto.
func (q QueryResolvers) ImportFields(ctx context.Context) ([]*model.ImportField, error) {
	known, err := q.plugin.registry(ctx)
	if err != nil {
		return nil, err
	}
	fields := make([]*model.ImportField, len(known.fields))
	for i, field := range known.fields {
		fields[i] = &model.ImportField{
			Name:     string(field.Name),
			Label:    field.Label,
			Required: field.Required,
		}
	}
	return fields, nil
}

// ImportJobResolvers serves the import's resolver backed fields.
type ImportJobResolvers struct {
	plugin *Plugin
}

// ImportJobResolvers returns the plugin's ImportJob resolver set.
func (p *Plugin) ImportJobResolvers() ImportJobResolvers {
	return ImportJobResolvers{plugin: p}
}

// Rows lists the staged rows of the import in position order.
func (j ImportJobResolvers) Rows(
	ctx context.Context, obj *model.ImportJob, limit *int,
) ([]*model.ImportRow, error) {
	bounded, err := graphRowLimit(limit)
	if err != nil {
		return nil, err
	}
	staged, err := j.plugin.store.listRows(ctx, obj.ID, bounded)
	if err != nil {
		return nil, err
	}
	rows := make([]*model.ImportRow, len(staged))
	for i, one := range staged {
		rows[i] = toGraphRow(one)
	}
	return rows, nil
}

// Contacts lists the contacts the import created, in row order.
func (j ImportJobResolvers) Contacts(
	ctx context.Context, obj *model.ImportJob,
) ([]*model.ImportContact, error) {
	linked, err := j.plugin.store.listImportContacts(ctx, obj.ID)
	if err != nil {
		return nil, err
	}
	contacts := make([]*model.ImportContact, len(linked))
	for i, one := range linked {
		contacts[i] = &model.ImportContact{ContactID: one.ContactID, Name: one.Name, RowID: one.RowID}
	}
	return contacts, nil
}

// MutationResolvers serves the plugin's Mutation root fields.
type MutationResolvers struct {
	plugin *Plugin
}

// MutationResolvers returns the plugin's Mutation resolver set.
func (p *Plugin) MutationResolvers() MutationResolvers {
	return MutationResolvers{plugin: p}
}

// ImportUpload stages an uploaded file as a new import.
func (m MutationResolvers) ImportUpload(
	ctx context.Context, file graphql.Upload,
) (*model.ImportJob, error) {
	uploader, ok := sdk.UserFromContext(ctx)
	if !ok {
		return nil, sdk.GraphError{Code: "UNAUTHENTICATED", Reason: "authentication_required", Err: errNoUploader}
	}
	data, err := io.ReadAll(io.LimitReader(file.File, maxUploadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("importer: read upload: %w", err)
	}
	if len(data) > maxUploadBytes {
		return nil, sdk.GraphError{Code: "VALIDATION", Reason: "file_too_large",
			Meta: map[string]any{"maxBytes": maxUploadBytes}, Err: errTooLarge}
	}
	parsed, err := Parse(data)
	if err != nil {
		return nil, sdk.GraphError{Code: "VALIDATION", Reason: "file_unreadable", Err: err}
	}
	stored, err := m.plugin.store.insertImport(ctx, uploader, filepath.Base(file.Filename), parsed)
	if err != nil {
		return nil, err
	}
	return toGraphImport(stored), nil
}

// ImportSetMapping stores the column assignments of an import.
func (m MutationResolvers) ImportSetMapping(
	ctx context.Context, id uuid.UUID, assignments []*model.ImportAssignmentInput,
) (*model.ImportJob, error) {
	stored, err := m.plugin.loadImportJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if stored.State != stateReady {
		return nil, sdk.GraphError{Code: "CONFLICT", Reason: "mapping_locked", Err: errMappingLocked}
	}
	known, err := m.plugin.registry(ctx)
	if err != nil {
		return nil, err
	}
	assigned, err := buildMapping(toAssignments(assignments), len(stored.Columns), known)
	if err != nil {
		return nil, sdk.GraphError{Code: "VALIDATION", Reason: "mapping_invalid", Err: err}
	}
	if err := m.plugin.store.updateMapping(ctx, stored.ID, assigned); err != nil {
		return nil, err
	}
	stored.Mapping = assigned
	return toGraphImport(stored), nil
}

// toAssignments maps the graph inputs onto the plugin's assignments.
func toAssignments(inputs []*model.ImportAssignmentInput) []assignment {
	converted := make([]assignment, len(inputs))
	for i, input := range inputs {
		converted[i] = assignment{Column: input.Column, Field: fieldName(input.Field)}
	}
	return converted
}

// ImportCommit turns the staged rows of an import into contacts.
func (m MutationResolvers) ImportCommit(
	ctx context.Context, id uuid.UUID,
) (*model.ImportCommitPayload, error) {
	stored, err := m.plugin.loadImportJob(ctx, id)
	if err != nil {
		return nil, err
	}
	known, err := m.plugin.registry(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkEntry(stored, known); err != nil {
		return nil, sdk.GraphError{Code: "VALIDATION", Reason: "mapping_invalid", Err: err}
	}
	claimed, err := m.plugin.store.claimForCommit(ctx, stored.ID)
	if err != nil {
		return nil, classifyClaimError(err)
	}
	if err := m.plugin.commitRows(ctx, stored.ID, claimed, known); err != nil {
		return nil, err
	}
	counts, err := m.plugin.store.finishCommit(ctx, stored.ID)
	if err != nil {
		return nil, err
	}
	m.plugin.announceCompletion(ctx, stored.ID, counts)
	return &model.ImportCommitPayload{
		ID:       stored.ID,
		Imported: counts.Imported,
		Skipped:  counts.Skipped,
		Failed:   counts.Failed,
	}, nil
}

// classifyClaimError maps a refused commit onto its graph code.
func classifyClaimError(err error) error {
	switch {
	case errors.Is(err, errAlreadyCommitted):
		return sdk.GraphError{Code: "CONFLICT", Reason: "already_committed", Err: err}
	case errors.Is(err, errNoMapping), errors.Is(err, pgx.ErrNoRows):
		return sdk.GraphError{Code: "VALIDATION", Reason: "mapping_required", Err: errNoMapping}
	}
	return err
}
