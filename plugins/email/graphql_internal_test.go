// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// graphNow is the time the clock of the plugin of a graph test stands at, one sync window after mailReceived.
var graphNow = mailReceived.Add(testSyncWindow)

// deleteLine is the message of the line each delete that took mail or files logs.
const deleteLine = "email: deleted stored mail"

// graphTest is one graph test: an ingest test beside a plugin over its database whose clock stands at graphNow.
type graphTest struct {
	*ingestTest
	// plugin serves the graph over the database of the test, logging to its buffer.
	plugin *Plugin
	// account is the account each request of the test acts as.
	account uuid.UUID
}

// newGraphTest returns a graph test whose plugin shares the pool, the ingester and the logger of its ingest test.
func newGraphTest(t *testing.T) *graphTest {
	t.Helper()
	it := newIngestTest(t)
	return &graphTest{ingestTest: it, account: uuid.Must(uuid.NewV7()), plugin: &Plugin{
		pool: it.pool, pages: sdk.PageBounds{Size: sdk.DefaultPageSize, Cap: sdk.DefaultPageCap}, ingester: it.ingester,
		loaders: mailLoaders{db: it.pool, bodiesPerRequest: 2, listsPerRequest: 2},
		now:     func() time.Time { return graphNow }, logger: it.ingester.logger, syncWindow: testSyncWindow,
	}}
}

// asAccount returns the context of one graph request of the account of the test in the default workspace.
func (gt *graphTest) asAccount() context.Context {
	return sdk.WithUser(inRequest(gt.t, sdk.DefaultTenantID), gt.account)
}

// deleteLines returns each line the deletes of the test logged, its time left out.
func (gt *graphTest) deleteLines() []map[string]any {
	gt.t.Helper()
	return slices.DeleteFunc(loggedLines(gt.t, gt.logged), func(line map[string]any) bool {
		return line[slog.MessageKey] != deleteLine
	})
}

func TestADeleteOfMailJudgesTheSyncWindowByThePluginClockAndKeepsItsTime(t *testing.T) {
	t.Parallel()

	gt := newGraphTest(t)
	gt.ingestAt(lampMail(), syncedRef, 0)
	gt.ingestAt(invoiceMail("%PDF-1.4 invoice"), "provider:invoice", -time.Second)
	windowAgo, earlier := gt.mailTitled("Your quote"), gt.mailTitled("Invoice")
	mutations := gt.plugin.MutationResolvers()

	_, rejected := mutations.EmailDeleteMessages(gt.asAccount(), gt.threadOf(windowAgo), &windowAgo)
	deletion, err := mutations.EmailDeleteMessages(gt.asAccount(), gt.threadOf(earlier), &earlier)

	var inside sdk.GraphError
	if !errors.As(rejected, &inside) || inside.Reason != "inside_sync_window" || inside.Meta["window"] != "720h0m0s" {
		t.Errorf("the delete of a synced mail received one window ago error = %v, want inside_sync_window of 720h0m0s",
			rejected)
	}
	if err != nil || deletion.DeletedMessages != 1 || !deletion.ThreadDeleted {
		t.Errorf("the delete of a synced mail received a second earlier = %+v, %v, want it deleted with its thread",
			deletion, err)
	}
	guards := guardRows(t, gt.pool, sdk.DefaultTenantID)
	if len(guards) != 1 || guards[0].messageID != earlier || !guards[0].deletedAt.Equal(graphNow) {
		t.Errorf("the keys of deleted mail = %+v, want the earlier mail's alone, deleted at %v", guards, graphNow)
	}
	if lines := gt.deleteLines(); len(lines) != 1 {
		t.Errorf("the two deletes logged %+v, want one line for the delete that took mail", lines)
	}
}

