// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// emailDeleteFeature is the feature deleting stored mail and reading the space a workspace uses answers to.
const emailDeleteFeature = "../../test/features/features/email-delete.feature"

// adminRequired is the error the graph answers a caller whose role lacks the capability a field needs.
const adminRequired = "admin required"

// workspaceDeactivated is the reason the graph answers a write from a deactivated workspace.
const workspaceDeactivated = "tenant_deactivated"

// fileStored is the state of a file of a mail that is still kept.
const fileStored = "stored"

// fileDeleted is the state of a file of a mail that was deleted.
const fileDeleted = "deleted"

// freesNothing is the word a usage step uses for a workspace with no file waiting for the sweep.
const freesNothing = "nothing"

// filesWaitingForTheSweep counts the files the queue of a workspace holds for the sweep to delete.
const filesWaitingForTheSweep = `SELECT count(*) FROM plugin_email.pending_files
	WHERE tenant_id = $1 AND state = 'deleting'`

// mailCostQuery reads one mail of a thread with its original and its files.
const mailCostQuery = `query Cost($thread: UUID!, $id: UUID!) {
	emailThread(id: $thread) { message(id: $id) { id subject rawState rawSize attachments { id filename size state } } }
}`

// threadCostQuery reads the original and the files of each mail of one thread.
const threadCostQuery = `query ThreadCost($id: UUID!) {
	emailThread(id: $id) { id messages { id rawState rawSize attachments { size state } } }
}`

// keptMailQuery reads what is kept of one mail of a thread, its text, its original and its files.
const keptMailQuery = `query Kept($thread: UUID!, $id: UUID!) {
	emailThread(id: $thread) {
		message(id: $id) {
			id subject rawState rawSize originalPath
			body { text }
			attachments { id filename contentType size state downloadPath }
		}
	}
}`

// threadTimesQuery reads one thread with its times and the time each of its mail was received.
const threadTimesQuery = `query Times($id: UUID!) {
	emailThread(id: $id) { id subject firstMessageAt lastMessageAt messages { id subject receivedAt } }
}`

// deleteMailQuery deletes one mail of a thread, or the whole thread when it names no mail.
const deleteMailQuery = `mutation DeleteMail($thread: UUID!, $id: UUID) {
	emailDeleteMessages(threadId: $thread, messageId: $id) { deletedMessages threadDeleted }
}`

// deleteFilesQuery deletes one attachment of a mail, its original, or both.
const deleteFilesQuery = `mutation DeleteFiles($id: UUID!, $file: UUID, $original: Boolean!) {
	emailDeleteFiles(messageId: $id, attachmentId: $file, original: $original) {
		id subject rawState originalPath attachments { id filename state downloadPath }
	}
}`

// deleteTwoFilesQuery deletes two attachments of one mail in one request, each answer under its own name.
const deleteTwoFilesQuery = `mutation DeleteTwo($id: UUID!, $first: UUID!, $second: UUID!) {
	first: emailDeleteFiles(messageId: $id, attachmentId: $first) { id attachments { filename state downloadPath } }
	second: emailDeleteFiles(messageId: $id, attachmentId: $second) { id attachments { filename state downloadPath } }
}`

// usageQuery reads the space the workspace uses, its limit, the space still being freed and its costliest mails.
const usageQuery = `query Usage($limit: Int) {
	emailUsage {
		usedBytes freeingBytes limitBytes
		largestMessages(limit: $limit) { id subject storedBytes rawState rawSize attachments { filename size state } }
	}
}`

// servedDeletion is what a delete of whole mail answers, as the graph answers it.
type servedDeletion struct {
	DeletedMessages int  `json:"deletedMessages"`
	ThreadDeleted   bool `json:"threadDeleted"`
}

// servedUsage is the space a workspace uses, as the graph answers it.
type servedUsage struct {
	UsedBytes       float64      `json:"usedBytes"`
	FreeingBytes    float64      `json:"freeingBytes"`
	LimitBytes      *float64     `json:"limitBytes"`
	LargestMessages []servedMail `json:"largestMessages"`
}

// deleteScenario is what one email delete scenario keeps between its steps, beside what the read steps keep.
type deleteScenario struct {
	*readScenario
	sharedMail string
	freed      float64
	sizes      map[string]float64
	rawSizes   map[string]float64
	usages     []servedUsage
}

