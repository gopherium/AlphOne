// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

const (
	// defaultMessageMaxBytes is the largest raw mail read when ALPHONE_EMAIL_MESSAGE_MAX_BYTES is unset, 35 MiB.
	defaultMessageMaxBytes = 35 << 20
	// defaultMessageMaxParts is the most parts one mail holds when ALPHONE_EMAIL_MESSAGE_MAX_PARTS is unset.
	defaultMessageMaxParts = 200
	// defaultMessageMaxContainers is the most containers a mail holds when ALPHONE_EMAIL_MESSAGE_MAX_CONTAINERS is unset.
	defaultMessageMaxContainers = 16
	// defaultMessageMaxFieldBytes is the longest media field read when ALPHONE_EMAIL_MESSAGE_MAX_FIELD_BYTES is unset.
	defaultMessageMaxFieldBytes = 4096
	// defaultHeaderMaxBytes is the largest top header block read when ALPHONE_EMAIL_HEADER_MAX_BYTES is unset, 256 KiB.
	defaultHeaderMaxBytes = 256 << 10
	// defaultBodyMaxBytes is the most text and HTML kept when ALPHONE_EMAIL_BODY_MAX_BYTES is unset, 4 MiB.
	defaultBodyMaxBytes = 4 << 20
	// defaultSubjectMaxBytes is the most of a subject kept when ALPHONE_EMAIL_SUBJECT_MAX_BYTES is unset.
	defaultSubjectMaxBytes = 2048
	// defaultNameMaxBytes is the most of a name or content id kept when ALPHONE_EMAIL_NAME_MAX_BYTES is unset.
	defaultNameMaxBytes = 512
	// defaultRecipientsMax is the most addresses of one mail matched and kept when ALPHONE_EMAIL_RECIPIENTS_MAX is unset.
	defaultRecipientsMax = 500
	// defaultMailsInMemoryMax is the most mails held at once when ALPHONE_EMAIL_MAILS_IN_MEMORY_MAX is unset.
	defaultMailsInMemoryMax = 1
	// defaultSnippetLetters is the length of a mail's preview when ALPHONE_EMAIL_SNIPPET_LETTERS is unset.
	defaultSnippetLetters = 200
	// defaultAttachmentMaxBytes is the largest attachment kept when ALPHONE_EMAIL_ATTACHMENT_MAX_BYTES is unset, 25 MiB.
	defaultAttachmentMaxBytes = 25 << 20
	// defaultWorkspaceMaxBytes is the limit of a workspace when ALPHONE_EMAIL_WORKSPACE_MAX_BYTES is unset, 5 GiB.
	defaultWorkspaceMaxBytes = 5 << 30
	// defaultEventTextMaxBytes is the most text an event carries when ALPHONE_EMAIL_EVENT_TEXT_MAX_BYTES is unset, 16 KiB.
	defaultEventTextMaxBytes = 16 << 10
	// defaultThreadWindow is the subject and provider id window of a thread when ALPHONE_EMAIL_THREAD_WINDOW is unset.
	defaultThreadWindow = 14 * 24 * time.Hour
	// defaultThreadReferencesMax is the most reply ids kept when ALPHONE_EMAIL_THREAD_REFERENCES_MAX is unset.
	defaultThreadReferencesMax = 64
	// defaultRetryWindow is how long a mail may keep failing when ALPHONE_EMAIL_RETRY_WINDOW is unset.
	defaultRetryWindow = 6 * time.Hour
	// defaultCleanupTimeout bounds the clean up of a failed run when ALPHONE_EMAIL_CLEANUP_TIMEOUT is unset.
	defaultCleanupTimeout = 10 * time.Second
	// defaultSweepInterval is the pause between two sweeps when ALPHONE_EMAIL_SWEEP_INTERVAL is unset.
	defaultSweepInterval = time.Hour
	// defaultSweepBatch is the most rows of one workspace a sweep statement takes when ALPHONE_EMAIL_SWEEP_BATCH is unset.
	defaultSweepBatch = 100
	// defaultSweepBudget is how long one sweep may run when ALPHONE_EMAIL_SWEEP_BUDGET is unset.
	defaultSweepBudget = 10 * time.Minute
	// defaultOrphanAge is how long a file of a stopped run waits for the sweep when ALPHONE_EMAIL_ORPHAN_AGE is unset.
	defaultOrphanAge = 24 * time.Hour
)

