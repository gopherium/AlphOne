// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
)

// addEntryMutation adds one entry to a contact's repeater through the graph.
const addEntryMutation = `mutation($contactId: UUID!, $field: String!, $entry: JSON!) {
	addContactFieldEntry(contactId: $contactId, field: $field, entry: $entry)
}`

// updateEntryMutation replaces the cells of one entry through the graph.
const updateEntryMutation = `mutation($contactId: UUID!, $field: String!, $entryId: UUID!, $entry: JSON!) {
	updateContactFieldEntry(contactId: $contactId, field: $field, entryId: $entryId, entry: $entry)
}`

// deleteEntryMutation drops one entry through the graph.
const deleteEntryMutation = `mutation($contactId: UUID!, $field: String!, $entryId: UUID!) {
	deleteContactFieldEntry(contactId: $contactId, field: $field, entryId: $entryId)
}`

// entryAnswer is the envelope an entry mutation answers.
type entryAnswer struct {
	Data struct {
		Added   map[string]any `json:"addContactFieldEntry"`
		Updated map[string]any `json:"updateContactFieldEntry"`
	} `json:"data"`
}

// secretFor returns the token acting for the named tenant, or the owner's when no tenant is named.
func (w *world) secretFor(tenant string) (string, error) {
	if tenant == "" {
		return w.secret, nil
	}
	return w.secretOf(tenant)
}

// remember keeps an entry's id under every text cell it holds, so later steps name the entry by its text.
func (w *world) remember(entry map[string]any) {
	id, _ := entry["id"].(string)
	if w.entryIDs == nil {
		w.entryIDs = map[string]string{}
	}
	for key, value := range entry {
		if text, isText := value.(string); isText && key != "id" {
			w.entryIDs[text] = id
		}
	}
}

// entryIDOf returns the id the scenario remembers for the entry holding the given text.
func (w *world) entryIDOf(text string) (string, error) {
	id, held := w.entryIDs[text]
	if !held {
		return "", fmt.Errorf("the scenario added no entry holding %q", text)
	}
	return id, nil
}

// addEntry adds one entry to the seeded contact as the given secret, remembering it when the graph accepts it.
func (w *world) addEntry(ctx context.Context, secret, field string, entry any) (bool, error) {
	w.entryField = field
	if _, err := w.operationAs(ctx, secret, addEntryMutation,
		map[string]any{"contactId": w.lastContact.String(), "field": field, "entry": entry}); err != nil {
		return false, err
	}
	var answer entryAnswer
	if err := json.Unmarshal(w.answered, &answer); err != nil {
		return false, fmt.Errorf("decoding %s: %w", w.answered, err)
	}
	if answer.Data.Added == nil {
		return false, nil
	}
	w.remember(answer.Data.Added)
	id, _ := answer.Data.Added["id"].(string)
	w.answeredIDs = append(w.answeredIDs, id)
	return true, nil
}

// editEntry replaces the cells of the entry holding the given text, as the given secret.
func (w *world) editEntry(ctx context.Context, secret, text, field string, entry map[string]any) error {
	id, err := w.entryIDOf(text)
	if err != nil {
		return err
	}
	if _, err := w.operationAs(ctx, secret, updateEntryMutation, map[string]any{
		"contactId": w.lastContact.String(), "field": field, "entryId": id, "entry": entry,
	}); err != nil {
		return err
	}
	var answer entryAnswer
	if err := json.Unmarshal(w.answered, &answer); err != nil {
		return fmt.Errorf("decoding %s: %w", w.answered, err)
	}
	if answer.Data.Updated != nil {
		w.remember(answer.Data.Updated)
	}
	return nil
}

// removeEntry drops the entry of the given id from the seeded contact, as the given secret.
func (w *world) removeEntry(ctx context.Context, secret, id, field string) error {
	_, err := w.operationAs(ctx, secret, deleteEntryMutation,
		map[string]any{"contactId": w.lastContact.String(), "field": field, "entryId": id})
	return err
}

// answeredEntries reads a field of the seeded contact and splits its entries into their ids and their cells.
func (w *world) answeredEntries(ctx context.Context, field string) ([]string, []map[string]any, error) {
	answered, err := w.readField(ctx, field)
	if err != nil {
		return nil, nil, err
	}
	listed, _ := answered.Data.Contact[field].([]any)
	ids := make([]string, 0, len(listed))
	cells := make([]map[string]any, 0, len(listed))
	for _, held := range listed {
		entry, _ := held.(map[string]any)
		id, _ := entry["id"].(string)
		ids = append(ids, id)
		rest := map[string]any{}
		for key, value := range entry {
			if key != "id" {
				rest[key] = value
			}
		}
		cells = append(cells, rest)
	}
	return ids, cells, nil
}

// distinctIDs reports whether every id is a UUID and no two are the same.
func distinctIDs(ids []string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil || seen[id] {
			return fmt.Errorf("ids = %q, want a distinct id on every entry", ids)
		}
		seen[id] = true
	}
	return nil
}

// bindRepeaterEntrySteps binds the steps that add, edit, remove and read repeater entries one at a time.
func bindRepeaterEntrySteps(sc *godog.ScenarioContext) {
	bindEntryWriteSteps(sc)
	bindEntryReadSteps(sc)
}

