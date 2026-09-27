// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// errNotARepeater reports an entry written to a field that keeps no list of entries.
var errNotARepeater = errors.New("fields: the field is not a repeater")

// AddContactFieldEntry stores one entry on a contact's repeater and answers it under its new id.
func (m MutationResolvers) AddContactFieldEntry(
	ctx context.Context, contactID uuid.UUID, field string, entry any,
) (any, error) {
	cells, err := m.plugin.checkedEntry(ctx, field, entry, "")
	if err != nil {
		return nil, err
	}
	stored, err := m.plugin.store.addEntry(ctx, contactID, field, []map[string]any{cells}, m.plugin.entriesMax)
	if err != nil {
		return nil, m.plugin.storeRefusal(err)
	}
	return stored[0], nil
}

// UpdateContactFieldEntry replaces the cells of one entry on a contact's repeater and answers the entry.
func (m MutationResolvers) UpdateContactFieldEntry(
	ctx context.Context, contactID uuid.UUID, field string, entryID uuid.UUID, entry any,
) (any, error) {
	cells, err := m.plugin.checkedEntry(ctx, field, entry, entryID.String())
	if err != nil {
		return nil, err
	}
	if err := m.plugin.store.updateEntry(ctx, contactID, field, entryID, cells); err != nil {
		return nil, m.plugin.storeRefusal(err)
	}
	answered := maps.Clone(cells)
	answered[entryIDKey] = entryID.String()
	return answered, nil
}

// DeleteContactFieldEntry drops one entry from a contact's repeater.
func (m MutationResolvers) DeleteContactFieldEntry(
	ctx context.Context, contactID uuid.UUID, field string, entryID uuid.UUID,
) (bool, error) {
	if _, err := m.plugin.repeaterColumns(ctx, field); err != nil {
		return false, err
	}
	if err := m.plugin.store.deleteEntry(ctx, contactID, field, entryID); err != nil {
		return false, m.plugin.storeRefusal(err)
	}
	return true, nil
}

// checkedEntry returns the storable cells of one entry of a live repeater of the caller's.
func (p *Plugin) checkedEntry(ctx context.Context, field string, entry any, own string) (map[string]any, error) {
	columns, err := p.repeaterColumns(ctx, field)
	if err != nil {
		return nil, err
	}
	cells, err := checkEntry(field, columns, entry, own)
	if err != nil {
		return nil, sdk.GraphError{Code: "VALIDATION", Reason: fieldReason(err), Err: err}
	}
	return cells, nil
}

// repeaterColumns returns the sub fields of a live repeater the caller's tenant defines.
func (p *Plugin) repeaterColumns(ctx context.Context, field string) ([]SubField, error) {
	read, err := p.catalog.viewFor(ctx)
	if err != nil {
		return nil, err
	}
	held, defined := read.kinds[field]
	if !defined {
		err := fmt.Errorf("%w: %s", errNoField, field)
		return nil, sdk.GraphError{Code: "VALIDATION", Reason: fieldReason(err), Err: err}
	}
	if held != kindRepeater {
		err := fmt.Errorf("%w: %s", errNotARepeater, field)
		return nil, sdk.GraphError{Code: "VALIDATION", Reason: fieldReason(err), Err: err}
	}
	return read.columns[field], nil
}

// storeRefusal returns the graph refusal a store sentinel stands for, or the error as it is.
func (p *Plugin) storeRefusal(err error) error {
	switch {
	case errors.Is(err, errNoContact):
		return sdk.GraphError{Code: "NOT_FOUND", Reason: "contact_not_found", Err: err}
	case errors.Is(err, errNoEntry):
		return sdk.GraphError{Code: "NOT_FOUND", Reason: fieldReason(err), Err: err}
	case errors.Is(err, errEntriesFull):
		return sdk.GraphError{
			Code: "CONFLICT", Reason: fieldReason(err), Meta: map[string]any{"max": p.entriesMax}, Err: err,
		}
	}
	return err
}
