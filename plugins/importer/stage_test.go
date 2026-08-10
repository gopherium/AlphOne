// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/sdk"
)

// newUploadPlugin returns a migrated plugin and an assertion pool over its database.
func newUploadPlugin(t *testing.T) (*importer.Plugin, *pgxpool.Pool) {
	t.Helper()
	cfg := newTestDatabase(t)
	p := newPlugin(t, cfg.URL())
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return p, newAssertionPool(t, cfg.URL())
}

// uploadNamed stages one CSV under the given filename and returns its import id.
func uploadNamed(t *testing.T, p *importer.Plugin, filename, content string) uuid.UUID {
	t.Helper()
	ctx := sdk.WithUser(t.Context(), uuid.Must(uuid.NewV7()))
	staged, err := p.MutationResolvers().ImportUpload(ctx, graphql.Upload{
		File:     strings.NewReader(content),
		Filename: filename,
		Size:     int64(len(content)),
	})
	if err != nil {
		t.Fatalf("ImportUpload() error = %v, want nil", err)
	}
	return staged.ID
}

// mapNameAndEmail assigns the first column to name and the second to email.
func mapNameAndEmail(t *testing.T, p *importer.Plugin, id uuid.UUID) {
	t.Helper()
	_, err := p.MutationResolvers().ImportSetMapping(t.Context(), id, []*model.ImportAssignmentInput{
		{Column: 0, Field: "name"},
		{Column: 1, Field: "email"},
	})
	if err != nil {
		t.Fatalf("ImportSetMapping() error = %v, want nil", err)
	}
}

// commitImport commits one import, returning its counts or the refusal.
func commitImport(
	t *testing.T, p *importer.Plugin, id uuid.UUID,
) (*model.ImportCommitPayload, error) {
	t.Helper()
	return p.MutationResolvers().ImportCommit(t.Context(), id)
}

// mustCommit commits one import, failing the test on any refusal.
func mustCommit(t *testing.T, p *importer.Plugin, id uuid.UUID) *model.ImportCommitPayload {
	t.Helper()
	committed, err := commitImport(t, p, id)
	if err != nil {
		t.Fatalf("ImportCommit() error = %v, want nil", err)
	}
	return committed
}

// refusalCode returns the graph code an error carries.
func refusalCode(t *testing.T, err error) string {
	t.Helper()
	var refused sdk.GraphError
	if !errors.As(err, &refused) {
		t.Fatalf("error = %v, want a graph error", err)
	}
	return refused.Code
}
