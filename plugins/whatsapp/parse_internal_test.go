// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"fmt"
	"testing"
	"time"
)

func webhookBody(message string) []byte {
	return fmt.Appendf(nil, `{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"contacts": [{"wa_id": "184467235", "profile": {"name": "María Pérez"}}],
			"messages": [%s]
		}}]}]
	}`, message)
}

func envelope(id, kind, fields string) string {
	return fmt.Sprintf(`{"from": "184467235", "id": %q, "timestamp": "1751791000", "type": %q%s}`,
		id, kind, fields)
}

func TestParseMessagesClassifiesEveryType(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		message     string
		contentType string
		content     string
		media       *mediaDescriptor
	}{
		"text": {
			message:     envelope("wamid.t", "text", `, "text": {"body": "hola"}`),
			contentType: "text",
			content:     "hola",
		},
		"image with caption": {
			message: envelope("wamid.i", "image",
				`, "image": {"id": "MEDIA1", "mime_type": "image/jpeg", "sha256": "c2hh", "caption": "la factura"}`),
			contentType: "image",
			content:     "la factura",
			media:       &mediaDescriptor{mediaID: "MEDIA1", mimeType: "image/jpeg", sha256: "c2hh"},
		},
		"image without caption": {
			message: envelope("wamid.i2", "image",
				`, "image": {"id": "MEDIA2", "mime_type": "image/png", "sha256": "c2hh"}`),
			contentType: "image",
			content:     "",
			media:       &mediaDescriptor{mediaID: "MEDIA2", mimeType: "image/png", sha256: "c2hh"},
		},
		"voice note": {
			message: envelope("wamid.a", "audio",
				`, "audio": {"id": "MEDIA3", "mime_type": "audio/ogg; codecs=opus", "sha256": "c2hh", "voice": true}`),
			contentType: "audio",
			content:     "",
			media:       &mediaDescriptor{mediaID: "MEDIA3", mimeType: "audio/ogg; codecs=opus", sha256: "c2hh", voice: true},
		},
		"video with caption": {
			message: envelope("wamid.v", "video",
				`, "video": {"id": "MEDIA4", "mime_type": "video/mp4", "sha256": "c2hh", "caption": "mira"}`),
			contentType: "video",
			content:     "mira",
			media:       &mediaDescriptor{mediaID: "MEDIA4", mimeType: "video/mp4", sha256: "c2hh"},
		},
		"document": {
			message: envelope("wamid.d", "document",
				`, "document": {"id": "MEDIA5", "mime_type": "application/pdf", "sha256": "c2hh",`+
					` "filename": "receipt.pdf", "caption": "factura"}`),
			contentType: "document",
			content:     "factura",
			media: &mediaDescriptor{
				mediaID: "MEDIA5", mimeType: "application/pdf", sha256: "c2hh", filename: "receipt.pdf",
			},
		},
		"animated sticker": {
			message: envelope("wamid.s", "sticker",
				`, "sticker": {"id": "MEDIA6", "mime_type": "image/webp", "sha256": "c2hh", "animated": true}`),
			contentType: "sticker",
			content:     "",
			media:       &mediaDescriptor{mediaID: "MEDIA6", mimeType: "image/webp", sha256: "c2hh", animated: true},
		},
		"location with place": {
			message: envelope("wamid.l", "location",
				`, "location": {"latitude": 40.4168, "longitude": -3.7038, "name": "Museo del Prado",`+
					` "address": "C. de Ruiz de Alarcón 23"}`),
			contentType: "location",
			content:     "Museo del Prado C. de Ruiz de Alarcón 23",
		},
		"bare location pin": {
			message:     envelope("wamid.l2", "location", `, "location": {"latitude": 40.4168, "longitude": -3.7038}`),
			contentType: "location",
			content:     "40.4168, -3.7038",
		},
		"contact cards": {
			message: envelope("wamid.c", "contacts",
				`, "contacts": [{"name": {"formatted_name": "Ana García"}}, {"name": {"formatted_name": "Luis Ruiz"}}]`),
			contentType: "contacts",
			content:     "Ana García, Luis Ruiz",
		},
		"reaction": {
			message:     envelope("wamid.r", "reaction", `, "reaction": {"message_id": "wamid.t", "emoji": "👍"}`),
			contentType: "reaction",
			content:     "👍",
		},
		"reaction removal": {
			message:     envelope("wamid.r2", "reaction", `, "reaction": {"message_id": "wamid.t"}`),
			contentType: "reaction",
			content:     "",
		},
		"meta unsupported type": {
			message: envelope("wamid.u", "unsupported",
				`, "errors": [{"code": 131051, "title": "Message type unknown"}]`),
			contentType: "unsupported",
			content:     "",
		},
		"unknown future type": {
			message:     envelope("wamid.u2", "poll", ""),
			contentType: "unsupported",
			content:     "",
		},
		"media type without its asset": {
			message:     envelope("wamid.u3", "image", ""),
			contentType: "unsupported",
			content:     "",
		},
		"location type without its pin": {
			message:     envelope("wamid.u4", "location", ""),
			contentType: "unsupported",
			content:     "",
		},
		"reaction type without its body": {
			message:     envelope("wamid.u5", "reaction", ""),
			contentType: "unsupported",
			content:     "",
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			batch, err := parseWebhook(webhookBody(tc.message))
			if err != nil {
				t.Fatalf("parseWebhook() error = %v, want nil", err)
			}

			if got, want := len(batch.messages), 1; got != want {
				t.Fatalf("len(messages) = %d, want %d", got, want)
			}
			got := batch.messages[0]
			if got.contentType != tc.contentType {
				t.Errorf("contentType = %q, want %q", got.contentType, tc.contentType)
			}
			if got.content != tc.content {
				t.Errorf("content = %q, want %q", got.content, tc.content)
			}
			switch {
			case tc.media == nil && got.media != nil:
				t.Errorf("media = %+v, want nil", *got.media)
			case tc.media != nil && got.media == nil:
				t.Errorf("media = nil, want %+v", *tc.media)
			case tc.media != nil && *got.media != *tc.media:
				t.Errorf("media = %+v, want %+v", *got.media, *tc.media)
			}
		})
	}
}

