// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

// published is one event a recordingPublisher was handed.
type published struct {
	// tenant is the workspace the event's context named, uuid.Nil when it named none.
	tenant uuid.UUID
	// name is the event's name.
	name string
	// data is what the event carries.
	data map[string]any
}

// recordingPublisher records every event it is handed.
type recordingPublisher struct {
	// events lists the events in the order they came.
	events []published
}

// Publish records the event beside the workspace its context names.
func (r *recordingPublisher) Publish(ctx context.Context, name string, data map[string]any) {
	tenant, _ := sdk.TenantFromContext(ctx)
	r.events = append(r.events, published{tenant: tenant, name: name, data: data})
}

// personMail returns a new inbound mail from Maria Perez to the mailbox info@example.com, which an event announces.
func personMail() announcedMail {
	row := mailRow(mailbox{id: uuid.MustParse("01900000-0000-7000-8000-000000000001")},
		uuid.MustParse("01900000-0000-7000-8000-000000000002"))
	return announcedMail{
		id:       uuid.MustParse("01900000-0000-7000-8000-000000000003"),
		row:      row,
		contacts: []uuid.UUID{uuid.MustParse("01900000-0000-7000-8000-000000000004")},
		from:     message.Address{Name: "Maria Perez", Email: "maria.perez@example.com"},
		own:      []string{"info@example.com", "sales@example.com"},
	}
}

func TestEventDataCarriesTheIDsTheContactsTheStoredSubjectAndTheText(t *testing.T) {
	t.Parallel()

	data := personMail().eventData(16384)

	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("encoding the event data: %v", err)
	}
	want := `{"contact_ids":["01900000-0000-7000-8000-000000000004"],` +
		`"id":"01900000-0000-7000-8000-000000000003","subject":"Your quote",` +
		`"text":"Could you send me a quote?","thread_id":"01900000-0000-7000-8000-000000000002"}`
	if string(encoded) != want {
		t.Errorf("eventData() = %s, want %s", encoded, want)
	}
}

func TestEventDataCarriesTheSubjectTheRowKeeps(t *testing.T) {
	t.Parallel()

	mail := personMail()
	mail.row.subject = "Your quote for the brass lamp, cut at the subject cap"

	if subject := mail.eventData(16384)["subject"]; subject != mail.row.subject {
		t.Errorf("eventData() subject = %v, want the stored %q", subject, mail.row.subject)
	}
}

func TestEventDataListsAMailWithNoContactAsAnEmptyList(t *testing.T) {
	t.Parallel()

	mail := personMail()
	mail.contacts = nil

	encoded, err := json.Marshal(mail.eventData(16384)["contact_ids"])
	if err != nil || string(encoded) != "[]" {
		t.Errorf("eventData() contact_ids = %s, %v, want []", encoded, err)
	}
}

func TestEventDataCutsTheTextAtTheCapWhereALetterStarts(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		text  string
		limit int
		want  string
	}{
		"a plain text over the cap":         {"Could you send me a quote?", 16, "Could you send m"},
		"a text the cap meets exactly":      {"Could you send m", 16, "Could you send m"},
		"a text under the cap":              {"Could you", 16, "Could you"},
		"a two byte letter the cap splits":  {"ab\U000000e9cd", 3, "ab"},
		"a two byte letter the cap keeps":   {"ab\U000000e9cd", 4, "ab\U000000e9"},
		"a four byte letter the cap splits": {"\U0001F600 thanks", 3, ""},
		"an empty text":                     {"", 16, ""},
	} {
		mail := personMail()
		mail.row.bodyText = test.text

		if text := mail.eventData(test.limit)["text"]; text != test.want {
			t.Errorf("eventData(%d) text of %s = %q, want %q", test.limit, name, text, test.want)
		}
	}
}

func TestEventDataSendsAnEmptyTextForAnEncryptedMail(t *testing.T) {
	t.Parallel()

	mail := personMail()
	mail.row.encrypted = true

	if text := mail.eventData(16384)["text"]; text != "" {
		t.Errorf("eventData() text of an encrypted mail = %q, want empty", text)
	}
}

func TestAnnouncesOnlyANewMailFromAPerson(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		change func(*announcedMail)
		want   bool
	}{
		"a mail from a person":         {func(*announcedMail) {}, true},
		"a mail cut at the body cap":   {func(m *announcedMail) { m.row.bodyState = bodyCut }, true},
		"an encrypted mail":            {func(m *announcedMail) { m.row.encrypted = true }, true},
		"a mail with no From":          {func(m *announcedMail) { m.from = message.Address{} }, true},
		"an auto reply":                {func(m *announcedMail) { m.row.automation = message.AutoReply }, false},
		"a mailing list mail":          {func(m *announcedMail) { m.row.automation = message.ListMail }, false},
		"a bounce":                     {func(m *announcedMail) { m.row.automation = message.Bounce }, false},
		"a Sent copy":                  {func(m *announcedMail) { m.row.direction = outbound }, false},
		"a backfill":                   {func(m *announcedMail) { m.backfill = true }, false},
		"a mail from the own address":  {func(m *announcedMail) { m.from.Email = "info@example.com" }, false},
		"a mail from an alias":         {func(m *announcedMail) { m.from.Email = "sales@example.com" }, false},
		"a mail from the own capitals": {func(m *announcedMail) { m.from.Email = "INFO@EXAMPLE.COM" }, false},
		"a headers only mail":          {func(m *announcedMail) { m.row.bodyState = bodyHeadersOnly }, false},
	} {
		mail := personMail()
		test.change(&mail)

		if got := mail.announces(); got != test.want {
			t.Errorf("announces() of %s = %t, want %t", name, got, test.want)
		}
	}
}

func TestPublishArrivalAnnouncesTheMailInTheWorkspaceItServes(t *testing.T) {
	t.Parallel()

	publisher := &recordingPublisher{}
	acme := uuid.Must(uuid.NewV7())
	mail := personMail()

	publishArrival(t.Context(), publisher, acme, mail, 16)

	if len(publisher.events) != 1 {
		t.Fatalf("publishArrival() published %d events, want 1", len(publisher.events))
	}
	event := publisher.events[0]
	if event.name != eventMessageReceived || event.tenant != acme {
		t.Errorf("publishArrival() published %s in %v, want %s in %v", event.name, event.tenant, eventMessageReceived, acme)
	}
	if event.data["id"] != mail.id.String() || event.data["text"] != "Could you send m" {
		t.Errorf("publishArrival() data = %v, want the mail's own data with its text cut at 16 bytes", event.data)
	}
}

func TestPublishArrivalPublishesNothingForAQuietMail(t *testing.T) {
	t.Parallel()

	publisher := &recordingPublisher{}
	mail := personMail()
	mail.row.automation = message.AutoGenerated

	publishArrival(t.Context(), publisher, sdk.DefaultTenantID, mail, 16)

	if len(publisher.events) != 0 {
		t.Errorf("publishArrival() published %v for an automated mail, want nothing", publisher.events)
	}
}
