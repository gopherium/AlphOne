// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

func TestAddProviderRefKeepsTwoReferencesOfOneMail(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))

	for _, ref := range []providerRef{
		{mailboxID: box.id, ref: "file:inbox", messageID: id},
		{mailboxID: box.id, ref: "file:sent", messageID: id, sentCopy: true},
	} {
		if err := addProviderRef(t.Context(), tx, sdk.DefaultTenantID, ref); err != nil {
			t.Fatalf("addProviderRef(%s) error = %v, want nil", ref.ref, err)
		}
	}

	if refs := countRows(t, tx, "SELECT count(*) FROM plugin_email.provider_refs WHERE message_id = $1", id); refs != 2 {
		t.Errorf("the mail holds %d provider references, want 2", refs)
	}
	sentCopies := countRows(t, tx,
		"SELECT count(*) FROM plugin_email.provider_refs WHERE message_id = $1 AND sent_copy", id)
	if sentCopies != 1 {
		t.Errorf("the mail holds %d Sent copies, want 1", sentCopies)
	}
}

func TestAddProviderRefLeavesAReferenceTheMailboxHoldsAsItIs(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeEmptyThread(t, tx, sdk.DefaultTenantID, box)
	held := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, thread))
	other := mailRow(box, thread)
	other.dedupeKey = nil
	second := storeMail(t, tx, sdk.DefaultTenantID, other)
	if err := addProviderRef(t.Context(), tx, sdk.DefaultTenantID,
		providerRef{mailboxID: box.id, ref: "file:inbox", messageID: held}); err != nil {
		t.Fatalf("addProviderRef() error = %v, want nil", err)
	}

	err := addProviderRef(t.Context(), tx, sdk.DefaultTenantID,
		providerRef{mailboxID: box.id, ref: "file:inbox", messageID: second, sentCopy: true})

	if err != nil {
		t.Fatalf("addProviderRef() of a held reference error = %v, want nil", err)
	}
	got, err := readMessageIDByProviderRef(t.Context(), tx, sdk.DefaultTenantID, box.id, "file:inbox")
	if err != nil || got != held {
		t.Errorf("the reference leads to %v, %v, want the mail that held it first, %v", got, err, held)
	}
}

func TestReadMessageIDByProviderRefAnswersTheMailItNames(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))
	if err := addProviderRef(t.Context(), tx, sdk.DefaultTenantID,
		providerRef{mailboxID: box.id, ref: "file:inbox", messageID: id}); err != nil {
		t.Fatalf("addProviderRef() error = %v, want nil", err)
	}

	got, err := readMessageIDByProviderRef(t.Context(), tx, sdk.DefaultTenantID, box.id, "file:inbox")

	if err != nil || got != id {
		t.Errorf("readMessageIDByProviderRef() = %v, %v, want %v", got, err, id)
	}
}

func TestReadMessageIDByProviderRefReportsAReferenceNoMailOfTheWorkspaceHolds(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	second := storeWorkspace(t, pool)
	box := storeMailbox(t, pool, second)
	tx := beginTx(t, pool)
	id := storeMail(t, tx, second, mailRow(box, storeEmptyThread(t, tx, second, box)))
	if err := addProviderRef(t.Context(), tx, second,
		providerRef{mailboxID: box.id, ref: "file:inbox", messageID: id}); err != nil {
		t.Fatalf("addProviderRef() error = %v, want nil", err)
	}

	for _, unheld := range []struct {
		name   string
		tenant uuid.UUID
		ref    string
	}{
		{name: "another workspace's", tenant: sdk.DefaultTenantID, ref: "file:inbox"},
		{name: "an unknown", tenant: second, ref: "file:unknown"},
	} {
		_, err := readMessageIDByProviderRef(t.Context(), tx, unheld.tenant, box.id, unheld.ref)

		if !errors.Is(err, pgx.ErrNoRows) || !strings.HasPrefix(err.Error(), "email: read the mail of a provider ref: ") {
			t.Errorf("readMessageIDByProviderRef() of %s reference error = %v, want no rows named", unheld.name, err)
		}
	}
}

func TestReadMessageIDByDedupeKeyAnswersTheMailItNames(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	id := storeMail(t, tx, sdk.DefaultTenantID, row)

	got, err := readMessageIDByDedupeKey(t.Context(), tx, sdk.DefaultTenantID, box.id, row.dedupeKey)

	if err != nil || got != id {
		t.Errorf("readMessageIDByDedupeKey() = %v, %v, want %v", got, err, id)
	}
}

