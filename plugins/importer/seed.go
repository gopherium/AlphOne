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
	seedImportID  = "0198d000-0000-7000-8000-0000000000a0"
	seedUploader  = "0198d000-0000-7000-8000-0000000000a1"
	seedFilename  = "spring-leads.csv"
	seedClaimed   = "ada@example.com"
	seedFieldName = "birthDate"
	seedFieldHead = "Birth date"
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
			cells:   []string{"Maria Perez", "maria.perez@example.com", "184467235", "1990-04-17"},
			outcome: outcomeImported,
		},
		{
			id:      "0198d000-0000-7000-8000-0000000000b2",
			cells:   []string{"Grace Hopper", "grace.hopper@example.com", "184467236", "1906-12-09"},
			outcome: outcomeImported,
		},
		{
			id:      "0198d000-0000-7000-8000-0000000000b3",
			cells:   []string{"Alan Turing", "alan.turing@example.com", "184467237", "1912-06-23"},
			outcome: outcomeImported,
		},
		{
			id:      "0198d000-0000-7000-8000-0000000000b4",
			cells:   []string{"Ada Lovelace", seedClaimed, "", "1815-12-10"},
			outcome: outcomeSkipped,
			reason:  "the contact detail already belongs to a contact",
		},
		{
			id:      "0198d000-0000-7000-8000-0000000000b5",
			cells:   []string{"M. Perez", "maria.perez@example.com", "", ""},
			outcome: outcomeSkipped,
			reason:  "the contact detail already belongs to a contact",
		},
		{
			id:      "0198d000-0000-7000-8000-0000000000b6",
			cells:   []string{"", "", "184467238", ""},
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
	served, err := p.seedRegistry(ctx)
	if err != nil {
		return err
	}
	rows := demoRows()
	links, err := p.seedLinks(ctx, rows, served)
	if err != nil {
		return err
	}
	return p.seedImport(ctx, id, rows, links, served.holds(seedFieldName))
}

// seedRegistry returns the registry the demo import fills, refusing a lost demo field.
func (p *Plugin) seedRegistry(ctx context.Context) (registry, error) {
	if len(p.providers) == 0 {
		return registry{owner: map[fieldName]int{}}, nil
	}
	served, err := p.registry(ctx)
	if err != nil {
		return registry{}, err
	}
	if !served.holds(seedFieldName) {
		return registry{}, fmt.Errorf("importer: the demo import expects the %q field", seedFieldName)
	}
	return served, nil
}

// seedLinks returns the contact each demo row points at, in position order.
func (p *Plugin) seedLinks(
	ctx context.Context, rows []demoRow, served registry,
) ([]*uuid.UUID, error) {
	links := make([]*uuid.UUID, len(rows))
	for i, row := range rows {
		link, err := p.seedLink(ctx, row, served)
		if err != nil {
			return nil, err
		}
		links[i] = link
	}
	return links, nil
}

// seedLink returns the contact one demo row points at, empty when it has none.
func (p *Plugin) seedLink(ctx context.Context, row demoRow, served registry) (*uuid.UUID, error) {
	switch row.outcome {
	case outcomeImported:
		return p.seedContact(ctx, row, served)
	case outcomeSkipped:
		return p.seedClaim(ctx, row)
	}
	return nil, nil
}

// seedContact stores the contact one imported demo row stands for and its field value.
func (p *Plugin) seedContact(ctx context.Context, row demoRow, served registry) (*uuid.UUID, error) {
	created, _, err := p.contacts.CreateWithIdentities(ctx, row.cells[0], []sdk.Identity{
		{Channel: sdk.Channel(fieldEmail), Identifier: row.cells[1]},
		{Channel: sdk.Channel(fieldPhone), Identifier: row.cells[2]},
	})
	if err != nil {
		return nil, fmt.Errorf("importer: seed contact: %w", err)
	}
	if err := served.writeOne(ctx, created.ID, seedFieldName, row.cells[3]); err != nil {
		return nil, fmt.Errorf("importer: seed field value: %w", err)
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
	ctx context.Context, id uuid.UUID, rows []demoRow, links []*uuid.UUID, withField bool,
) error {
	counts := tally(rows)
	headers, columns, fields := seedColumns(withField)
	if _, err := p.pool.Exec(ctx,
		"INSERT INTO plugin_importer.imports (id, user_id, filename, columns, mapping, "+
			"state, row_count, imported_count, skipped_count, failed_count, created_at) "+
			"VALUES ($1, $2, $3, $4, jsonb_object($5, $6), $7, $8, $9, $10, $11, "+
			"now() - interval '1 day')",
		id, uuid.MustParse(seedUploader), seedFilename,
		headers, columns, fields,
		stateCommitted, len(rows),
		counts[outcomeImported], counts[outcomeSkipped], counts[outcomeFailed],
	); err != nil {
		return fmt.Errorf("importer: seed import: %w", err)
	}
	for i, row := range rows {
		if _, err := p.pool.Exec(ctx,
			"INSERT INTO plugin_importer.import_rows (id, import_id, position, cells, "+
				"outcome, reason, contact_id) VALUES ($1, $2, $3, to_jsonb($4::text[]), $5, $6, $7)",
			uuid.MustParse(row.id), id, i+1, row.cells[:len(headers)],
			row.outcome, optionalReason(row.reason), links[i],
		); err != nil {
			return fmt.Errorf("importer: seed row %d: %w", i+1, err)
		}
	}
	return nil
}

// seedColumns returns the demo header beside the mapping keys and fields it carries.
func seedColumns(withField bool) ([]string, []string, []string) {
	headers := []string{"Name", "Email", "Phone"}
	columns := []string{"0", "1", "2"}
	fields := []string{string(fieldContactName), string(fieldEmail), string(fieldPhone)}
	if withField {
		headers = append(headers, seedFieldHead)
		columns = append(columns, "3")
		fields = append(fields, seedFieldName)
	}
	return headers, columns, fields
}