// initializeEmailDelete returns the binding of the delete steps beside the read and email steps, t holding databases.
func initializeEmailDelete(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := newEmailScenario(t)
		bindEmailSteps(sc, s)
		r := newReadScenario(s)
		bindReadSteps(sc, r)
		d := &deleteScenario{readScenario: r, sizes: map[string]float64{}, rawSizes: map[string]float64{}}
		d.bindDeleteSetup(sc)
		d.bindDeletes(sc)
		d.bindDeleteAnswers(sc)
		d.bindKeptMail(sc)
		d.bindUsage(sc)
	}
}

// bindDeleteSetup binds the steps that seed the sample mailbox, hold admin tokens and feed a deleted mail again.
func (d *deleteScenario) bindDeleteSetup(sc *godog.ScenarioContext) {
	sc.Given(`^the sample mailbox is seeded in the default workspace$`, d.seedSampleMailbox)
	sc.Given(`^the next mail is received just now$`, d.receiveJustNow)
	sc.Given(`^the administrator "([^"]*)" holds a token scoped to "([^"]*)"$`, d.holdToken)
	sc.Given(`^the member "([^"]*)" shares the mail "([^"]*)"$`, d.shareMail)
	sc.When(`^the same mail is previewed for "([^"]*)"$`, d.previewAgain)
	sc.When(`^the same mail reaches "([^"]*)" as a Sent copy$`, d.reachAgainAsSentCopy)
	sc.Then(`^the workspace "([^"]*)" keeps no file but the ones waiting for the sweep$`, d.keepsOnlyFilesWaiting)
}

// bindDeletes binds the steps that delete a mail, a thread, an attachment or an original through the graph.
func (d *deleteScenario) bindDeletes(sc *godog.ScenarioContext) {
	sc.When(`^the (?:administrator|member) "([^"]*)" deletes the mail "([^"]*)"$`, d.deleteMail)
	sc.Given(`^the administrator "([^"]*)" deleted the mail "([^"]*)"$`, d.deletedMail)
	sc.When(`^the administrator "([^"]*)" deletes the thread "([^"]*)"$`, d.deleteThread)
	sc.When(`^the (?:administrator|member) "([^"]*)" deletes the file "([^"]*)" of the mail "([^"]*)"$`, d.deleteFile)
	sc.Given(`^the administrator "([^"]*)" deleted the file "([^"]*)" of the mail "([^"]*)"$`, d.deletedFile)
	sc.When(`^the administrator "([^"]*)" deletes the original of the mail "([^"]*)"$`, d.deleteOriginal)
	sc.Given(`^the administrator "([^"]*)" deleted the original of the mail "([^"]*)"$`, d.deletedOriginal)
	sc.When(`^the administrator "([^"]*)" deletes both "([^"]*)" and the original of the mail "([^"]*)"$`,
		d.deleteFileAndOriginal)
	sc.When(`^the administrator "([^"]*)" deletes the files of the mail "([^"]*)" naming none$`, d.deleteNoFile)
	sc.When(`^the administrator "([^"]*)" deletes both "([^"]*)" and "([^"]*)" of "([^"]*)" in one request$`,
		d.deleteTwoFiles)
	sc.When(`^the administrator "([^"]*)" deletes the shared mail$`, d.deleteSharedMail)
	sc.When(`^the administrator "([^"]*)" deletes the original of the shared mail$`, d.deleteSharedOriginal)
}

// bindDeleteAnswers binds the steps that check what a delete answered.
func (d *deleteScenario) bindDeleteAnswers(sc *godog.ScenarioContext) {
	sc.Then(`^the delete answers (\d+) mails? deleted and the thread (deleted|kept)$`, d.answersDeletion)
	sc.Then(`^the delete is rejected for want of the scope "([^"]*)"$`, d.rejectedForScope)
	sc.Then(`^the delete is rejected for want of the capability "([^"]*)"$`, d.rejectedForCapability)
	sc.Then(`^the delete is rejected as the workspace is deactivated$`, d.rejectedAsDeactivated)
	sc.Then(`^the delete is rejected with the reason "([^"]*)"$`, d.rejectedWithReason)
	sc.Then(`^the delete answers the mail with the file "([^"]*)" deleted$`, d.answersFileDeleted)
	sc.Then(`^the delete answers the mail with its original deleted$`, d.answersOriginalDeleted)
	sc.Then(`^the last answer of the request names the files "([^"]*)" as deleted$`, d.lastAnswerNamesDeleted)
}