func TestReadMessageIDByDedupeKeyReportsAKeyNoMailHolds(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))

	_, err := readMessageIDByDedupeKey(t.Context(), tx, sdk.DefaultTenantID, box.id, bytes.Repeat([]byte{9}, 32))

	if !errors.Is(err, pgx.ErrNoRows) || !strings.HasPrefix(err.Error(), "email: read the mail of a dedupe key: ") {
		t.Errorf("readMessageIDByDedupeKey() of an unknown key error = %v, want no rows named", err)
	}
}

func TestLinkContactsCountsOnlyTheNewLinks(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))
	sender := storeContact(t, tx, sdk.DefaultTenantID, "Sender contact")
	copied := storeContact(t, tx, sdk.DefaultTenantID, "Copied contact")
	later := storeContact(t, tx, sdk.DefaultTenantID, "Contact named by a later copy")

	first, err := linkContacts(t.Context(), tx, sdk.DefaultTenantID, id, []uuid.UUID{sender, copied})
	if err != nil || first != 2 {
		t.Fatalf("linkContacts() of two contacts = %d, %v, want 2 new links", first, err)
	}
	second, err := linkContacts(t.Context(), tx, sdk.DefaultTenantID, id, []uuid.UUID{copied, later})

	if err != nil || second != 1 {
		t.Errorf("linkContacts() of one held and one new contact = %d, %v, want 1 new link", second, err)
	}
	if links := countRows(t, tx,
		"SELECT count(*) FROM plugin_email.message_contacts WHERE message_id = $1", id); links != 3 {
		t.Errorf("the mail holds %d links, want 3", links)
	}
}

func TestLinkContactsLeavesOutAContactOfAnotherWorkspace(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	box := storeMailbox(t, pool, sdk.DefaultTenantID)
	foreign := storeContact(t, pool, storeWorkspace(t, pool), "Contact of another workspace")
	tx := beginTx(t, pool)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))

	linked, err := linkContacts(t.Context(), tx, sdk.DefaultTenantID, id, []uuid.UUID{foreign})

	if err != nil || linked != 0 {
		t.Errorf("linkContacts() of another workspace's contact = %d, %v, want no link", linked, err)
	}
}

func TestInsertAttachmentsStoresEachFileWithOrWithoutItsBytes(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))
	files := []attachmentRow{
		{position: 0, filename: "quote.pdf", contentType: "application/pdf", size: 3000,
			sha256: bytes.Repeat([]byte{3}, 32), fileID: uuid.Must(uuid.NewV7()), state: fileStored},
		{position: 1, filename: "photo.png", contentType: "image/png", size: 9000, sha256: bytes.Repeat([]byte{4}, 32),
			inline: true, contentID: "a+b@example.com", state: fileTooLarge},
	}

	err := insertAttachments(t.Context(), tx, sdk.DefaultTenantID, id, files)

	if err != nil {
		t.Fatalf("insertAttachments() error = %v, want nil", err)
	}
	rows, _ := tx.Query(t.Context(), `SELECT position, filename, content_type, size, sha256, inline, content_id,
		file_id, state FROM plugin_email.attachments WHERE message_id = $1 ORDER BY position`, id)
	stored, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (attachmentRow, error) {
		var file attachmentRow
		var fileID *uuid.UUID
		err := row.Scan(&file.position, &file.filename, &file.contentType, &file.size, &file.sha256, &file.inline,
			&file.contentID, &fileID, &file.state)
		if fileID != nil {
			file.fileID = *fileID
		}
		return file, err
	})
	if err != nil {
		t.Fatalf("reading the attachments: %v", err)
	}
	if !reflect.DeepEqual(stored, files) {
		t.Errorf("the stored attachments = %+v, want %+v", stored, files)
	}
}

func TestInsertAttachmentsStoresNothingForAMailWithNoFile(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box)))

	if err := insertAttachments(t.Context(), tx, sdk.DefaultTenantID, id, nil); err != nil {
		t.Fatalf("insertAttachments() of no file error = %v, want nil", err)
	}
	if files := countRows(t, tx, "SELECT count(*) FROM plugin_email.attachments WHERE message_id = $1", id); files != 0 {
		t.Errorf("the mail holds %d attachments, want none", files)
	}
}

