// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
)

// importHeader is the header row every import scenario uploads.
const importHeader = "Name,Email,Birth date"

// uploadOperation is the multipart operations payload declaring the upload.
const uploadOperation = `{"query":"mutation($file: Upload!) { importUpload(file: $file) { id state columns } }",` +
	`"variables":{"file":null}}`

// setMappingMutation assigns the uploaded columns to fields.
const setMappingMutation = `mutation($id: UUID!, $assignments: [ImportAssignmentInput!]!) {
	importSetMapping(id: $id, assignments: $assignments) { id state }
}`

// commitMutation turns the staged rows into contacts.
const commitMutation = `mutation($id: UUID!) {
	importCommit(id: $id) { imported skipped failed }
}`

// importJobQuery reads one import's state and rows back.
const importJobQuery = `query($id: UUID!) {
	importJob(id: $id) { state rows { outcome reason } }
}`

// registryQuery reads the mappable registry.
const registryQuery = `{ importFields { name label required } }`

// contactsFieldQuery lists contacts with one runtime defined field selected.
const contactsFieldQuery = `{ contacts(first: 50) { edges { node { name %s } } } }`

// importAnswer is the envelope every import step reads.
type importAnswer struct {
	Data struct {
		ImportUpload struct {
			ID      string   `json:"id"`
			State   string   `json:"state"`
			Columns []string `json:"columns"`
		} `json:"importUpload"`
		ImportCommit struct {
			Imported int `json:"imported"`
			Skipped  int `json:"skipped"`
			Failed   int `json:"failed"`
		} `json:"importCommit"`
		ImportJob *struct {
			State string `json:"state"`
			Rows  []struct {
				Outcome string  `json:"outcome"`
				Reason  *string `json:"reason"`
			} `json:"rows"`
		} `json:"importJob"`
		ImportFields []struct {
			Name     string `json:"name"`
			Label    string `json:"label"`
			Required bool   `json:"required"`
		} `json:"importFields"`
		Contacts *struct {
			Edges []struct {
				Node map[string]any `json:"node"`
			} `json:"edges"`
		} `json:"contacts"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// importOperation posts a graph operation and decodes the import envelope.
func (w *world) importOperation(ctx context.Context, document string, variables map[string]any) (importAnswer, error) {
	payload := map[string]any{"query": document}
	if variables != nil {
		payload["variables"] = variables
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return importAnswer{}, fmt.Errorf("encoding the operation: %w", err)
	}
	raw, err := w.postGraph(ctx, string(body))
	if err != nil {
		return importAnswer{}, err
	}
	w.answered = raw
	var answer importAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		return importAnswer{}, fmt.Errorf("decoding %s: %w", raw, err)
	}
	return answer, nil
}

// uploadSpreadsheet posts a one row CSV through the multipart upload protocol.
func (w *world) uploadSpreadsheet(ctx context.Context, row string) error {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("operations", uploadOperation); err != nil {
		return fmt.Errorf("writing the operations part: %w", err)
	}
	if err := form.WriteField("map", `{"0":["variables.file"]}`); err != nil {
		return fmt.Errorf("writing the map part: %w", err)
	}
	part, err := form.CreateFormFile("0", "spring-leads.csv")
	if err != nil {
		return fmt.Errorf("creating the file part: %w", err)
	}
	if _, err := part.Write([]byte(importHeader + "\n" + row + "\n")); err != nil {
		return fmt.Errorf("writing the file part: %w", err)
	}
	if err := form.Close(); err != nil {
		return fmt.Errorf("closing the form: %w", err)
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, w.server.URL+"/api/graphql", &body)
	if err != nil {
		return fmt.Errorf("building the upload request: %w", err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+w.secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("posting the upload: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("reading the upload answer: %w", err)
	}
	w.answered = raw
	var answer importAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		return fmt.Errorf("decoding %s: %w", raw, err)
	}
	if len(answer.Errors) > 0 {
		return fmt.Errorf("the upload was refused, answered %s", raw)
	}
	parsed, err := uuid.Parse(answer.Data.ImportUpload.ID)
	if err != nil {
		return fmt.Errorf("reading the import id from %s: %w", raw, err)
	}
	w.lastImport = parsed
	return nil
}

// readContactField finds one contact by name in a listing selecting the field.
func (w *world) readContactField(ctx context.Context, contactName, field string) (any, bool, error) {
	listing := fmt.Sprintf(contactsFieldQuery, field)
	answer, err := w.importOperation(ctx, listing, nil)
	if err != nil {
		return nil, false, err
	}
	if len(answer.Errors) > 0 {
		return nil, false, fmt.Errorf("the listing was refused, answered %s", w.answered)
	}
	if answer.Data.Contacts == nil {
		return nil, false, fmt.Errorf("the listing answered no contacts, answered %s", w.answered)
	}
	for _, edge := range answer.Data.Contacts.Edges {
		if edge.Node["name"] == contactName {
			return edge.Node[field], true, nil
		}
	}
	return nil, false, nil
}

// registerImportFieldsSteps binds the import mapping steps and the world lifecycle.
func registerImportFieldsSteps(sc *godog.ScenarioContext, t *testing.T) {
	registerFieldsCatalogSteps(sc, t)

	sc.Given(`^a contact named "([^"]*)" reachable at "([^"]*)"$`,
		func(ctx context.Context, name, email string) error {
			return worldFrom(ctx).seedReachableContact(ctx, name, email)
		})

	sc.Given(`^an uploaded spreadsheet holding the row "([^"]*)"$`,
		func(ctx context.Context, row string) error {
			return worldFrom(ctx).uploadSpreadsheet(ctx, row)
		})

	sc.Given(`^the columns are mapped onto name, email and the field "([^"]*)"$`,
		func(ctx context.Context, field string) error {
			w := worldFrom(ctx)
			answer, err := w.importOperation(ctx, setMappingMutation, map[string]any{
				"id": w.lastImport.String(),
				"assignments": []map[string]any{
					{"column": 0, "field": "name"},
					{"column": 1, "field": "email"},
					{"column": 2, "field": field},
				},
			})
			if err != nil {
				return err
			}
			if len(answer.Errors) > 0 {
				return fmt.Errorf("the mapping was refused, answered %s", w.answered)
			}
			return nil
		})

	sc.When(`^the import is committed$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		_, err := w.importOperation(ctx, commitMutation, map[string]any{"id": w.lastImport.String()})
		return err
	})

	sc.Then(`^the commit answers (\d+) (imported|skipped|failed)$`,
		func(ctx context.Context, want int, outcome string) error {
			w := worldFrom(ctx)
			var answer importAnswer
			if err := json.Unmarshal(w.answered, &answer); err != nil {
				return fmt.Errorf("decoding %s: %w", w.answered, err)
			}
			if len(answer.Errors) > 0 {
				return fmt.Errorf("the commit was refused, answered %s", w.answered)
			}
			counts := map[string]int{
				"imported": answer.Data.ImportCommit.Imported,
				"skipped":  answer.Data.ImportCommit.Skipped,
				"failed":   answer.Data.ImportCommit.Failed,
			}
			if counts[outcome] != want {
				return fmt.Errorf("%s = %d, want %d, answered %s", outcome, counts[outcome], want, w.answered)
			}
			return nil
		})

	sc.Then(`^the commit is refused naming "([^"]*)"$`, func(ctx context.Context, name string) error {
		w := worldFrom(ctx)
		if err := w.refusedFor("VALIDATION"); err != nil {
			return err
		}
		if !strings.Contains(string(w.answered), name) {
			return fmt.Errorf("the refusal does not name %q, answered %s", name, w.answered)
		}
		return nil
	})

	sc.Then(`^the import stays ready for a new mapping$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		answer, err := w.importOperation(ctx, importJobQuery, map[string]any{"id": w.lastImport.String()})
		if err != nil {
			return err
		}
		if answer.Data.ImportJob == nil {
			return fmt.Errorf("the import is gone, answered %s", w.answered)
		}
		if answer.Data.ImportJob.State != "ready" {
			return fmt.Errorf("state = %q, want ready", answer.Data.ImportJob.State)
		}
		return nil
	})

	sc.Then(`^a row settles failed naming "([^"]*)" and kind ([A-Z]+)$`,
		func(ctx context.Context, field, kind string) error {
			w := worldFrom(ctx)
			answer, err := w.importOperation(ctx, importJobQuery, map[string]any{"id": w.lastImport.String()})
			if err != nil {
				return err
			}
			if answer.Data.ImportJob == nil {
				return fmt.Errorf("the import is gone, answered %s", w.answered)
			}
			for _, row := range answer.Data.ImportJob.Rows {
				if row.Outcome != "failed" || row.Reason == nil {
					continue
				}
				if strings.Contains(*row.Reason, field) && strings.Contains(*row.Reason, kind) {
					return nil
				}
			}
			return fmt.Errorf("no failed row names %q and %s, answered %s", field, kind, w.answered)
		})

	sc.Then(`^the mapping registry lists "([^"]*)" labelled "([^"]*)" beside the core columns$`,
		func(ctx context.Context, name, label string) error {
			listed, err := worldFrom(ctx).readRegistry(ctx)
			if err != nil {
				return err
			}
			for _, core := range []string{"name", "email", "phone"} {
				if !listed[core] {
					return fmt.Errorf("the registry lost the core column %q", core)
				}
			}
			w := worldFrom(ctx)
			if !listed[name] {
				return fmt.Errorf("the registry does not list %q, answered %s", name, w.answered)
			}
			if !strings.Contains(string(w.answered), label) {
				return fmt.Errorf("the registry does not carry the label %q, answered %s", label, w.answered)
			}
			return nil
		})

	sc.Then(`^the mapping registry lists "([^"]*)" exactly once$`,
		func(ctx context.Context, name string) error {
			w := worldFrom(ctx)
			answer, err := w.importOperation(ctx, registryQuery, nil)
			if err != nil {
				return err
			}
			var seen int
			for _, field := range answer.Data.ImportFields {
				if field.Name == name {
					seen++
				}
			}
			if seen != 1 {
				return fmt.Errorf("the registry lists %q %d times, want once", name, seen)
			}
			return nil
		})

	sc.Then(`^the contact "([^"]*)" answers "([^"]*)" for the field "([^"]*)"$`,
		func(ctx context.Context, contactName, want, field string) error {
			got, found, err := worldFrom(ctx).readContactField(ctx, contactName, field)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("no contact named %q was listed", contactName)
			}
			if got != want {
				return fmt.Errorf("%s = %#v, want %q", field, got, want)
			}
			return nil
		})

	sc.Then(`^the contact "([^"]*)" answers null for the field "([^"]*)"$`,
		func(ctx context.Context, contactName, field string) error {
			got, found, err := worldFrom(ctx).readContactField(ctx, contactName, field)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("no contact named %q was listed", contactName)
			}
			if got != nil {
				return fmt.Errorf("%s = %#v, want null", field, got)
			}
			return nil
		})

	sc.Then(`^no contact named "([^"]*)" exists$`, func(ctx context.Context, contactName string) error {
		_, found, err := worldFrom(ctx).readContactField(ctx, contactName, "name")
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("a contact named %q exists", contactName)
		}
		return nil
	})
}

// readRegistry reads the mappable registry into a set of names.
func (w *world) readRegistry(ctx context.Context) (map[string]bool, error) {
	answer, err := w.importOperation(ctx, registryQuery, nil)
	if err != nil {
		return nil, err
	}
	if len(answer.Errors) > 0 {
		return nil, fmt.Errorf("the registry was refused, answered %s", w.answered)
	}
	listed := make(map[string]bool, len(answer.Data.ImportFields))
	for _, field := range answer.Data.ImportFields {
		listed[field.Name] = true
	}
	return listed, nil
}
