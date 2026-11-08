// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/webhook"
)

// webhookAnswer is the envelope the webhook steps read.
type webhookAnswer struct {
	Data struct {
		CreateWebhook *struct {
			Webhook struct {
				URL    string   `json:"url"`
				Events []string `json:"events"`
			} `json:"webhook"`
			Secret string `json:"secret"`
		} `json:"createWebhook"`
		Webhooks []struct {
			ID string `json:"id"`
		} `json:"webhooks"`
		WebhookEvents []string `json:"webhookEvents"`
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
	registerWebhookEventSteps(sc)
	registerWebhookPayloadSteps(sc)

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

// registerWebhookEventSteps binds the steps naming the events a webhook carries and listing every published event.
func registerWebhookEventSteps(sc *godog.ScenarioContext) {
	sc.When(`^that token registers a webhook for the event "([^"]*)"$`, func(ctx context.Context, name string) error {
		return worldFrom(ctx).postGraphScoped(ctx, fmt.Sprintf(
			`{"query":"mutation { createWebhook(url: \"https://example.com/hook\", events: [\"%s\"])`+
				` { webhook { url events } secret } }"}`, name))
	})

	sc.Then(`^the webhook is registered for the event "([^"]*)"$`, func(ctx context.Context, name string) error {
		return worldFrom(ctx).registeredForEvent(name)
	})

	sc.Then(`^the webhook is refused as an unknown event$`, func(ctx context.Context) error {
		return worldFrom(ctx).refusedForReason("VALIDATION", "event_unknown")
	})

	sc.When(`^that token lists the webhook events$`, func(ctx context.Context) error {
		return worldFrom(ctx).postGraphScoped(ctx, `{"query":"{ webhookEvents }"}`)
	})

	sc.Then(`^the event list names, in order$`, func(ctx context.Context, table *godog.Table) error {
		return worldFrom(ctx).listedInOrder(table)
	})
}

// registeredForEvent reports whether the last operation stored a webhook carrying only the event name.
func (w *world) registeredForEvent(name string) error {
	if err := w.answeredWithoutError(); err != nil {
		return err
	}
	created, err := w.webhookAnswered()
	if err != nil {
		return err
	}
	if created.Data.CreateWebhook == nil || !slices.Equal(created.Data.CreateWebhook.Webhook.Events, []string{name}) {
		return fmt.Errorf("answer = %s, want a webhook carrying %s", w.answered, name)
	}
	return nil
}

// listedInOrder reports whether the last operation listed exactly the event names of table, in its order.
func (w *world) listedInOrder(table *godog.Table) error {
	if err := w.answeredWithoutError(); err != nil {
		return err
	}
	listed, err := w.webhookAnswered()
	if err != nil {
		return err
	}
	want := make([]string, 0, len(table.Rows))
	for _, row := range table.Rows {
		want = append(want, row.Cells[0].Value)
	}
	if !slices.Equal(listed.Data.WebhookEvents, want) {
		return fmt.Errorf("webhookEvents = %v, want exactly %v in that order", listed.Data.WebhookEvents, want)
	}
	return nil
}

// storedPayloadQuery reads the payload the queue holds for one delivery.
const storedPayloadQuery = "SELECT payload FROM core.webhook_deliveries WHERE id = $1"

// storedStatusQuery reads the status the queue holds for one delivery.
const storedStatusQuery = "SELECT status FROM core.webhook_deliveries WHERE id = $1"

// registerWebhookPayloadSteps binds the steps that queue deliveries, run the delivery worker and read their payloads.
func registerWebhookPayloadSteps(sc *godog.ScenarioContext) {
	sc.Given(`^the user holds a webhook to "([^"]*)"$`, func(ctx context.Context, address string) error {
		w := worldFrom(ctx)
		sub, err := webhook.NewSubscription(w.ownerID, address, []event.Name{event.TaskCreated})
		if err != nil {
			return err
		}
		w.ownedHooks = map[string]uuid.UUID{"user": sub.ID}
		return w.graph.Webhooks.CreateSubscription(ctx, sub)
	})

	sc.Given(`^a delivery to that webhook (was sent|failed for good) (\d+) days ago$`,
		func(ctx context.Context, outcome string, days int) error {
			status := webhook.StatusDelivered
			if outcome == "failed for good" {
				status = webhook.StatusFailed
			}
			label := strings.TrimPrefix(outcome, "was ") + " " + strconv.Itoa(days)
			return worldFrom(ctx).queueDelivery(ctx, label, status, time.Now().UTC().AddDate(0, 0, -days))
		})

	sc.Given(`^a delivery to that webhook has waited (\d+) days to be sent$`, func(ctx context.Context, days int) error {
		label := "waited " + strconv.Itoa(days)
		return worldFrom(ctx).queueDelivery(ctx, label, webhook.StatusPending, time.Now().UTC().AddDate(0, 0, -days))
	})

	sc.Given(`^(\d+) deliveries to that webhook have waited (\d+) days to be sent$`,
		func(ctx context.Context, count, days int) error {
			return worldFrom(ctx).queueWaitingDeliveries(ctx, count, days)
		})

	sc.When(`^the delivery worker runs, keeping payloads for "([^"]*)"$`, func(ctx context.Context, raw string) error {
		age, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		w := worldFrom(ctx)
		queue := postgres.NewWebhookStore(w.pool)
		worker := webhook.NewWorker(queue, slog.New(slog.DiscardHandler), w.graph.WebhookGuard, age)
		worker.Sweep(ctx)
		worker.ClearPayloads(ctx)
		return nil
	})

	sc.Then(`^the delivery (?:that )?(sent|failed for good|waited) (\d+) days(?: ago)? `+
		`(holds no payload|still holds its payload)$`,
		func(ctx context.Context, kind string, days int, held string) error {
			return worldFrom(ctx).payloadHeld(ctx, kind+" "+strconv.Itoa(days), held == "still holds its payload")
		})

	sc.Then(`^the delivery that waited (\d+) days is still waiting to be sent$`,
		func(ctx context.Context, days int) error {
			return worldFrom(ctx).stillWaiting(ctx, "waited "+strconv.Itoa(days))
		})
}

// queueWaitingDeliveries stores count task event deliveries to the user's webhook, each waiting since days ago.
func (w *world) queueWaitingDeliveries(ctx context.Context, count, days int) error {
	since := time.Now().UTC().AddDate(0, 0, -days)
	for i := range count {
		label := fmt.Sprintf("waited %d, %d of %d", days, i+1, count)
		if err := w.queueDelivery(ctx, label, webhook.StatusPending, since); err != nil {
			return err
		}
	}
	return nil
}

// queueDelivery stores a task event delivery to the user's webhook under label, in status since at.
func (w *world) queueDelivery(ctx context.Context, label, status string, at time.Time) error {
	occurred, err := event.New(event.TaskCreated, map[string]any{"title": "Call Maria Perez"})
	if err != nil {
		return err
	}
	delivery, err := webhook.NewDelivery(webhook.Subscription{ID: w.ownedHooks["user"]}, occurred)
	if err != nil {
		return err
	}
	delivery.Status, delivery.DeliverAfter, delivery.CreatedAt, delivery.Attempts = status, at, at, 1
	if err := postgres.NewWebhookStore(w.pool).EnqueueDelivery(ctx, delivery); err != nil {
		return fmt.Errorf("queueing the delivery: %w", err)
	}
	if w.deliveries == nil {
		w.deliveries = map[string]webhook.Delivery{}
	}
	w.deliveries[label] = delivery
	return nil
}

// storedColumn reads through query the one column the queue holds for the delivery under label.
func (w *world) storedColumn(ctx context.Context, label, query string) (webhook.Delivery, string, error) {
	delivery, found := w.deliveries[label]
	if !found {
		return webhook.Delivery{}, "", fmt.Errorf("no delivery was queued as %q", label)
	}
	var value string
	if err := w.pool.QueryRow(ctx, query, delivery.ID).Scan(&value); err != nil {
		return webhook.Delivery{}, "", fmt.Errorf("reading the delivery %s: %w", label, err)
	}
	return delivery, value, nil
}

// stillWaiting reports whether the delivery under label is still waiting to be sent.
func (w *world) stillWaiting(ctx context.Context, label string) error {
	_, status, err := w.storedColumn(ctx, label, storedStatusQuery)
	if err != nil {
		return err
	}
	if status != webhook.StatusPending {
		return fmt.Errorf("the delivery %s is %q, want %q", label, status, webhook.StatusPending)
	}
	return nil
}

// payloadHeld reports whether the delivery under label still holds the payload it was queued with, or none.
func (w *world) payloadHeld(ctx context.Context, label string, kept bool) error {
	delivery, payload, err := w.storedColumn(ctx, label, storedPayloadQuery)
	if err != nil {
		return err
	}
	want := ""
	if kept {
		want = string(delivery.Payload)
	}
	if payload != want {
		return fmt.Errorf("the delivery %s holds %q, want %q", label, payload, want)
	}
	return nil
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