// settings holds every limit and interval the email plugin reads from the environment.
type settings struct {
	// parse bounds what Parse and ReadHeaders read of one raw mail.
	parse message.Limits
	// caps bound how many mails a process holds and how much of each one it keeps and publishes.
	caps mailCaps
	// files bound the bytes one attachment and one workspace keep as files.
	files fileCaps
	// times bound the thread window, how long a mail keeps failing and how long a clean up runs.
	times timeLimits
	// sweep paces the loop that deletes the files no stored mail holds.
	sweep sweepSettings
}

// mailCaps bound how many mails a process holds and how much of each one it keeps and publishes.
type mailCaps struct {
	// mailsInMemoryMax is the most mails one process holds in memory at once.
	mailsInMemoryMax int
	// subjectMaxBytes is the most bytes of a subject a mail and its thread keep.
	subjectMaxBytes int
	// nameMaxBytes is the most bytes of one display name, file name or content id a mail keeps.
	nameMaxBytes int
	// recipientsMax is the most addresses of one mail matched to contacts and kept.
	recipientsMax int
	// snippetLetters is the most letters the preview of one mail holds.
	snippetLetters int
	// threadReferencesMax is the most reply ids of one mail kept and used for threading.
	threadReferencesMax int
	// eventTextMaxBytes is the most bytes of a mail's text its event carries.
	eventTextMaxBytes int
}

// fileCaps bound the bytes one attachment and one workspace keep as files.
type fileCaps struct {
	// attachmentMaxBytes is the most bytes one attachment kept as a file holds.
	attachmentMaxBytes int
	// workspaceMaxBytes is the most bytes of mail one workspace keeps, zero for no limit.
	workspaceMaxBytes int
}

// timeLimits bound the thread window, how long a mail keeps failing and how long a clean up runs.
type timeLimits struct {
	// threadWindow is how near a thread a mail must be to join it by its subject or its provider thread id.
	threadWindow time.Duration
	// retryWindow is how long a mail that keeps failing is tried again before it is set aside.
	retryWindow time.Duration
	// cleanupTimeout bounds the queueing of a failed run's files for deletion and the record of its failure.
	cleanupTimeout time.Duration
}

// sweepSettings pace the loop that deletes the files no stored mail holds.
type sweepSettings struct {
	// interval is the pause before each sweep.
	interval time.Duration
	// batch is the most rows of one workspace one sweep statement takes.
	batch int
	// budget is how long one sweep may run.
	budget time.Duration
	// orphanAge is how long a file a stopped run left waits before the sweep deletes it.
	orphanAge time.Duration
}

// readSettings returns every email setting from the environment, an error naming the first setting that does not read.
func readSettings(deps sdk.Deps) (settings, error) {
	parse, err := readParseLimits(deps)
	if err != nil {
		return settings{}, err
	}
	caps, err := readMailCaps(deps)
	if err != nil {
		return settings{}, err
	}
	files, err := readFileCaps(deps)
	if err != nil {
		return settings{}, err
	}
	times, err := readTimeLimits(deps)
	if err != nil {
		return settings{}, err
	}
	sweep, err := readSweepSettings(deps)
	if err != nil {
		return settings{}, err
	}
	return settings{parse: parse, caps: caps, files: files, times: times, sweep: sweep}, nil
}

// readParseLimits returns the limits Parse and ReadHeaders read one raw mail within.
func readParseLimits(deps sdk.Deps) (message.Limits, error) {
	env := deps.Env.Within("EMAIL_")
	maxBytes, err := env.Count("MESSAGE_MAX_BYTES", defaultMessageMaxBytes)
	if err != nil {
		return message.Limits{}, err
	}
	maxParts, err := env.Count("MESSAGE_MAX_PARTS", defaultMessageMaxParts)
	if err != nil {
		return message.Limits{}, err
	}
	maxContainers, err := env.Count("MESSAGE_MAX_CONTAINERS", defaultMessageMaxContainers)
	if err != nil {
		return message.Limits{}, err
	}
	maxFieldBytes, err := env.Count("MESSAGE_MAX_FIELD_BYTES", defaultMessageMaxFieldBytes)
	if err != nil {
		return message.Limits{}, err
	}
	maxHeaderBytes, err := env.Count("HEADER_MAX_BYTES", defaultHeaderMaxBytes)
	if err != nil {
		return message.Limits{}, err
	}
	maxBodyBytes, err := env.Count("BODY_MAX_BYTES", defaultBodyMaxBytes)
	if err != nil {
		return message.Limits{}, err
	}
	return message.Limits{
		MaxBytes: int64(maxBytes), MaxParts: maxParts, MaxContainers: maxContainers,
		MaxFieldBytes: maxFieldBytes, MaxHeaderBytes: maxHeaderBytes, MaxBodyBytes: maxBodyBytes,
	}, nil
}