// bindEntryWriteSteps binds the steps that add, edit and remove repeater entries.
func bindEntryWriteSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the operator (?:adds|added) the entries to "([^"]*)" of the contact, oldest first:$`,
		func(ctx context.Context, field string, table *godog.Table) error {
			w := worldFrom(ctx)
			for _, entry := range tableRecords(table) {
				accepted, err := w.addEntry(ctx, w.secret, field, entry)
				if err != nil {
					return err
				}
				if !accepted {
					return fmt.Errorf("the graph refused the entry %v, answered %s", entry, w.answered)
				}
			}
			return nil
		})

	sc.Step(`^(?:the operator|the tenant "([^"]*)") adds an entry to "([^"]*)" of the contact:$`,
		func(ctx context.Context, tenant, field string, table *godog.Table) error {
			w := worldFrom(ctx)
			secret, err := w.secretFor(tenant)
			if err != nil {
				return err
			}
			_, err = w.addEntry(ctx, secret, field, tableRecords(table)[0])
			return err
		})

	sc.Step(`^the operator adds the raw entry (.+) to "([^"]*)" of the contact$`,
		func(ctx context.Context, raw, field string) error {
			w := worldFrom(ctx)
			var entry any
			if err := json.Unmarshal([]byte(raw), &entry); err != nil {
				return fmt.Errorf("decoding the raw entry %s: %w", raw, err)
			}
			_, err := w.addEntry(ctx, w.secret, field, entry)
			return err
		})

	sc.Step(`^(?:the operator|the tenant "([^"]*)") edits the entry "([^"]*)" of "([^"]*)" to:$`,
		func(ctx context.Context, tenant, text, field string, table *godog.Table) error {
			w := worldFrom(ctx)
			secret, err := w.secretFor(tenant)
			if err != nil {
				return err
			}
			return w.editEntry(ctx, secret, text, field, tableRecords(table)[0])
		})

	sc.Step(`^the operator edits the entry "([^"]*)" of "([^"]*)" sending its own id with:$`,
		func(ctx context.Context, text, field string, table *godog.Table) error {
			w := worldFrom(ctx)
			id, err := w.entryIDOf(text)
			if err != nil {
				return err
			}
			entry := tableRecords(table)[0]
			entry["id"] = id
			return w.editEntry(ctx, w.secret, text, field, entry)
		})

	sc.Step(`^the operator edits the entry "([^"]*)" of "([^"]*)" to the raw entry (.+)$`,
		func(ctx context.Context, text, field, raw string) error {
			var entry map[string]any
			if err := json.Unmarshal([]byte(raw), &entry); err != nil {
				return fmt.Errorf("decoding the raw entry %s: %w", raw, err)
			}
			w := worldFrom(ctx)
			return w.editEntry(ctx, w.secret, text, field, entry)
		})

	sc.Step(`^(?:the operator|the tenant "([^"]*)") removes the entry "([^"]*)" from "([^"]*)"$`,
		func(ctx context.Context, tenant, text, field string) error {
			w := worldFrom(ctx)
			secret, err := w.secretFor(tenant)
			if err != nil {
				return err
			}
			id, err := w.entryIDOf(text)
			if err != nil {
				return err
			}
			return w.removeEntry(ctx, secret, id, field)
		})

	sc.When(`^the operator removes an unknown entry from "([^"]*)"$`, func(ctx context.Context, field string) error {
		w := worldFrom(ctx)
		return w.removeEntry(ctx, w.secret, uuid.Must(uuid.NewV7()).String(), field)
	})
}

// bindEntryReadSteps binds the steps that read the entries a repeater answers.
func bindEntryReadSteps(sc *godog.ScenarioContext) {
	sc.Then(`^the add answers the entry with an id$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		var answer entryAnswer
		if err := json.Unmarshal(w.answered, &answer); err != nil {
			return fmt.Errorf("decoding %s: %w", w.answered, err)
		}
		id, _ := answer.Data.Added["id"].(string)
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("the add answered no id, answered %s", w.answered)
		}
		return nil
	})

	sc.Then(`^the contact's "([^"]*)" answers the entries, newest first:$`,
		func(ctx context.Context, field string, table *godog.Table) error {
			w := worldFrom(ctx)
			ids, cells, err := w.answeredEntries(ctx, field)
			if err != nil {
				return err
			}
			same, err := sameJSON(cells, tableRecords(table))
			if err != nil {
				return err
			}
			if !same {
				return fmt.Errorf("%s = %v, answered %s", field, cells, w.answered)
			}
			return distinctIDs(ids)
		})

	sc.Then(`^every entry keeps the id its add answered$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		ids, _, err := w.answeredEntries(ctx, w.entryField)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return fmt.Errorf("%s answered no entries, answered %s", w.entryField, w.answered)
		}
		for _, id := range ids {
			if !slices.Contains(w.answeredIDs, id) {
				return fmt.Errorf("the entry id %q is not one an add answered, answered %s", id, w.answered)
			}
		}
		return nil
	})
}
