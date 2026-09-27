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

	"github.com/gopherium/alphone/sdk"
)

// seedContactEmail is the email the demo contact [Plugin.Seed] writes onto holds, for development only.
const seedContactEmail = "maria.perez@example.com"

// demoHistoryName is the repeater [Plugin.Seed] adds the demo history to, for development only.
const demoHistoryName = "history"

// demoFields are the definitions [Plugin.Seed] stores, for development only.
var demoFields = []Definition{
	{Name: "birthDate", Label: "Birth date", Kind: kindDate},
	{Name: demoHistoryName, Label: "History", Kind: kindRepeater, SubFields: []SubField{
		{Name: "date", Label: "Date", Kind: kindDate},
		{Name: "comment", Label: "Comment", Kind: kindLongText},
	}},
}

// demoValues are the plain values [Plugin.Seed] writes onto the demo contact, for development only.
var demoValues = map[string]any{
	"birthDate": "1990-04-17",
}

// demoHistory is the history [Plugin.Seed] adds to the demo contact, oldest first, for development only.
var demoHistory = []map[string]any{
	{"date": "2026-09-01", "comment": "First call about the yearly plan."},
	{"date": "2026-09-10", "comment": "Sent the offer and booked a follow-up call."},
	{"date": "2026-09-18", "comment": "Follow-up call.\nAsked for a second quote."},
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

// seedValues writes the demo values the demo contact does not hold yet, the history cut to the cap, when one exists.
func (p *Plugin) seedValues(ctx context.Context) error {
	const query = `SELECT contact_id FROM core.contact_identities
		WHERE channel = 'email' AND identifier = $1 AND tenant_id = $2 ORDER BY created_at, id LIMIT 1`
	var contactID uuid.UUID
	err := p.pool.QueryRow(ctx, query, seedContactEmail, sdk.TenantOrDefault(ctx)).Scan(&contactID)
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
	if _, kept := held[contactID][demoHistoryName]; !kept {
		history := demoHistory[max(0, len(demoHistory)-p.entriesMax):]
		if _, err := p.store.addEntry(ctx, contactID, demoHistoryName, history, p.entriesMax); err != nil {
			return fmt.Errorf("fields: seed history: %w", err)
		}
	}
	return p.store.writeValues(ctx, contactID, missing)
}