// bindKeptMail binds the steps that read and check what is kept of a thread or a mail after a delete.
func (d *deleteScenario) bindKeptMail(sc *godog.ScenarioContext) {
	sc.Then(`^the inbox lists no thread$`, d.inboxListsNoThread)
	sc.When(`^the member "([^"]*)" opens the thread "([^"]*)" with its times$`, d.openThreadTimes)
	sc.Then(`^the thread starts and ends when its mail "([^"]*)" was received$`, d.threadSpansMail)
	sc.When(`^the member "([^"]*)" reads what is kept of the mail "([^"]*)"$`, d.readKeptMail)
	sc.Then(`^the mail names the file "([^"]*)" as "([^"]*)" of its old size, deleted with no download$`,
		d.mailNamesDeletedFile)
	sc.Then(`^the mail names its original of its old size as deleted with no download$`, d.mailNamesDeletedOriginal)
}

// bindUsage binds the steps that read and check the space a workspace uses.
func (d *deleteScenario) bindUsage(sc *godog.ScenarioContext) {
	sc.Step(`^the member "([^"]*)" reads the usage$`, d.readUsage)
	sc.When(`^the member "([^"]*)" reads the usage with its (\d+) costliest mails$`, d.readUsageCostliest)
	sc.Then(`^the usage counts the bytes of every stored mail and frees (nothing|each deleted file once)$`,
		d.usageCountsEveryMail)
	sc.Then(`^the usage holds as many bytes as before$`, d.usageUnchanged)
	sc.Then(`^the usage answers a limit of (\d+) bytes$`, d.usageLimitIs)
	sc.Then(`^the usage answers no limit$`, d.usageHasNoLimit)
	sc.Then(`^the usage lists the mails "([^"]*)" costliest first$`, d.usageListsCostliest)
	sc.Then(`^the usage lists the mail "([^"]*)" with the files "([^"]*)"$`, d.usageListsMailWithFiles)
}

// seedSampleMailbox runs alphone seed -yes, which stores the sample mailbox in the default workspace.
func (d *deleteScenario) seedSampleMailbox() error {
	d.result = d.answer("", "seed", "-yes")
	return d.wantExitZero()
}

// receiveJustNow sets the time the next mail is received at to now.
func (d *deleteScenario) receiveJustNow() {
	d.received = time.Now().UTC().Truncate(time.Second)
}

// shareMail keeps the thread and the id of the mail titled subject that email reads, for another account to name.
func (d *deleteScenario) shareMail(ctx context.Context, email, subject string) error {
	thread, id, err := d.findMail(ctx, email, subject)
	d.sharedThread, d.sharedMail = thread, id
	return err
}

// lastFed answers the last mail the scenario fed.
func (d *deleteScenario) lastFed() (string, error) {
	if len(d.fed) == 0 {
		return "", errors.New("no mail was fed before, want one to feed again")
	}
	return d.fed[len(d.fed)-1], nil
}

// previewAgain feeds the last mail fed once more to the workspace called name with no -yes.
func (d *deleteScenario) previewAgain(name string) error {
	raw, err := d.lastFed()
	if err != nil {
		return err
	}
	d.feed(name, d.operator, raw)
	return nil
}

// reachAgainAsSentCopy feeds the last mail fed once more to the workspace called name as a copy from the Sent folder.
func (d *deleteScenario) reachAgainAsSentCopy(name string) error {
	raw, err := d.lastFed()
	if err != nil {
		return err
	}
	d.feed(name, d.operator, raw, "-sent-copy", "-yes")
	return nil
}

// keepsOnlyFilesWaiting fails unless every file the workspace called name keeps waits in the queue for the sweep.
func (d *deleteScenario) keepsOnlyFilesWaiting(ctx context.Context, name string) error {
	kept, err := d.filesOf(ctx, name)
	if err != nil {
		return err
	}
	waiting, err := d.count(ctx, filesWaitingForTheSweep, d.workspaces[name])
	if err != nil {
		return err
	}
	if kept != waiting {
		return fmt.Errorf("the workspace %s keeps %d files and %d of them wait for the sweep, want all of them waiting",
			name, kept, waiting)
	}
	return nil
}