func TestInsertAttachmentsReportsAnIDItCannotMake(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	err := insertAttachments(t.Context(), nil, sdk.DefaultTenantID, uuid.Nil, []attachmentRow{{}})

	if !errors.Is(err, errEntropy) {
		t.Errorf("insertAttachments() error = %v, want %v", err, errEntropy)
	}
}

func TestAddParticipantsAddsOnlyTheAddressesTheMailLacks(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	id := storeMail(t, tx, sdk.DefaultTenantID, row)
	copied := participant{Role: "cc", Email: "ada@example.com"}

	_, err := addParticipants(t.Context(), tx, sdk.DefaultTenantID, id, []participant{row.participants[0], copied}, 500)

	if err != nil {
		t.Fatalf("addParticipants() error = %v, want nil", err)
	}
	want := []participant{row.participants[0], copied}
	if got := storedRow(t, tx, id).participants; !reflect.DeepEqual(got, want) {
		t.Errorf("the participants = %+v, want %+v", got, want)
	}
}

func TestAddParticipantsStopsAtTheRecipientCap(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	row := mailRow(box, storeEmptyThread(t, tx, sdk.DefaultTenantID, box))
	id := storeMail(t, tx, sdk.DefaultTenantID, row)
	copied := []participant{{Role: "cc", Email: "ada@example.com"}, {Role: "cc", Email: "sales@example.com"}}

	if _, err := addParticipants(t.Context(), tx, sdk.DefaultTenantID, id, copied, 2); err != nil {
		t.Fatalf("addParticipants() error = %v, want nil", err)
	}

	want := []participant{row.participants[0], copied[0]}
	if got := storedRow(t, tx, id).participants; !reflect.DeepEqual(got, want) {
		t.Errorf("the participants under a cap of 2 = %+v, want %+v", got, want)
	}
}

func TestTurnOutboundTurnsAnInboundMailOutbound(t *testing.T) {
	t.Parallel()

	tx, box := newMailbox(t)
	thread := storeEmptyThread(t, tx, sdk.DefaultTenantID, box)
	id := storeMail(t, tx, sdk.DefaultTenantID, mailRow(box, thread))
	other := mailRow(box, thread)
	other.dedupeKey = nil
	untouched := storeMail(t, tx, sdk.DefaultTenantID, other)

	if err := turnOutbound(t.Context(), tx, sdk.DefaultTenantID, id); err != nil {
		t.Fatalf("turnOutbound() error = %v, want nil", err)
	}

	if direction := storedRow(t, tx, id).direction; direction != outbound {
		t.Errorf("the turned mail is %s, want %s", direction, outbound)
	}
	if direction := storedRow(t, tx, untouched).direction; direction != inbound {
		t.Errorf("another mail is %s, want it left %s", direction, inbound)
	}
}

func TestMessageStatementsReportAnEndedTransaction(t *testing.T) {
	t.Parallel()

	tx := endedTx(t)
	ctx, id := t.Context(), uuid.Must(uuid.NewV7())
	for prefix, call := range map[string]func() error{
		"email: insert message: ": func() error {
			_, err := insertMessage(ctx, tx, sdk.DefaultTenantID, messageRow{})
			return err
		},
		"email: add provider ref: ": func() error {
			return addProviderRef(ctx, tx, sdk.DefaultTenantID, providerRef{})
		},
		"email: read the mail of a provider ref: ": func() error {
			_, err := readMessageIDByProviderRef(ctx, tx, sdk.DefaultTenantID, id, "file:inbox")
			return err
		},
		"email: read the mail of a dedupe key: ": func() error {
			_, err := readMessageIDByDedupeKey(ctx, tx, sdk.DefaultTenantID, id, []byte{1})
			return err
		},
		"email: link contacts: ": func() error {
			_, err := linkContacts(ctx, tx, sdk.DefaultTenantID, id, []uuid.UUID{id})
			return err
		},
		"email: insert attachments: ": func() error {
			return insertAttachments(ctx, tx, sdk.DefaultTenantID, id, nil)
		},
		"email: add participants: ": func() error {
			_, err := addParticipants(ctx, tx, sdk.DefaultTenantID, id, nil, 1)
			return err
		},
		"email: turn outbound: ": func() error {
			return turnOutbound(ctx, tx, sdk.DefaultTenantID, id)
		},
	} {
		err := call()

		if !errors.Is(err, pgx.ErrTxClosed) || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("a statement on an ended transaction error = %v, want %q naming %v", err, prefix, pgx.ErrTxClosed)
		}
	}
}
