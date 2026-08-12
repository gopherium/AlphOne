// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/sdk"
)

// reservedNames lists the Contact fields the compiled schema owns.
var reservedNames = map[string]bool{
	"id": true, "name": true, "createdAt": true, "identities": true,
	"tasks": true, "field": true, "whatsAppConversations": true,
}

// QueryResolvers serves the plugin's Query fields.
type QueryResolvers struct {
	plugin *Plugin
}

// QueryResolvers returns the plugin's Query resolver set.
func (p *Plugin) QueryResolvers() QueryResolvers {
	return QueryResolvers{plugin: p}
}

// MutationResolvers serves the plugin's Mutation fields.
type MutationResolvers struct {
	plugin *Plugin
}

// MutationResolvers returns the plugin's Mutation resolver set.
func (p *Plugin) MutationResolvers() MutationResolvers {
	return MutationResolvers{plugin: p}
}

// toGraphDefinition maps a stored definition onto its graph model.
func toGraphDefinition(stored Definition) *model.FieldDefinition {
	return &model.FieldDefinition{
		ID:         stored.ID,
		Name:       stored.Name,
		Label:      stored.Label,
		Kind:       model.FieldKind(stored.Kind),
		ArchivedAt: stored.ArchivedAt,
	}
}

// Fields lists the catalogue.
func (q QueryResolvers) Fields(ctx context.Context, includeArchived *bool) ([]*model.FieldDefinition, error) {
	read := q.plugin.store.liveDefinitions
	if includeArchived != nil && *includeArchived {
		read = q.plugin.store.allDefinitions
	}
	held, err := read(ctx)
	if err != nil {
		return nil, err
	}
	listed := make([]*model.FieldDefinition, 0, len(held))
	for _, definition := range held {
		listed = append(listed, toGraphDefinition(definition))
	}
	return listed, nil
}

// DefineField stores a definition and republishes the catalogue.
func (m MutationResolvers) DefineField(
	ctx context.Context, name, label string, declared model.FieldKind,
) (*model.FieldDefinition, error) {
	definition, err := newDefinition(name, label, string(declared), reservedNames)
	if err != nil {
		return nil, sdk.GraphError{Code: "VALIDATION", Err: err}
	}
	if err := m.plugin.store.create(ctx, definition); err != nil {
		if errors.Is(err, errNameTaken) {
			return nil, sdk.GraphError{Code: "CONFLICT", Err: err}
		}
		return nil, err
	}
	if err := m.plugin.catalog.reload(ctx); err != nil {
		return nil, err
	}
	return toGraphDefinition(definition), nil
}

// ArchiveField archives a definition and republishes the catalogue.
func (m MutationResolvers) ArchiveField(ctx context.Context, id uuid.UUID) (bool, error) {
	if err := m.plugin.store.archive(ctx, id); err != nil {
		if errors.Is(err, errNoDefinition) {
			return false, sdk.GraphError{Code: "NOT_FOUND", Err: err}
		}
		return false, err
	}
	if err := m.plugin.catalog.reload(ctx); err != nil {
		return false, err
	}
	return true, nil
}