// keptOriginal answers the size of the original of mail when it is still kept, 0 otherwise.
func keptOriginal(mail servedMail) float64 {
	if mail.RawState != fileStored {
		return 0
	}
	return mail.RawSize
}

// keptSize answers the size of file when it is still kept, 0 otherwise.
func keptSize(file servedFile) float64 {
	if file.State != fileStored {
		return 0
	}
	return file.Size
}

// keptBytes answers the bytes of every file mail still keeps, its original and each attachment.
func keptBytes(mail servedMail) float64 {
	kept := keptOriginal(mail)
	for _, file := range mail.Attachments {
		kept += keptSize(file)
	}
	return kept
}

// credit adds bytes to what the deletes of the scenario free, when the last delete answered no error.
func (d *deleteScenario) credit(bytes float64) {
	if len(d.reply.Errors) == 0 {
		d.freed += bytes
	}
}

// costOf answers the thread and the mail titled subject as email reads them, with its original and its files.
func (d *deleteScenario) costOf(ctx context.Context, email, subject string) (string, servedMail, error) {
	thread, id, err := d.findMail(ctx, email, subject)
	if err != nil {
		return "", servedMail{}, err
	}
	var found *servedThread
	variables := map[string]any{"thread": thread, "id": id}
	if err := d.learn(ctx, email, mailCostQuery, variables, "emailThread", &found); err != nil {
		return "", servedMail{}, fmt.Errorf("reading the files of %q: %w", subject, err)
	}
	if found == nil || found.Message == nil {
		return "", servedMail{}, fmt.Errorf("the thread of %q answered no mail %s", subject, id)
	}
	return thread, *found.Message, nil
}

// wantDeleted fails with err, or unless the last delete answered no error.
func (d *deleteScenario) wantDeleted(err error) error {
	if err != nil {
		return err
	}
	if len(d.reply.Errors) > 0 {
		return fmt.Errorf("the delete answered %s, want no error", d.replied())
	}
	return nil
}

// deleteMail deletes the mail titled subject as email.
func (d *deleteScenario) deleteMail(ctx context.Context, email, subject string) error {
	thread, mail, err := d.costOf(ctx, email, subject)
	if err != nil {
		return err
	}
	if err := d.read(ctx, email, deleteMailQuery, map[string]any{"thread": thread, "id": mail.ID}); err != nil {
		return err
	}
	d.credit(keptBytes(mail))
	return nil
}

// deletedMail deletes the mail titled subject as email and fails unless the delete answered no error.
func (d *deleteScenario) deletedMail(ctx context.Context, email, subject string) error {
	return d.wantDeleted(d.deleteMail(ctx, email, subject))
}

// deleteThread deletes the whole thread titled subject as email.
func (d *deleteScenario) deleteThread(ctx context.Context, email, subject string) error {
	id, err := d.findThread(ctx, email, subject)
	if err != nil {
		return err
	}
	var found *servedThread
	if err := d.learn(ctx, email, threadCostQuery, map[string]any{"id": id}, "emailThread", &found); err != nil {
		return fmt.Errorf("reading the files of the thread %q: %w", subject, err)
	}
	if found == nil {
		return fmt.Errorf("the inbox answered no thread %s for %q", id, subject)
	}
	if err := d.read(ctx, email, deleteMailQuery, map[string]any{"thread": id}); err != nil {
		return err
	}
	for _, mail := range found.Messages {
		d.credit(keptBytes(mail))
	}
	return nil
}

// deleteFiles sends the files delete the variables name as email, adding freed to what the deletes free.
func (d *deleteScenario) deleteFiles(ctx context.Context, email string, variables map[string]any, freed float64) error {
	if err := d.read(ctx, email, deleteFilesQuery, variables); err != nil {
		return err
	}
	d.credit(freed)
	return nil
}

// fileOf answers the mail titled subject and its file called filename as email reads them, keeping the file's size.
func (d *deleteScenario) fileOf(ctx context.Context, email, filename, subject string) (servedMail, servedFile, error) {
	_, mail, err := d.costOf(ctx, email, subject)
	if err != nil {
		return servedMail{}, servedFile{}, err
	}
	file, err := fileCalled(mail.Attachments, filename)
	if err != nil {
		return servedMail{}, servedFile{}, fmt.Errorf("the mail %q %w", subject, err)
	}
	d.sizes[filename] = file.Size
	return mail, file, nil
}

