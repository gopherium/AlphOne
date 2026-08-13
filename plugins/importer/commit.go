// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// eventImportCompleted names the event a finished commit announces.
const eventImportCompleted = "import.completed"

// errNoMapping reports a commit asked for before the columns were assigned.
var errNoMapping = errors.New("the import carries no mapping yet")

// errAlreadyCommitted reports a commit asked for twice.
var errAlreadyCommitted = errors.New("the import is already committed")

// draft is the contact one staged row describes.
type draft struct {
	name       string
	identities []sdk.Identity
	texts      map[string]string
}

// settlement is the outcome one staged row settles into.
type settlement struct {
	outcome   string
	reason    string
	contactID *uuid.UUID
}

// commitRows settles every row of the import still waiting to be committed.
func (p *Plugin) commitRows(
	ctx context.Context, importID uuid.UUID, assigned mapping, known registry,
) error {
	pending, err := p.store.pendingRows(ctx, importID)
	if err != nil {
		return err
	}
	for _, row := range pending {
		if err := p.commitRow(ctx, row, assigned, known); err != nil {
			return err
		}
	}
	return nil
}

// commitRow settles one staged row, recording details the host refuses as a failure.
func (p *Plugin) commitRow(
	ctx context.Context, staged stagedRow, assigned mapping, known registry,
) error {
	settled, err := p.settle(ctx, draftOf(staged.Cells, assigned), known)
	if errors.Is(err, sdk.ErrInvalidContact) {
		settled, err = refused(), nil
	}
	if err != nil {
		return err
	}
	return p.store.settleRow(ctx, staged.ID, settled)
}

// refused returns the settlement of a row whose details the host would not store.
func refused() settlement {
	return settlement{
		outcome: outcomeFailed,
		reason:  "the row holds a name or a contact detail AlphOne cannot use",
	}
}

// settle turns one drafted contact into the outcome its row records.
func (p *Plugin) settle(ctx context.Context, d draft, known registry) (settlement, error) {
	if !d.usable() {
		return settlement{outcome: outcomeFailed, reason: "the row carries no name or no contact detail"}, nil
	}
	grouped, err := known.group(d.texts)
	if err != nil {
		return refusedText(err), nil
	}
	if err := known.check(ctx, grouped); err != nil {
		if errors.Is(err, sdk.ErrInvalidFieldText) {
			return refusedText(err), nil
		}
		return settlement{}, err
	}
	owner, claimed, err := p.claimant(ctx, d.identities)
	if err != nil {
		return settlement{}, err
	}
	if claimed {
		return skipped(owner), nil
	}
	return p.create(ctx, d, known, grouped)
}

// refusedText returns the settlement of a row carrying a value no field accepts.
func refusedText(err error) settlement {
	return settlement{
		outcome: outcomeFailed,
		reason:  strings.TrimPrefix(err.Error(), sdk.ErrInvalidFieldText.Error()+": "),
	}
}

// usable reports whether the draft carries enough to become a contact.
func (d draft) usable() bool {
	return d.name != "" && len(d.identities) > 0
}

// create stores the drafted contact and its field values, reporting a lost race as a skip.
func (p *Plugin) create(
	ctx context.Context, d draft, known registry, grouped shares,
) (settlement, error) {
	created, wasCreated, err := p.contacts.CreateWithIdentities(ctx, d.name, d.identities)
	if err != nil {
		return settlement{}, err
	}
	if !wasCreated {
		return skipped(created), nil
	}
	if err := known.write(ctx, created.ID, grouped); err != nil {
		return settlement{}, err
	}
	return settlement{outcome: outcomeImported, contactID: &created.ID}, nil
}

// claimant returns the contact already owning any of the identities.
func (p *Plugin) claimant(ctx context.Context, identities []sdk.Identity) (sdk.Contact, bool, error) {
	for _, identity := range identities {
		owner, found, err := p.contacts.FindByIdentity(ctx, identity.Channel, identity.Identifier)
		if err != nil {
			return sdk.Contact{}, false, err
		}
		if found {
			return owner, true, nil
		}
	}
	return sdk.Contact{}, false, nil
}

// skipped returns the settlement of a row whose contact already exists.
func skipped(owner sdk.Contact) settlement {
	id := owner.ID
	return settlement{
		outcome:   outcomeSkipped,
		reason:    "the contact detail already belongs to " + owner.Name,
		contactID: &id,
	}
}

// draftOf returns the contact the cells describe under the assigned mapping.
func draftOf(cells []string, assigned mapping) draft {
	var d draft
	for index, field := range assigned {
		value := cellAt(cells, index)
		if value == "" {
			continue
		}
		switch {
		case field == fieldContactName:
			d.name = value
		case core(field):
			d.identities = append(d.identities, sdk.Identity{
				Channel: sdk.Channel(field), Identifier: value,
			})
		default:
			if d.texts == nil {
				d.texts = map[string]string{}
			}
			d.texts[string(field)] = value
		}
	}
	return d
}

// cellAt returns the cell the decimal column index names, or an empty string.
func cellAt(cells []string, index string) string {
	position, err := strconv.Atoi(index)
	if err != nil || position < 0 || position >= len(cells) {
		return ""
	}
	return cells[position]
}

// announceCompletion publishes the event a finished import carries.
func (p *Plugin) announceCompletion(ctx context.Context, importID uuid.UUID, counts commitCounts) {
	if p.events == nil {
		return
	}
	p.events.Publish(ctx, eventImportCompleted, map[string]any{
		"id": importID.String(), "imported": counts.Imported, "skipped": counts.Skipped,
	})
}
