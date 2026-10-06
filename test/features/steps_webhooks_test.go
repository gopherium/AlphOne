// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cucumber/godog"

	"github.com/gopherium/alphone/internal/webhook"
)

// webhookAnswer is the envelope the webhook steps read.
type webhookAnswer struct {
	Data struct {
		CreateWebhook *struct {
			Webhook struct {
				URL string `json:"url"`
			} `json:"webhook"`
			Secret string `json:"secret"`
		} `json:"createWebhook"`
		Webhooks []struct {
			ID string `json:"id"`
		} `json:"webhooks"`
	} `json:"data"`
}

// webhookAnswered reads the last graph answer as a webhook operation.
func (w *world) webhookAnswered() (webhookAnswer, error) {
	var parsed webhookAnswer
	if err := json.Unmarshal(w.answered, &parsed); err != nil {
		return webhookAnswer{}, fmt.Errorf("reading the answer %s: %w", w.answered, err)
	}
	return parsed, nil
}

// refusedForReason reports whether the last operation carried one error with code and reason.
func (w *world) refusedForReason(code, reason string) error {
	parsed, err := w.scopeErrors()
	if err != nil {
		return err
	}
	if len(parsed.Errors) != 1 {
		return fmt.Errorf("errors = %v, want exactly one error: %s", parsed.Errors, w.answered)
	}
	extensions := parsed.Errors[0].Extensions
	if extensions["code"] != code || extensions["reason"] != reason {
		return fmt.Errorf("extensions = %v, want code %s and reason %s", extensions, code, reason)
	}
	return nil
}

// registerWebhookSteps binds the token steps and the steps registering webhooks.
func registerWebhookSteps(sc *godog.ScenarioContext, t *testing.T) {
	registerTokenSteps(sc, t)

	sc.Given(`^the operator allows webhook deliveries to "([^"]*)"$`, func(ctx context.Context, raw string) error {
		allowed, err := webhook.ParseAllowList(raw)
		if err != nil {
			return err
		}
		worldFrom(ctx).graph.WebhookGuard = webhook.AddressGuard{Allowed: allowed}
		return nil
	})

	registerWebhookCreateSteps(sc)

	sc.Then(`^that token lists no webhook$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.postGraphScoped(ctx, `{"query":"{ webhooks { id } }"}`); err != nil {
			return err
		}
		listed, err := w.webhookAnswered()
		if err != nil {
			return err
		}
		if len(listed.Data.Webhooks) != 0 {
			return fmt.Errorf("webhooks = %v, want none stored", listed.Data.Webhooks)
		}
		return nil
	})

	sc.Then(`^the webhook is refused as an internal address$`, func(ctx context.Context) error {
		return worldFrom(ctx).refusedForReason("VALIDATION", "webhook_url_internal")
	})
}

// registerWebhookCreateSteps binds the steps registering a webhook with the scoped token and reading the answer.
func registerWebhookCreateSteps(sc *godog.ScenarioContext) {
	sc.When(`^that token registers a webhook to "([^"]*)"$`, func(ctx context.Context, address string) error {
		return worldFrom(ctx).postGraphScoped(ctx, fmt.Sprintf(
			`{"query":"mutation { createWebhook(url: \"%s\", events: [\"task.created\"])`+
				` { webhook { url } secret } }"}`, address))
	})

	sc.Then(`^the webhook is registered for "([^"]*)"$`, func(ctx context.Context, address string) error {
		w := worldFrom(ctx)
		if err := w.answeredWithoutError(); err != nil {
			return err
		}
		created, err := w.webhookAnswered()
		if err != nil {
			return err
		}
		if created.Data.CreateWebhook == nil || created.Data.CreateWebhook.Webhook.URL != address {
			return fmt.Errorf("answer = %s, want a webhook stored for %s exactly as written", w.answered, address)
		}
		if created.Data.CreateWebhook.Secret == "" {
			return fmt.Errorf("answer = %s, want the signing secret answered once", w.answered)
		}
		return nil
	})
}