// deleteFile deletes the file called filename of the mail titled subject as email.
func (d *deleteScenario) deleteFile(ctx context.Context, email, filename, subject string) error {
	mail, file, err := d.fileOf(ctx, email, filename, subject)
	if err != nil {
		return err
	}
	return d.deleteFiles(ctx, email, map[string]any{"id": mail.ID, "file": file.ID, "original": false}, keptSize(file))
}

// deletedFile deletes the file called filename of the mail titled subject as email, failing on any error it answers.
func (d *deleteScenario) deletedFile(ctx context.Context, email, filename, subject string) error {
	return d.wantDeleted(d.deleteFile(ctx, email, filename, subject))
}

// originalOf answers the mail titled subject as email reads it, keeping the size of its original.
func (d *deleteScenario) originalOf(ctx context.Context, email, subject string) (servedMail, error) {
	_, mail, err := d.costOf(ctx, email, subject)
	if err != nil {
		return servedMail{}, err
	}
	d.rawSizes[subject] = mail.RawSize
	return mail, nil
}

// deleteOriginal deletes the original of the mail titled subject as email.
func (d *deleteScenario) deleteOriginal(ctx context.Context, email, subject string) error {
	mail, err := d.originalOf(ctx, email, subject)
	if err != nil {
		return err
	}
	return d.deleteFiles(ctx, email, map[string]any{"id": mail.ID, "original": true}, keptOriginal(mail))
}

// deletedOriginal deletes the original of the mail titled subject as email and fails unless it answered no error.
func (d *deleteScenario) deletedOriginal(ctx context.Context, email, subject string) error {
	return d.wantDeleted(d.deleteOriginal(ctx, email, subject))
}

// deleteFileAndOriginal deletes the file called filename and the original of the mail titled subject in one delete.
func (d *deleteScenario) deleteFileAndOriginal(ctx context.Context, email, filename, subject string) error {
	mail, file, err := d.fileOf(ctx, email, filename, subject)
	if err != nil {
		return err
	}
	d.rawSizes[subject] = mail.RawSize
	variables := map[string]any{"id": mail.ID, "file": file.ID, "original": true}
	return d.deleteFiles(ctx, email, variables, keptSize(file)+keptOriginal(mail))
}

// deleteNoFile sends a files delete of the mail titled subject that names neither an attachment nor its original.
func (d *deleteScenario) deleteNoFile(ctx context.Context, email, subject string) error {
	_, mail, err := d.costOf(ctx, email, subject)
	if err != nil {
		return err
	}
	return d.deleteFiles(ctx, email, map[string]any{"id": mail.ID, "original": false}, 0)
}

// deleteTwoFiles deletes the files called first and second of the mail titled subject in one request as email.
func (d *deleteScenario) deleteTwoFiles(ctx context.Context, email, first, second, subject string) error {
	_, mail, err := d.costOf(ctx, email, subject)
	if err != nil {
		return err
	}
	one, err := fileCalled(mail.Attachments, first)
	if err != nil {
		return fmt.Errorf("the mail %q %w", subject, err)
	}
	other, err := fileCalled(mail.Attachments, second)
	if err != nil {
		return fmt.Errorf("the mail %q %w", subject, err)
	}
	variables := map[string]any{"id": mail.ID, "first": one.ID, "second": other.ID}
	if err := d.read(ctx, email, deleteTwoFilesQuery, variables); err != nil {
		return err
	}
	d.credit(keptSize(one) + keptSize(other))
	return nil
}

// deleteSharedMail deletes the shared mail as email, naming the thread and the mail another account read.
func (d *deleteScenario) deleteSharedMail(ctx context.Context, email string) error {
	return d.read(ctx, email, deleteMailQuery, map[string]any{"thread": d.sharedThread, "id": d.sharedMail})
}

// deleteSharedOriginal deletes the original of the shared mail as email.
func (d *deleteScenario) deleteSharedOriginal(ctx context.Context, email string) error {
	return d.read(ctx, email, deleteFilesQuery, map[string]any{"id": d.sharedMail, "original": true})
}