func TestEachDeleteLogsOneLineOfItsAccountWorkspaceActionAndIDsAndNoText(t *testing.T) {
	t.Parallel()

	gt := newGraphTest(t)
	thread, mails := gt.threadOfThree("file:reply")
	gt.ingestAt(invoiceMail("%PDF-1.4 invoice"), "file:invoice", 0)
	invoice := gt.mailTitled("Invoice")
	file := gt.attachmentIDs(invoice)[0]
	mutations := gt.plugin.MutationResolvers()
	gt.logged.Reset()

	_, oneErr := mutations.EmailDeleteMessages(gt.asAccount(), thread, &mails[2])
	_, restErr := mutations.EmailDeleteMessages(gt.asAccount(), thread, nil)
	_, filesErr := mutations.EmailDeleteFiles(gt.asAccount(), invoice, &file, true)

	if err := errors.Join(oneErr, restErr, filesErr); err != nil {
		t.Fatalf("the deletes error = %v, want nil", err)
	}
	workspace, account := sdk.DefaultTenantID.String(), gt.account.String()
	want := []map[string]any{
		{
			"level": "INFO", "msg": deleteLine, "account": account, "workspace": workspace, "action": "delete_message",
			"thread": thread.String(), "messages": []any{mails[2].String()},
		},
		{
			"level": "INFO", "msg": deleteLine, "account": account, "workspace": workspace, "action": "delete_thread",
			"thread": thread.String(), "messages": []any{mails[0].String(), mails[1].String()},
		},
		{
			"level": "INFO", "msg": deleteLine, "account": account, "workspace": workspace, "action": "delete_files",
			"message": invoice.String(), "attachment": file.String(), "original": true,
		},
	}
	if got := gt.deleteLines(); !reflect.DeepEqual(got, want) {
		t.Errorf("the deletes logged %+v, want %+v", got, want)
	}
	for _, text := range []string{"Your quote", "Invoice", "maria.perez@example.com", "info@example.com", "Thank you"} {
		if strings.Contains(gt.logged.String(), text) {
			t.Errorf("the log of the deletes holds %q, want no subject, address or text", text)
		}
	}
}

func TestARejectedDeleteAndAFilesDeleteThatTookNothingLogNoLine(t *testing.T) {
	t.Parallel()

	gt := newGraphTest(t)
	gt.ingestAt(invoiceMail("%PDF-1.4 invoice"), "file:invoice", 0)
	invoice := gt.mailTitled("Invoice")
	mutations := gt.plugin.MutationResolvers()
	if _, err := mutations.EmailDeleteFiles(gt.asAccount(), invoice, nil, true); err != nil {
		t.Fatalf("EmailDeleteFiles() of the original error = %v, want nil", err)
	}
	gt.logged.Reset()
	unknown := uuid.Must(uuid.NewV7())

	_, unknownThread := mutations.EmailDeleteMessages(gt.asAccount(), unknown, nil)
	_, unknownFile := mutations.EmailDeleteFiles(gt.asAccount(), invoice, &unknown, false)
	again, againErr := mutations.EmailDeleteFiles(gt.asAccount(), invoice, nil, true)

	wantRejected(t, unknownThread, "NOT_FOUND", "thread_not_found")
	wantRejected(t, unknownFile, "NOT_FOUND", "attachment_not_found")
	if againErr != nil || again.ID != invoice || again.RawState != fileDeleted || again.OriginalPath != nil {
		t.Errorf("a second delete of the original = %+v, %v, want the mail answered with its original deleted",
			again, againErr)
	}
	if lines := gt.deleteLines(); len(lines) != 0 {
		t.Errorf("the deletes that took nothing logged %+v, want no line", lines)
	}
}

func TestAnswerMailAnswersNotFoundForAMailNoLongerHeldAndTheFailureOfItsRead(t *testing.T) {
	t.Parallel()

	_, gone := answerMail(mailView{}, false, nil)
	_, failed := answerMail(mailView{}, false, errStoreDown)

	wantRejected(t, gone, "NOT_FOUND", "message_not_found")
	if !errors.Is(failed, errStoreDown) {
		t.Errorf("answerMail() of a read that failed error = %v, want %v", failed, errStoreDown)
	}
}

func TestToGraphUsageAnswersNoLimitForALimitOfZero(t *testing.T) {
	t.Parallel()

	none := toGraphUsage(workspaceUsage{usedBytes: 591, freeingBytes: 14})
	own := toGraphUsage(workspaceUsage{usedBytes: 591, limitBytes: 2000000})

	if none.UsedBytes != 591 || none.FreeingBytes != 14 || none.LimitBytes != nil {
		t.Errorf("toGraphUsage() of a workspace with no limit = %+v, want 591 used, 14 freeing and no limit", none)
	}
	if own.LimitBytes == nil || *own.LimitBytes != 2000000 {
		t.Errorf("toGraphUsage() of a workspace with its own limit = %+v, want the limit 2000000", own)
	}
}
