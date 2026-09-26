// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// seedContactEmail is the email the demo contact [Plugin.Seed] writes onto holds, for development only.
const seedContactEmail = "maria.perez@example.com"

// demoFields are the definitions [Plugin.Seed] stores, for development only.
var demoFields = []Definition{
	{Name: "birthDate", Label: "Birth date", Kind: kindDate},
	{Name: "history", Label: "History", Kind: kindRepeater, SubFields: []SubField{
		{Name: "date", Label: "Date", Kind: kindDate},
		{Name: "comment", Label: "Comment", Kind: kindLongText},
	}},
}

// demoValues are the values [Plugin.Seed] writes onto the demo contact, for development only.
var demoValues = map[string]any{
	"birthDate": "1990-04-17",
	"history": []map[string]any{
		{"date": "2026-09-01", "comment": "First call about the yearly plan."},
		{"date": "2026-09-10", "comment": "Sent the offer and booked a follow-up call."},
	},
}

// Seed stores the demo fields and their values on the demo contact.
func (p *Plugin) Seed(ctx context.Context) error {
	if err := p.seedDefinitions(ctx); err != nil {
		return err
	}
	p.catalog.forget(ctx)
	return p.seedValues(ctx)
}

// seedDefinitions stores every demo definition the catalogue does not hold live.
func (p *Plugin) seedDefinitions(ctx context.Context) error {
	held, err := p.store.allDefinitions(ctx)
	if err != nil {
		return err
	}
	for _, demo := range demoFields {
		live := func(definition Definition) bool { return definition.Name == demo.Name && definition.ArchivedAt == nil }
		if slices.ContainsFunc(held, live) {
			continue
		}
		demo.ID = uuid.Must(uuid.NewV7())
		demo.CreatedAt = time.Now().UTC()
		if err := p.store.define(ctx, demo); err != nil {
			return err
		}
	}
	return nil
}

// seedValues writes the demo values the demo contact does not hold yet, when one exists.
func (p *Plugin) seedValues(ctx context.Context) error {
	const query = `SELECT contact_id FROM core.contact_identities
		WHERE channel = 'email' AND identifier = $1 ORDER BY created_at, id LIMIT 1`
	var contactID uuid.UUID
	err := p.pool.QueryRow(ctx, query, seedContactEmail).Scan(&contactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("fields: seed contact lookup: %w", err)
	}
	held, err := p.store.valuesFor(ctx, []uuid.UUID{contactID})
	if err != nil {
		return fmt.Errorf("fields: seed values lookup: %w", err)
	}
	missing := make(map[string]any, len(demoValues))
	for name, value := range demoValues {
		if _, kept := held[contactID][name]; !kept {
			missing[name] = value
		}
	}
	return p.store.writeValues(ctx, contactID, missing)
}