// answersDeletion fails unless the last delete deleted count mails and deleted or kept the thread as outcome says.
func (d *deleteScenario) answersDeletion(count int, outcome string) error {
	var deletion servedDeletion
	if err := d.reply.decode("emailDeleteMessages", &deletion); err != nil {
		return err
	}
	if deletion.DeletedMessages != count || deletion.ThreadDeleted != (outcome == "deleted") {
		return fmt.Errorf("the delete answered %+v, want %d mails deleted and the thread %s", deletion, count, outcome)
	}
	return nil
}

// rejectedForCapability fails unless the last delete was rejected for want of the capability.
func (d *deleteScenario) rejectedForCapability(capability string) error {
	if !d.reply.rejects(func(fault graphFault) bool {
		return fault.Message == adminRequired && fault.Extensions["capability"] == capability
	}) {
		return fmt.Errorf("the delete answered %s, want it rejected for want of %s", d.replied(), capability)
	}
	return nil
}

// rejectedAsDeactivated fails unless the last delete was rejected as its workspace is deactivated.
func (d *deleteScenario) rejectedAsDeactivated() error {
	return d.rejectedWithReason(workspaceDeactivated)
}

// filesAnswer decodes the mail the last files delete answered under the root field called name.
func (d *deleteScenario) filesAnswer(name string) (servedMail, error) {
	var mail servedMail
	err := d.reply.decode(name, &mail)
	return mail, err
}

// namesDeletedFiles fails unless mail names every file of the comma list filenames as deleted with no download.
func namesDeletedFiles(mail servedMail, filenames string) error {
	for filename := range strings.SplitSeq(filenames, ", ") {
		file, err := fileCalled(mail.Attachments, filename)
		if err != nil {
			return fmt.Errorf("the mail %s %w", mail.ID, err)
		}
		if file.State != fileDeleted || file.DownloadPath != nil {
			return fmt.Errorf("the mail %s names the file %s as %q at %s, want it deleted with no download",
				mail.ID, filename, file.State, shownPath(file.DownloadPath))
		}
	}
	return nil
}

// answersFileDeleted fails unless the last files delete answered its mail naming the file called filename as deleted.
func (d *deleteScenario) answersFileDeleted(filename string) error {
	mail, err := d.filesAnswer("emailDeleteFiles")
	if err != nil {
		return err
	}
	return namesDeletedFiles(mail, filename)
}

// answersOriginalDeleted fails unless the last files delete answered its mail naming its original as deleted.
func (d *deleteScenario) answersOriginalDeleted() error {
	mail, err := d.filesAnswer("emailDeleteFiles")
	if err != nil {
		return err
	}
	if mail.RawState != fileDeleted || mail.OriginalPath != nil {
		return fmt.Errorf("the mail %s keeps its original as %q at %s, want it deleted with no download",
			mail.ID, mail.RawState, shownPath(mail.OriginalPath))
	}
	return nil
}

// lastAnswerNamesDeleted fails unless the second delete of the last request names every file of filenames as deleted.
func (d *deleteScenario) lastAnswerNamesDeleted(filenames string) error {
	mail, err := d.filesAnswer("second")
	if err != nil {
		return err
	}
	return namesDeletedFiles(mail, filenames)
}

// inboxListsNoThread fails unless the last inbox read lists no thread and counts none.
func (d *deleteScenario) inboxListsNoThread() error {
	page, err := d.inboxPage()
	if err != nil {
		return err
	}
	if len(page.Items) != 0 || page.Total != 0 {
		return fmt.Errorf("the inbox lists the threads %q of a total of %d, want none",
			listedThreads(page.Items), page.Total)
	}
	return nil
}

// openThreadTimes reads the thread titled subject in the inbox of email with its times and those of its mail.
func (d *deleteScenario) openThreadTimes(ctx context.Context, email, subject string) error {
	id, err := d.findThread(ctx, email, subject)
	if err != nil {
		return err
	}
	return d.read(ctx, email, threadTimesQuery, map[string]any{"id": id})
}

// sameInstant reports whether the times the graph answered as first and second are one instant.
func sameInstant(first, second string) bool {
	one, err := time.Parse(time.RFC3339Nano, first)
	if err != nil {
		return false
	}
	other, err := time.Parse(time.RFC3339Nano, second)
	return err == nil && one.Equal(other)
}

