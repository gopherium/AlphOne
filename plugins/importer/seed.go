// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// The demo import stored by [Plugin.Seed], for development only.
const (
	seedImportID = "0198d000-0000-7000-8000-0000000000a0"
	seedUploader = "0198d000-0000-7000-8000-0000000000a1"
	seedFilename = "spring-leads.csv"
	seedClaimed  = "ada@example.com"
)

// demoRow is one scripted row of the demo import.
type demoRow struct {
	id      string
	cells   []string
	outcome string
	reason  string
}

// demoRows returns the scripted rows of the demo import in position order.
func demoRows() []demoRow {
	return []demoRow{
		{
			id:      "0198d000-0000-7000-8000-0000000000b1",
			cells:   []string{"Maria Perez", "maria.perez@example.com", "184467235"},
			outcome: outcomeImported,
		},
		{
			id:      "0198d000-0000-7000-8000-0000000000b2",
			cells:   []string{"Grace Hopper", "grace.hopper@example.com", "184467236"},
			outcome: outcomeImported,
		},
		{
			id:      "0198d000-0000-7000-8000-0000000000b3",
			cells:   []string{"Alan Turing", "alan.turing@example.com", "184467237"},
			outcome: outcomeImported,
		},
		{
			id:      "0198d000-0000-7000-8000-0000000000b4",
			cells:   []string{"Ada Lovelace", seedClaimed, ""},
			outcome: outcomeSkipped,
			reason:  "the contact detail already belongs to a contact",
		},
		{
			id:      "0198d000-0000-7000-8000-0000000000b5",
			cells:   []string{"M. Perez", "maria.perez@example.com", ""},
			outcome: outcomeSkipped,
			reason:  "the contact detail already belongs to a contact",
		},
		{
			id:      "0198d000-0000-7000-8000-0000000000b6",
			cells:   []string{"", "", "184467238"},
			outcome: outcomeFailed,
			reason:  "the row carries no name or no contact detail",
		},
	}
}

// tally counts how many demo rows carry each outcome.
func tally(rows []demoRow) map[string]int {
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.outcome]++
	}
	return counts
}

// Seed stores a committed demo import, leaving the one an earlier run stored alone.
func (p *Plugin) Seed(ctx context.Context) error {
	id := uuid.MustParse(seedImportID)
	var exists bool
	if err := p.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM plugin_importer.imports WHERE id = $1)", id,
	).Scan(&exists); err != nil {
		return fmt.Errorf("importer: seed lookup: %w", err)
	}
	if exists {
		return nil
	}
	rows := demoRows()
	links, err := p.seedLinks(ctx, rows)
	if err != nil {
		return err
	}
	return p.seedImport(ctx, id, rows, links)
}

// seedLinks returns the contact each demo row points at, in position order.
func (p *Plugin) seedLinks(ctx context.Context, rows []demoRow) ([]*uuid.UUID, error) {
	links := make([]*uuid.UUID, len(rows))
	for i, row := range rows {
		link, err := p.seedLink(ctx, row)
		if err != nil {
			return nil, err
		}
		links[i] = link
	}
	return links, nil
}

// seedLink returns the contact one demo row points at, empty when it has none.
func (p *Plugin) seedLink(ctx context.Context, row demoRow) (*uuid.UUID, error) {
	switch row.outcome {
	case outcomeImported:
		return p.seedContact(ctx, row)
	case outcomeSkipped:
		return p.seedClaim(ctx, row)
	}
	return nil, nil
}

// seedContact stores the contact one imported demo row stands for.
func (p *Plugin) seedContact(ctx context.Context, row demoRow) (*uuid.UUID, error) {
	created, _, err := p.contacts.CreateWithIdentities(ctx, row.cells[0], []sdk.Identity{
		{Channel: sdk.Channel(fieldEmail), Identifier: row.cells[1]},
		{Channel: sdk.Channel(fieldPhone), Identifier: row.cells[2]},
	})
	if err != nil {
		return nil, fmt.Errorf("importer: seed contact: %w", err)
	}
	return &created.ID, nil
}

// seedClaim returns the contact already owning the email one skipped demo row carries.
func (p *Plugin) seedClaim(ctx context.Context, row demoRow) (*uuid.UUID, error) {
	owner, found, err := p.contacts.FindByIdentity(ctx, sdk.Channel(fieldEmail), row.cells[1])
	if err != nil {
		return nil, fmt.Errorf("importer: seed claim lookup: %w", err)
	}
	if !found {
		return nil, nil
	}
	return &owner.ID, nil
}

// seedImport stores the demo import and its scripted rows.
func (p *Plugin) seedImport(
	ctx context.Context, id uuid.UUID, rows []demoRow, links []*uuid.UUID,
) error {
	counts := tally(rows)
	if _, err := p.pool.Exec(ctx,
		"INSERT INTO plugin_importer.imports (id, user_id, filename, columns, mapping, "+
			"state, row_count, imported_count, skipped_count, failed_count, created_at) "+
			"VALUES ($1, $2, $3, $4, jsonb_object($5, $6), $7, $8, $9, $10, $11, "+
			"now() - interval '1 day')",
		id, uuid.MustParse(seedUploader), seedFilename,
		[]string{"Name", "Email", "Phone"},
		[]string{"0", "1", "2"},
		[]string{string(fieldContactName), string(fieldEmail), string(fieldPhone)},
		stateCommitted, len(rows),
		counts[outcomeImported], counts[outcomeSkipped], counts[outcomeFailed],
	); err != nil {
		return fmt.Errorf("importer: seed import: %w", err)
	}
	for i, row := range rows {
		if _, err := p.pool.Exec(ctx,
			"INSERT INTO plugin_importer.import_rows (id, import_id, position, cells, "+
				"outcome, reason, contact_id) VALUES ($1, $2, $3, to_jsonb($4::text[]), $5, $6, $7)",
			uuid.MustParse(row.id), id, i+1, row.cells,
			row.outcome, optionalReason(row.reason), links[i],
		); err != nil {
			return fmt.Errorf("importer: seed row %d: %w", i+1, err)
		}
	}
	return nil
}
