// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	alphonemcp "github.com/gopherium/alphone/internal/mcp"
	"github.com/gopherium/alphone/internal/task"
)

// seedIdentity attaches a channel address to the remembered contact.
func (w *world) seedIdentity(ctx context.Context, channel, identifier string) error {
	identity, err := contact.NewIdentity(w.lastContact, contact.Channel(channel), identifier, "")
	if err != nil {
		return fmt.Errorf("building the identity: %w", err)
	}
	if err := w.contacts.AddIdentity(ctx, identity); err != nil {
		return fmt.Errorf("storing the identity: %w", err)
	}
	return nil
}

// contactsAnswer decodes the contact list the last call answered with.
func (w *world) contactsAnswer() (alphonemcp.ContactsOutput, error) {
	var out alphonemcp.ContactsOutput
	raw, err := w.structuredAnswer()
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(raw, &out)
}

// contactAnswer decodes the single contact the last call answered with.
func (w *world) contactAnswer() (alphonemcp.ContactOutput, error) {
	var out alphonemcp.ContactOutput
	raw, err := w.structuredAnswer()
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(raw, &out)
}

// structuredAnswer returns the structured content of the last call.
func (w *world) structuredAnswer() ([]byte, error) {
	if w.called == nil {
		return nil, fmt.Errorf("no tool was called")
	}
	if w.called.IsError {
		return nil, fmt.Errorf("the tool failed: %s", contentText(w.called))
	}
	return json.Marshal(w.called.StructuredContent)
}

// findContact returns the listed contact carrying the given name.
func findContact(out alphonemcp.ContactsOutput, name string) (alphonemcp.ContactItem, bool) {
	for _, item := range out.Contacts {
		if item.Name == name {
			return item, true
		}
	}
	return alphonemcp.ContactItem{}, false
}

// registerContactSteps binds the contact reading steps.
func registerContactSteps(sc *godog.ScenarioContext, t *testing.T) {
	registerTaskSteps(sc, t)

	sc.Given(`^a contact "([^"]*)" holding an open task$`, func(ctx context.Context, name string) error {
		w := worldFrom(ctx)
		id, err := w.seedContact(ctx, name)
		if err != nil {
			return err
		}
		return w.seedTask(ctx, "Call "+name, today(), task.StatusOpen, id)
	})

	sc.Given(`^a contact "([^"]*)" holding no tasks$`, func(ctx context.Context, name string) error {
		_, err := worldFrom(ctx).seedContact(ctx, name)
		return err
	})

	sc.Given(`^a contact "([^"]*)" reachable on ([a-z]+) as "([^"]*)"$`,
		func(ctx context.Context, name, channel, identifier string) error {
			w := worldFrom(ctx)
			if _, err := w.seedContact(ctx, name); err != nil {
				return err
			}
			return w.seedIdentity(ctx, channel, identifier)
		})

	sc.When(`^the agent calls find_contacts with query "([^"]*)"$`,
		func(ctx context.Context, query string) error {
			return worldFrom(ctx).callTool(ctx, "find_contacts", map[string]any{"query": query})
		})

	sc.When(`^the agent calls get_contact for that contact$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.callTool(ctx, "get_contact", map[string]any{"contact_id": w.lastContact.String()})
	})

	sc.When(`^the agent calls get_contact with an id no contact holds$`, func(ctx context.Context) error {
		return worldFrom(ctx).callTool(ctx, "get_contact",
			map[string]any{"contact_id": uuid.Must(uuid.NewV7()).String()})
	})

	sc.Then(`^the answer lists "([^"]*)" marked holding open work$`,
		func(ctx context.Context, name string) error {
			return assertOpenWork(ctx, name, true)
		})

	sc.Then(`^the answer lists "([^"]*)" marked free$`, func(ctx context.Context, name string) error {
		return assertOpenWork(ctx, name, false)
	})

	sc.Then(`^the answer lists "([^"]*)"$`, func(ctx context.Context, name string) error {
		out, err := worldFrom(ctx).contactsAnswer()
		if err != nil {
			return err
		}
		if _, found := findContact(out, name); !found {
			return fmt.Errorf("the answer holds no contact named %q", name)
		}
		return nil
	})

	sc.Then(`^the answer names "([^"]*)"$`, func(ctx context.Context, name string) error {
		out, err := worldFrom(ctx).contactAnswer()
		if err != nil {
			return err
		}
		if out.Name != name {
			return fmt.Errorf("name = %q, want %q", out.Name, name)
		}
		return nil
	})

	sc.Then(`^the answer carries the ([a-z]+) identity "([^"]*)"$`,
		func(ctx context.Context, channel, identifier string) error {
			out, err := worldFrom(ctx).contactAnswer()
			if err != nil {
				return err
			}
			for _, held := range out.Identities {
				if held.Channel == channel && held.Identifier == identifier {
					return nil
				}
			}
			return fmt.Errorf("identities = %+v, want %s %s", out.Identities, channel, identifier)
		})

	sc.Then(`^the answer lists the open task "([^"]*)"$`, func(ctx context.Context, title string) error {
		out, err := worldFrom(ctx).contactAnswer()
		if err != nil {
			return err
		}
		for _, held := range out.OpenTasks {
			if held.Title == title {
				return nil
			}
		}
		return fmt.Errorf("open tasks = %+v, want one titled %q", out.OpenTasks, title)
	})

	sc.Then(`^the tool fails with a not found error$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if w.called == nil {
			return fmt.Errorf("no tool was called")
		}
		if !w.called.IsError {
			return fmt.Errorf("the tool succeeded, want it refused")
		}
		if !strings.Contains(contentText(w.called), "NOT_FOUND") {
			return fmt.Errorf("failure = %q, want the graph's not found error", contentText(w.called))
		}
		return nil
	})
}

// assertOpenWork checks whether a listed contact is marked as holding work.
func assertOpenWork(ctx context.Context, name string, want bool) error {
	out, err := worldFrom(ctx).contactsAnswer()
	if err != nil {
		return err
	}
	item, found := findContact(out, name)
	if !found {
		return fmt.Errorf("the answer holds no contact named %q", name)
	}
	if item.HasOpenTasks != want {
		return fmt.Errorf("%q has_open_tasks = %t, want %t", name, item.HasOpenTasks, want)
	}
	return nil
}