// threadSpansMail fails unless the thread the last read answered starts and ends when its mail titled subject arrived.
func (d *deleteScenario) threadSpansMail(subject string) error {
	thread, err := d.openedThread()
	if err != nil {
		return err
	}
	if thread == nil {
		return fmt.Errorf("the read answered %s, want a thread", d.replied())
	}
	at := slices.IndexFunc(thread.Messages, func(mail servedMail) bool { return mail.Subject == subject })
	if at < 0 {
		return fmt.Errorf("the thread lists the mails %q, want %q among them", listedMails(thread.Messages), subject)
	}
	received := thread.Messages[at].ReceivedAt
	if !sameInstant(thread.FirstMessageAt, received) || !sameInstant(thread.LastMessageAt, received) {
		return fmt.Errorf("the thread spans %s to %s, want both ends at %s when %q was received",
			thread.FirstMessageAt, thread.LastMessageAt, received, subject)
	}
	return nil
}

// readKeptMail reads what is kept of the mail titled subject as email, its text, its original and its files.
func (d *deleteScenario) readKeptMail(ctx context.Context, email, subject string) error {
	return d.readMailThrough(ctx, email, subject, keptMailQuery)
}

// mailNamesDeletedFile fails unless the mail the last read answered names the file as deleted, keeping type and size.
func (d *deleteScenario) mailNamesDeletedFile(filename, contentType string) error {
	size, learned := d.sizes[filename]
	if !learned {
		return fmt.Errorf("no delete learned the size of the file %s", filename)
	}
	return d.checkOpenedMail(func(mail servedMail) error {
		file, err := fileCalled(mail.Attachments, filename)
		if err != nil {
			return err
		}
		if file.ContentType != contentType || file.Size != size || file.State != fileDeleted || file.DownloadPath != nil {
			return fmt.Errorf("names the file %s as %s of %v bytes, %q at %s, want %s of %v bytes deleted with no download",
				filename, file.ContentType, file.Size, file.State, shownPath(file.DownloadPath), contentType, size)
		}
		return nil
	})
}

// mailNamesDeletedOriginal fails unless the mail the last read answered names its original as deleted at its old size.
func (d *deleteScenario) mailNamesDeletedOriginal() error {
	return d.checkOpenedMail(func(mail servedMail) error {
		size, learned := d.rawSizes[mail.Subject]
		if !learned {
			return errors.New("was read by no delete of its original")
		}
		if mail.RawSize != size || mail.RawState != fileDeleted || mail.OriginalPath != nil {
			return fmt.Errorf("keeps its original of %v bytes as %q at %s, want %v bytes deleted with no download",
				mail.RawSize, mail.RawState, shownPath(mail.OriginalPath), size)
		}
		return nil
	})
}

// readUsage reads the space the workspace of email uses, its costliest mails on a page of the default size.
func (d *deleteScenario) readUsage(ctx context.Context, email string) error {
	return d.readUsageWith(ctx, email, nil)
}

// readUsageCostliest reads the space the workspace of email uses with its limit costliest mails.
func (d *deleteScenario) readUsageCostliest(ctx context.Context, email string, limit int) error {
	return d.readUsageWith(ctx, email, map[string]any{"limit": limit})
}

// readUsageWith reads the space the workspace of email uses through the variables and keeps what it answered.
func (d *deleteScenario) readUsageWith(ctx context.Context, email string, variables map[string]any) error {
	if err := d.read(ctx, email, usageQuery, variables); err != nil {
		return err
	}
	var usage servedUsage
	if err := d.reply.decode("emailUsage", &usage); err != nil {
		return err
	}
	d.usages = append(d.usages, usage)
	return nil
}

// lastUsage answers what the last read of the usage answered.
func (d *deleteScenario) lastUsage() (servedUsage, error) {
	if len(d.usages) == 0 {
		return servedUsage{}, errors.New("no step read the usage, want one before it is checked")
	}
	return d.usages[len(d.usages)-1], nil
}

