// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/webhook"
)

type listedHook struct {
	URL   string `json:"url"`
	Owner *struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"owner"`
}

type managedAnswer struct {
	Data struct {
		Webhooks      []listedHook `json:"webhooks"`
		DeleteWebhook *bool        `json:"deleteWebhook"`
	} `json:"data"`
}

// managedAnswered decodes the last answer as a webhook listing or deletion.
func (w *world) managedAnswered() (managedAnswer, error) {
	var parsed managedAnswer
	if err := json.Unmarshal(w.answered, &parsed); err != nil {
		return managedAnswer{}, fmt.Errorf("reading the answer %s: %w", w.answered, err)
	}
	return parsed, nil
}

// postGraphAs posts a graph request carrying the session of the admin or the member.
func (w *world) postGraphAs(ctx context.Context, tier, body string) error {
	if tier == role.Member.String() {
		return w.postGraphAsMember(ctx, body)
	}
	return w.postGraphAsSession(ctx, body)
}

// holderOf returns the account the admin or the member stands for.
func (w *world) holderOf(tier string) uuid.UUID {
	if tier == role.Member.String() {
		return w.memberID
	}
	return w.ownerID
}

// listedHooks lists the webhooks the session of the admin or the member sees.
func (w *world) listedHooks(ctx context.Context, tier string) ([]listedHook, error) {
	if err := w.postGraphAs(ctx, tier, `{"query":"{ webhooks { url owner { name email } } }"}`); err != nil {
		return nil, err
	}
	if err := w.answeredWithoutError(); err != nil {
		return nil, err
	}
	parsed, err := w.managedAnswered()
	if err != nil {
		return nil, err
	}
	return parsed.Data.Webhooks, nil
}

// registerWebhookManagementSteps binds the steps that create, list and delete webhooks as an admin or a member.
func registerWebhookManagementSteps(sc *godog.ScenarioContext, t *testing.T) {
	registerRoleSteps(sc, t)
	registerWebhookCreateSteps(sc)

	sc.Given(`^the (admin|member) owns a webhook to "([^"]*)"$`, func(ctx context.Context, tier, address string) error {
		w := worldFrom(ctx)
		sub, err := webhook.NewSubscription(w.holderOf(tier), address, []event.Name{event.TaskCreated})
		if err != nil {
			return err
		}
		if err := w.graph.Webhooks.CreateSubscription(ctx, sub); err != nil {
			return err
		}
		if w.ownedHooks == nil {
			w.ownedHooks = map[string]uuid.UUID{}
		}
		w.ownedHooks[tier] = sub.ID
		return nil
	})

	sc.When(`^the (admin|member)'s session lists the webhooks$`, func(ctx context.Context, tier string) error {
		return worldFrom(ctx).postGraphAs(ctx, tier, `{"query":"{ webhooks { url owner { name email } } }"}`)
	})

	sc.When(`^the (admin|member)'s session deletes the (admin|member)'s webhook$`,
		func(ctx context.Context, tier, owner string) error {
			w := worldFrom(ctx)
			return w.postGraphAs(ctx, tier, fmt.Sprintf(
				`{"query":"mutation { deleteWebhook(id: \"%s\") }"}`, w.ownedHooks[owner]))
		})

	sc.Then(`^the list shows "([^"]*)" owned by "([^"]*)" at "([^"]*)"$`,
		func(ctx context.Context, address, name, email string) error {
			w := worldFrom(ctx)
			if err := w.answeredWithoutError(); err != nil {
				return err
			}
			parsed, err := w.managedAnswered()
			if err != nil {
				return err
			}
			for _, hook := range parsed.Data.Webhooks {
				if hook.URL == address && hook.Owner != nil && hook.Owner.Name == name && hook.Owner.Email == email {
					return nil
				}
			}
			return fmt.Errorf("webhooks = %s, want %s owned by %s at %s", w.answered, address, name, email)
		})

	sc.Then(`^the list holds only "([^"]*)"$`, func(ctx context.Context, address string) error {
		w := worldFrom(ctx)
		if err := w.answeredWithoutError(); err != nil {
			return err
		}
		parsed, err := w.managedAnswered()
		if err != nil {
			return err
		}
		if len(parsed.Data.Webhooks) != 1 || parsed.Data.Webhooks[0].URL != address {
			return fmt.Errorf("webhooks = %s, want only %s", w.answered, address)
		}
		return nil
	})

	sc.Then(`^the deletion is answered$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.answeredWithoutError(); err != nil {
			return err
		}
		parsed, err := w.managedAnswered()
		if err != nil {
			return err
		}
		if parsed.Data.DeleteWebhook == nil || !*parsed.Data.DeleteWebhook {
			return fmt.Errorf("answer = %s, want the deletion confirmed", w.answered)
		}
		return nil
	})

	sc.Then(`^the deletion is refused as not found$`, func(ctx context.Context) error {
		return worldFrom(ctx).refusedForReason("NOT_FOUND", "webhook_not_found")
	})

	sc.Then(`^the (admin|member)'s session lists no webhook$`, func(ctx context.Context, tier string) error {
		listed, err := worldFrom(ctx).listedHooks(ctx, tier)
		if err != nil {
			return err
		}
		if len(listed) != 0 {
			return fmt.Errorf("webhooks = %+v, want none left", listed)
		}
		return nil
	})

	sc.Then(`^the (admin|member)'s session still lists "([^"]*)"$`, func(ctx context.Context, tier, address string) error {
		listed, err := worldFrom(ctx).listedHooks(ctx, tier)
		if err != nil {
			return err
		}
		for _, hook := range listed {
			if hook.URL == address {
				return nil
			}
		}
		return fmt.Errorf("webhooks = %+v, want %s still there", listed, address)
	})

	sc.Then(`^the refusal names the capability "([^"]*)"$`, func(ctx context.Context, capability string) error {
		w := worldFrom(ctx)
		parsed, err := w.scopeErrors()
		if err != nil {
			return err
		}
		if len(parsed.Errors) != 1 || parsed.Errors[0].Extensions["capability"] != capability {
			return fmt.Errorf("errors = %s, want one refusal naming the capability %s", w.answered, capability)
		}
		return nil
	})
}
