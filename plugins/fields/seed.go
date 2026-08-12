// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The demo field stored by [Plugin.Seed], for development only.
const (
	seedFieldName   = "birthDate"
	seedFieldLabel  = "Birth date"
	seedFieldValue  = "1990-04-17"
	seedContactName = "Maria Perez"
)

// Seed stores the demo field and its value on the demo contact.
func (p *Plugin) Seed(ctx context.Context) error {
	if err := p.seedDefinition(ctx); err != nil {
		return err
	}
	if err := p.catalog.reload(ctx); err != nil {
		return err
	}
	return p.seedValue(ctx)
}

// seedDefinition stores the demo definition unless the catalogue holds it.
func (p *Plugin) seedDefinition(ctx context.Context) error {
	held, err := p.store.allDefinitions(ctx)
	if err != nil {
		return err
	}
	for _, definition := range held {
		if definition.Name == seedFieldName && definition.ArchivedAt == nil {
			return nil
		}
	}
	return p.store.define(ctx, Definition{
		ID:        uuid.Must(uuid.NewV7()),
		Name:      seedFieldName,
		Label:     seedFieldLabel,
		Kind:      kindDate,
		CreatedAt: time.Now().UTC(),
	})
}

// seedValue writes the demo value onto the demo contact when one exists.
func (p *Plugin) seedValue(ctx context.Context) error {
	const query = `SELECT id FROM core.contacts WHERE name = $1 ORDER BY created_at, id LIMIT 1`
	var contactID uuid.UUID
	err := p.pool.QueryRow(ctx, query, seedContactName).Scan(&contactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("fields: seed contact lookup: %w", err)
	}
	return p.store.writeValues(ctx, contactID, map[string]any{seedFieldName: seedFieldValue})
}