// usageCountsEveryMail fails unless the last usage counts the bytes of each mail it lists and frees as freeing says.
func (d *deleteScenario) usageCountsEveryMail(freeing string) error {
	usage, err := d.lastUsage()
	if err != nil {
		return err
	}
	want := d.freed
	if freeing == freesNothing {
		want = 0
	}
	stored := 0.0
	for _, mail := range usage.LargestMessages {
		stored += mail.StoredBytes
	}
	if usage.FreeingBytes != want || usage.UsedBytes != stored+want {
		return fmt.Errorf("the usage counts %v bytes with %v freeing, want %v for its mail and %v freeing",
			usage.UsedBytes, usage.FreeingBytes, stored+want, want)
	}
	return nil
}

// usageUnchanged fails unless the last two reads of the usage answered the same bytes used and freeing.
func (d *deleteScenario) usageUnchanged() error {
	if len(d.usages) < 2 {
		return fmt.Errorf("%d steps read the usage, want two to compare", len(d.usages))
	}
	before, after := d.usages[len(d.usages)-2], d.usages[len(d.usages)-1]
	if after.UsedBytes != before.UsedBytes || after.FreeingBytes != before.FreeingBytes {
		return fmt.Errorf("the usage counts %v bytes with %v freeing, want %v with %v freeing as before",
			after.UsedBytes, after.FreeingBytes, before.UsedBytes, before.FreeingBytes)
	}
	return nil
}

// usageLimitIs fails unless the last usage answers a limit of limit bytes.
func (d *deleteScenario) usageLimitIs(limit int) error {
	usage, err := d.lastUsage()
	if err != nil {
		return err
	}
	if usage.LimitBytes == nil || *usage.LimitBytes != float64(limit) {
		return fmt.Errorf("the usage answers the limit %v, want %d bytes", usage.LimitBytes, limit)
	}
	return nil
}

// usageHasNoLimit fails unless the last usage answers no limit.
func (d *deleteScenario) usageHasNoLimit() error {
	usage, err := d.lastUsage()
	if err != nil {
		return err
	}
	if usage.LimitBytes != nil {
		return fmt.Errorf("the usage answers a limit of %v bytes, want none", *usage.LimitBytes)
	}
	return nil
}

// usageListsCostliest fails unless the last usage lists the mails titled as the comma list want, costliest first.
func (d *deleteScenario) usageListsCostliest(want string) error {
	usage, err := d.lastUsage()
	if err != nil {
		return err
	}
	listed := usage.LargestMessages
	costliestFirst := slices.IsSortedFunc(listed, func(a, b servedMail) int {
		return cmp.Compare(b.StoredBytes, a.StoredBytes)
	})
	if listedMails(listed) != want || !costliestFirst {
		return fmt.Errorf("the usage lists the mails %q at %v bytes, want %q costliest first",
			listedMails(listed), storedBytesOf(listed), want)
	}
	return nil
}

// storedBytesOf answers the bytes each of mails stores, in their order.
func storedBytesOf(mails []servedMail) []float64 {
	stored := make([]float64, len(mails))
	for i, mail := range mails {
		stored[i] = mail.StoredBytes
	}
	return stored
}

// usageListsMailWithFiles fails unless the last usage lists the mail titled subject with the kept files of filenames.
func (d *deleteScenario) usageListsMailWithFiles(subject, filenames string) error {
	usage, err := d.lastUsage()
	if err != nil {
		return err
	}
	at := slices.IndexFunc(usage.LargestMessages, func(mail servedMail) bool { return mail.Subject == subject })
	if at < 0 {
		return fmt.Errorf("the usage lists the mails %q, want %q among them", listedMails(usage.LargestMessages), subject)
	}
	mail := usage.LargestMessages[at]
	names := make([]string, len(mail.Attachments))
	for i, file := range mail.Attachments {
		names[i] = file.Filename
		if keptSize(file) == 0 {
			return fmt.Errorf("the usage lists the file %s of %q as %q of %v bytes, want it kept",
				file.Filename, subject, file.State, file.Size)
		}
	}
	if strings.Join(names, ", ") != filenames || mail.StoredBytes < keptBytes(mail) {
		return fmt.Errorf("the usage lists %q at %v bytes with the files %v, want the files %q counted in its bytes",
			subject, mail.StoredBytes, mail.Attachments, filenames)
	}
	return nil
}

func TestEmailDelete(t *testing.T) {
	runFeature(t, emailDeleteFeature, initializeEmailDelete(t))
}
