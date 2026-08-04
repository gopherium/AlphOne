// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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
}

// settlement is the outcome one staged row settles into.
type settlement struct {
	outcome   string
	reason    string
	contactID *uuid.UUID
}

type commitResponse struct {
	ID       uuid.UUID `json:"id"`
	Imported int       `json:"imported"`
	Skipped  int       `json:"skipped"`
	Failed   int       `json:"failed"`
}

// handleCommit returns a handler turning the staged rows of an import into contacts.
func (p *Plugin) handleCommit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stored, ok := p.loadImport(w, r)
		if !ok {
			return
		}
		claimed, err := p.store.claimForCommit(r.Context(), stored.ID)
		if err != nil {
			respondClaimError(w, err)
			return
		}
		if err := p.commitRows(r.Context(), stored.ID, claimed); err != nil {
			respondError(w, http.StatusInternalServerError, "the import could not be committed")
			return
		}
		counts, err := p.store.finishCommit(r.Context(), stored.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "the import could not be finished")
			return
		}
		p.announceCompletion(r.Context(), stored.ID, counts)
		respondJSON(w, http.StatusOK, commitResponse{
			ID: stored.ID, Imported: counts.Imported, Skipped: counts.Skipped, Failed: counts.Failed,
		})
	}
}

// commitRows settles every row of the import still waiting to be committed.
func (p *Plugin) commitRows(ctx context.Context, importID uuid.UUID, assigned mapping) error {
	pending, err := p.store.pendingRows(ctx, importID)
	if err != nil {
		return err
	}
	for _, row := range pending {
		settled, err := p.settle(ctx, draftOf(row.Cells, assigned))
		if err != nil {
			return err
		}
		if err := p.store.settleRow(ctx, row.ID, settled); err != nil {
			return err
		}
	}
	return nil
}

// settle turns one drafted contact into the outcome its row records.
func (p *Plugin) settle(ctx context.Context, d draft) (settlement, error) {
	if !d.usable() {
		return settlement{outcome: outcomeFailed, reason: "the row carries no name or no contact detail"}, nil
	}
	owner, claimed, err := p.claimant(ctx, d.identities)
	if err != nil {
		return settlement{}, err
	}
	if claimed {
		return skipped(owner), nil
	}
	return p.create(ctx, d)
}

// usable reports whether the draft carries enough to become a contact.
func (d draft) usable() bool {
	return d.name != "" && len(d.identities) > 0
}

// create stores the drafted contact, reporting a lost race as a skip.
func (p *Plugin) create(ctx context.Context, d draft) (settlement, error) {
	created, wasCreated, err := p.contacts.CreateWithIdentities(ctx, d.name, d.identities)
	if err != nil {
		return settlement{}, err
	}
	if !wasCreated {
		return skipped(created), nil
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
		if field == fieldContactName {
			d.name = value
			continue
		}
		d.identities = append(d.identities, sdk.Identity{
			Channel: sdk.Channel(field), Identifier: value,
		})
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

// respondClaimError answers the status a refused commit maps to.
func respondClaimError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAlreadyCommitted):
		respondError(w, http.StatusConflict, errAlreadyCommitted.Error())
	case errors.Is(err, errNoMapping), errors.Is(err, pgx.ErrNoRows):
		respondError(w, http.StatusUnprocessableEntity, errNoMapping.Error())
	default:
		respondError(w, http.StatusInternalServerError, "the import could not be claimed")
	}
}