func TestParseMessagesCarriesEnvelopeMetadata(t *testing.T) {
	t.Parallel()

	message := envelope("wamid.meta", "image",
		`, "image": {"id": "MEDIA1", "mime_type": "image/jpeg", "sha256": "c2hh"}`)

	batch, err := parseWebhook(webhookBody(message))
	if err != nil {
		t.Fatalf("parseWebhook() error = %v, want nil", err)
	}

	if got, want := len(batch.messages), 1; got != want {
		t.Fatalf("len(messages) = %d, want %d", got, want)
	}
	got := batch.messages[0]
	if got.externalID != "wamid.meta" {
		t.Errorf("externalID = %q, want %q", got.externalID, "wamid.meta")
	}
	if got.sender != "184467235" {
		t.Errorf("sender = %q, want %q", got.sender, "184467235")
	}
	if got.senderName != "María Pérez" {
		t.Errorf("senderName = %q, want %q", got.senderName, "María Pérez")
	}
	if want := time.Unix(1751791000, 0).UTC(); !got.sentAt.Equal(want) {
		t.Errorf("sentAt = %v, want %v", got.sentAt, want)
	}
	if string(got.raw) != message {
		t.Errorf("raw = %s, want the message payload retained verbatim", got.raw)
	}
}

func TestParseMessagesFallsBackOnBadTimestamps(t *testing.T) {
	t.Parallel()

	before := time.Now().UTC()
	message := `{"from": "184467235", "id": "wamid.bad", "timestamp": "not-a-number", "type": "text",` +
		` "text": {"body": "hola"}}`

	batch, err := parseWebhook(webhookBody(message))
	if err != nil {
		t.Fatalf("parseWebhook() error = %v, want nil", err)
	}

	after := time.Now().UTC()
	if got, want := len(batch.messages), 1; got != want {
		t.Fatalf("len(messages) = %d, want %d", got, want)
	}
	sentAt := batch.messages[0].sentAt
	if sentAt.Before(before) || sentAt.After(after) {
		t.Errorf("sentAt = %v, want a now fallback between %v and %v", sentAt, before, after)
	}
}

