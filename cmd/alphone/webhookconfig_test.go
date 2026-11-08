// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/webhook"
)

func TestTheWebhookAllowListIsEmptyWhenUnset(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testkit.Getenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(held.webhookHosts, webhook.AllowList{}) {
		t.Errorf("webhookHosts = %+v, want nothing internal allowed", held.webhookHosts)
	}
}

func TestTheWebhookAllowListIsReadFromTheSetting(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testkit.Getenv(map[string]string{
		"ALPHONE_DATABASE_URL":          "postgres://localhost/x",
		"ALPHONE_WEBHOOK_ALLOWED_HOSTS": "127.0.0.1/32, n8n:5678",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	want := webhook.AllowList{Ranges: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}, Hosts: []string{"n8n:5678"}}
	if !reflect.DeepEqual(held.webhookHosts, want) {
		t.Errorf("webhookHosts = %+v, want %+v", held.webhookHosts, want)
	}
}

func TestAWebhookPayloadIsKeptThirtyDaysUnlessTheSettingNamesAnotherAge(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		set  string
		want time.Duration
	}{
		"unset":          {set: "", want: 720 * time.Hour},
		"set to 10 days": {set: "240h", want: 240 * time.Hour},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := map[string]string{"ALPHONE_DATABASE_URL": "postgres://localhost/x"}
			if tc.set != "" {
				env["ALPHONE_WEBHOOK_PAYLOAD_AGE"] = tc.set
			}

			held, err := loadRunConfig(testkit.Getenv(env))

			if err != nil {
				t.Fatalf("loadRunConfig() error = %v, want nil", err)
			}
			if held.webhookPayloadAge != tc.want {
				t.Errorf("webhookPayloadAge = %v, want %v", held.webhookPayloadAge, tc.want)
			}
		})
	}
}

func TestTheWebhookPayloadAgeIsTheOneTheExampleFileStates(t *testing.T) {
	t.Parallel()

	key := "ALPHONE_WEBHOOK_PAYLOAD_AGE"
	named := exampleSettings(t)

	if stated := statedValue(named, key); stated != webhook.DefaultPayloadAge.String() {
		t.Errorf("the example file states %s=%q, want the default %q", key, named[key], webhook.DefaultPayloadAge)
	}
}

// deliverySettledAgo stores a delivery of sub that was delivered ago before now.
func deliverySettledAgo(
	t *testing.T, store *postgres.WebhookStore, sub webhook.Subscription, ago time.Duration,
) webhook.Delivery {
	t.Helper()
	occurred, err := event.New(event.TaskCreated, map[string]any{"title": "Call Maria Perez"})
	if err != nil {
		t.Fatalf("event.New() error = %v, want nil", err)
	}
	delivery, err := webhook.NewDelivery(sub, occurred)
	if err != nil {
		t.Fatalf("webhook.NewDelivery() error = %v, want nil", err)
	}
	delivery.Status, delivery.DeliverAfter, delivery.Attempts = webhook.StatusDelivered, time.Now().UTC().Add(-ago), 1
	if err := store.EnqueueDelivery(t.Context(), delivery); err != nil {
		t.Fatalf("EnqueueDelivery() error = %v, want nil", err)
	}
	return delivery
}

func TestComposeHandsTheWorkerThePayloadAgeTheSettingNames(t *testing.T) {
	t.Parallel()

	env := map[string]string{"ALPHONE_DATABASE_URL": migratedPostgres(t), "ALPHONE_WEBHOOK_PAYLOAD_AGE": "1h"}
	built, err := compose(t.Context(), composeConfigOf(t, env), registeringNothing)
	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}
	abandonAtEnd(t, built)
	owner := uuid.Must(uuid.NewV7())
	sub, err := webhook.NewSubscription(owner, "https://example.com/hook", []event.Name{event.TaskCreated})
	if err != nil {
		t.Fatalf("webhook.NewSubscription() error = %v, want nil", err)
	}
	if err := built.webhooks.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}
	older := deliverySettledAgo(t, built.webhooks, sub, 2*time.Hour)
	newer := deliverySettledAgo(t, built.webhooks, sub, 30*time.Minute)

	built.worker.ClearPayloads(t.Context())

	payloads := map[uuid.UUID]string{}
	for _, id := range []uuid.UUID{older.ID, newer.ID} {
		var payload string
		if err := built.pool.QueryRow(t.Context(),
			"SELECT payload FROM core.webhook_deliveries WHERE id = $1", id).Scan(&payload); err != nil {
			t.Fatalf("reading the payload of %s: %v", id, err)
		}
		payloads[id] = payload
	}
	if payloads[older.ID] != "" {
		t.Errorf("the delivery settled two hours ago holds %s, want it cleared after the hour set", payloads[older.ID])
	}
	if payloads[newer.ID] != string(newer.Payload) {
		t.Errorf("the delivery settled half an hour ago holds %q, want %s kept", payloads[newer.ID], newer.Payload)
	}
}