// readMailCaps returns how many mails a process holds and how much of each one it keeps and publishes.
func readMailCaps(deps sdk.Deps) (mailCaps, error) {
	env := deps.Env.Within("EMAIL_")
	mailsInMemoryMax, err := env.Count("MAILS_IN_MEMORY_MAX", defaultMailsInMemoryMax)
	if err != nil {
		return mailCaps{}, err
	}
	subjectMaxBytes, err := env.Count("SUBJECT_MAX_BYTES", defaultSubjectMaxBytes)
	if err != nil {
		return mailCaps{}, err
	}
	nameMaxBytes, err := env.Count("NAME_MAX_BYTES", defaultNameMaxBytes)
	if err != nil {
		return mailCaps{}, err
	}
	recipientsMax, err := env.Count("RECIPIENTS_MAX", defaultRecipientsMax)
	if err != nil {
		return mailCaps{}, err
	}
	snippetLetters, err := env.Count("SNIPPET_LETTERS", defaultSnippetLetters)
	if err != nil {
		return mailCaps{}, err
	}
	threadReferencesMax, err := env.Count("THREAD_REFERENCES_MAX", defaultThreadReferencesMax)
	if err != nil {
		return mailCaps{}, err
	}
	eventTextMaxBytes, err := env.Count("EVENT_TEXT_MAX_BYTES", defaultEventTextMaxBytes)
	if err != nil {
		return mailCaps{}, err
	}
	return mailCaps{
		mailsInMemoryMax: mailsInMemoryMax, subjectMaxBytes: subjectMaxBytes, nameMaxBytes: nameMaxBytes,
		recipientsMax: recipientsMax, snippetLetters: snippetLetters, threadReferencesMax: threadReferencesMax,
		eventTextMaxBytes: eventTextMaxBytes,
	}, nil
}

// readFileCaps returns the bytes one attachment and one workspace keep as files, a workspace limit of zero for none.
func readFileCaps(deps sdk.Deps) (fileCaps, error) {
	env := deps.Env.Within("EMAIL_")
	attachmentMaxBytes, err := env.Count("ATTACHMENT_MAX_BYTES", defaultAttachmentMaxBytes)
	if err != nil {
		return fileCaps{}, err
	}
	workspaceMaxBytes, err := env.Count("WORKSPACE_MAX_BYTES", defaultWorkspaceMaxBytes, sdk.AllowZero())
	if err != nil {
		return fileCaps{}, err
	}
	return fileCaps{attachmentMaxBytes: attachmentMaxBytes, workspaceMaxBytes: workspaceMaxBytes}, nil
}

// readTimeLimits returns the thread window, how long a mail keeps failing and how long a clean up runs.
func readTimeLimits(deps sdk.Deps) (timeLimits, error) {
	env := deps.Env.Within("EMAIL_")
	threadWindow, err := env.Duration("THREAD_WINDOW", defaultThreadWindow)
	if err != nil {
		return timeLimits{}, err
	}
	retryWindow, err := env.Duration("RETRY_WINDOW", defaultRetryWindow)
	if err != nil {
		return timeLimits{}, err
	}
	cleanupTimeout, err := env.Duration("CLEANUP_TIMEOUT", defaultCleanupTimeout)
	if err != nil {
		return timeLimits{}, err
	}
	return timeLimits{threadWindow: threadWindow, retryWindow: retryWindow, cleanupTimeout: cleanupTimeout}, nil
}

// readSweepSettings returns the pace of the loop that deletes the files no stored mail holds.
func readSweepSettings(deps sdk.Deps) (sweepSettings, error) {
	env := deps.Env.Within("EMAIL_")
	interval, err := env.Duration("SWEEP_INTERVAL", defaultSweepInterval)
	if err != nil {
		return sweepSettings{}, err
	}
	batch, err := env.Count("SWEEP_BATCH", defaultSweepBatch)
	if err != nil {
		return sweepSettings{}, err
	}
	budget, err := env.Duration("SWEEP_BUDGET", defaultSweepBudget)
	if err != nil {
		return sweepSettings{}, err
	}
	orphanAge, err := env.Duration("ORPHAN_AGE", defaultOrphanAge)
	if err != nil {
		return sweepSettings{}, err
	}
	return sweepSettings{interval: interval, batch: batch, budget: budget, orphanAge: orphanAge}, nil
}