func TestParseMessagesSkipsUnattributableElements(t *testing.T) {
	t.Parallel()

	message := `42, {"type": "poll"}, ` + envelope("wamid.kept", "text", `, "text": {"body": "hola"}`)

	batch, err := parseWebhook(webhookBody(message))
	if err != nil {
		t.Fatalf("parseWebhook() error = %v, want nil", err)
	}

	if got, want := len(batch.messages), 1; got != want {
		t.Fatalf("len(messages) = %d, want %d", got, want)
	}
	if batch.messages[0].externalID != "wamid.kept" {
		t.Errorf("externalID = %q, want %q", batch.messages[0].externalID, "wamid.kept")
	}
}

func TestParseMessagesRejectsEnvelopeGarbage(t *testing.T) {
	t.Parallel()

	if _, err := parseWebhook([]byte(`{"entry":`)); err == nil {
		t.Fatal("parseWebhook() error = nil, want a decode failure")
	}
}

func statusEnvelope(statuses string) []byte {
	return fmt.Appendf(nil, `{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"statuses": [%s]
		}}]}]
	}`, statuses)
}

func TestParseWebhookStatuses(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status string
		want   []statusUpdate
	}{
		"delivered": {
			status: `{"id": "wamid.out.1", "status": "delivered", "timestamp": "1751791000",` +
				` "recipient_id": "184467235"}`,
			want: []statusUpdate{{wamid: "wamid.out.1", status: "delivered"}},
		},
		"failed with error": {
			status: `{"id": "wamid.out.2", "status": "failed", "errors": [{"code": 131047,` +
				` "title": "Re-engagement message"}]}`,
			want: []statusUpdate{{wamid: "wamid.out.2", status: "failed", detail: "131047 Re-engagement message"}},
		},
		"failed without errors": {
			status: `{"id": "wamid.out.3", "status": "failed"}`,
			want:   []statusUpdate{{wamid: "wamid.out.3", status: "failed"}},
		},
		"missing id skipped": {
			status: `{"status": "delivered"}`,
			want:   nil,
		},
		"missing status skipped": {
			status: `{"id": "wamid.out.4"}`,
			want:   nil,
		},
		"garbage element skipped": {
			status: `42`,
			want:   nil,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			batch, err := parseWebhook(statusEnvelope(tc.status))
			if err != nil {
				t.Fatalf("parseWebhook() error = %v, want nil", err)
			}

			if got, want := len(batch.statuses), len(tc.want); got != want {
				t.Fatalf("len(statuses) = %d, want %d", got, want)
			}
			for i, want := range tc.want {
				if batch.statuses[i] != want {
					t.Errorf("statuses[%d] = %+v, want %+v", i, batch.statuses[i], want)
				}
			}
		})
	}
}

func TestParseWebhookCarriesMessagesAndStatusesTogether(t *testing.T) {
	t.Parallel()

	body := fmt.Appendf(nil, `{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"contacts": [{"wa_id": "184467235", "profile": {"name": "María Pérez"}}],
			"messages": [%s],
			"statuses": [{"id": "wamid.out.9", "status": "read"}]
		}}]}]
	}`, envelope("wamid.in.9", "text", `, "text": {"body": "hola"}`))

	batch, err := parseWebhook(body)
	if err != nil {
		t.Fatalf("parseWebhook() error = %v, want nil", err)
	}

	if len(batch.messages) != 1 || batch.messages[0].externalID != "wamid.in.9" {
		t.Errorf("messages = %+v, want the one inbound text", batch.messages)
	}
	if len(batch.statuses) != 1 || batch.statuses[0] != (statusUpdate{wamid: "wamid.out.9", status: "read"}) {
		t.Errorf("statuses = %+v, want the one read update", batch.statuses)
	}
}
