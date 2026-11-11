// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"crypto/sha256"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/plugins/email/message"
)

// mailKeys are the thread keys of one mail.
type mailKeys struct {
	// provider is the key of the provider's thread id, nil for none.
	provider *threadKey
	// replies are the keys of the reply ids, nearest first.
	replies []threadKey
	// own is the key of the mail's own Message-ID, nil for none.
	own *threadKey
	// index is the key of the Thread-Index conversation, nil for none.
	index *threadKey
}

// threadMail is what choosing the thread of one mail reads of it.
type threadMail struct {
	// mailboxID names the mailbox the mail reached.
	mailboxID uuid.UUID
	// message is the mail as Parse or ReadHeaders read it.
	message *message.Message
	// subject is the subject the mail keeps, cut to the subject cap.
	subject string
	// receivedAt is when the mailbox received the mail.
	receivedAt time.Time
	// sentCopy reports a copy read from the mailbox's Sent folder.
	sentCopy bool
	// automated reports a mail a program sent rather than a person.
	automated bool
	// contacts maps each email the mail names to the contact it matched.
	contacts map[string]uuid.UUID
	// keys are the thread keys of the mail.
	keys mailKeys
}

// threadSearch is one mail beside the threads its keys lead to and the contacts those threads hold.
type threadSearch struct {
	// mail is the mail looking for its thread.
	mail threadMail
	// keyed maps each header key of the mail the mailbox holds to the thread it leads to.
	keyed map[threadKey]keyedThread
	// providerThreads are the threads the provider thread id of the mail leads to near its time, nearest first.
	providerThreads []uuid.UUID
	// contacts maps each thread a key leads to to the contacts its mail is linked to.
	contacts map[uuid.UUID][]uuid.UUID
}

// keysOf returns the thread keys of m, its provider thread id and its reply ids cut to limit.
func keysOf(m *message.Message, providerThreadID string, limit int) mailKeys {
	keys := mailKeys{replies: replyKeys(m, limit)}
	if providerThreadID != "" {
		keys.provider = keyOf(providerThreadKind, []byte(providerThreadID))
	}
	if m.MessageID != "" {
		keys.own = keyOf(internetMessageIDKind, []byte(m.MessageID))
	}
	if m.ThreadIndex != nil {
		keys.index = keyOf(threadIndexKind, m.ThreadIndex.GUID[:])
	}
	return keys
}

// keyOf returns the key of kind that hashes id.
func keyOf(kind string, id []byte) *threadKey {
	return &threadKey{kind: kind, hash: sha256.Sum256(id)}
}

// replyKeys returns the keys of the reply ids of m nearest first, without its own id, the emails it names and repeats.
func replyKeys(m *message.Message, limit int) []threadKey {
	emails := namedEmails(m)
	seen := map[string]bool{m.MessageID: true}
	var keys []threadKey
	for _, id := range replyIDs(m, limit) {
		if seen[id] || emails[strings.ToLower(id)] {
			continue
		}
		seen[id] = true
		keys = append(keys, *keyOf(internetMessageIDKind, []byte(id)))
	}
	return keys
}

// replyIDs returns the first In-Reply-To id of m, then its References newest first, cut to limit.
func replyIDs(m *message.Message, limit int) []string {
	var ids []string
	if len(m.InReplyTo) > 0 {
		ids = append(ids, m.InReplyTo[0])
	}
	for _, id := range slices.Backward(capIDs(m.References, limit)) {
		ids = append(ids, id)
	}
	return ids
}

// capIDs returns ids cut to at most limit, keeping the first and the newest.
func capIDs(ids []string, limit int) []string {
	if len(ids) <= limit {
		return ids
	}
	return slices.Concat(ids[:1], ids[len(ids)-limit+1:])
}

// namedEmails returns every email the address fields of m name.
func namedEmails(m *message.Message) map[string]bool {
	emails := map[string]bool{}
	for _, address := range slices.Concat([]message.Address{m.From}, m.ReplyTo, m.To, m.Cc, m.Bcc) {
		emails[address.Email] = true
	}
	return emails
}

// subjectKeyOf returns the SHA-256 of subject lowercased and normalized, nil when that is empty.
func subjectKeyOf(subject string) []byte {
	normalized := normalizedSubject(subject)
	if normalized == "" {
		return nil
	}
	hash := sha256.Sum256([]byte(normalized))
	return hash[:]
}

// mentioned returns every key of the mail but its own Message-ID.
func (keys mailKeys) mentioned() []threadKey {
	listed := slices.Clone(keys.replies)
	for _, key := range []*threadKey{keys.provider, keys.index} {
		if key != nil {
			listed = append(listed, *key)
		}
	}
	return listed
}

// headerKeys returns every key the headers of the mail give, all but the provider thread id.
func (keys mailKeys) headerKeys() []threadKey {
	listed := slices.Clone(keys.replies)
	for _, key := range []*threadKey{keys.own, keys.index} {
		if key != nil {
			listed = append(listed, *key)
		}
	}
	return listed
}

// namedContacts returns every contact the mail names.
func (mail threadMail) namedContacts() []uuid.UUID {
	return slices.Collect(maps.Values(mail.contacts))
}

// senderContacts returns the contacts on From of an inbound mail, or on To, Cc and Bcc of a Sent copy.
func (mail threadMail) senderContacts() []uuid.UUID {
	senders := []message.Address{mail.message.From}
	if mail.sentCopy {
		senders = slices.Concat(mail.message.To, mail.message.Cc, mail.message.Bcc)
	}
	var contacts []uuid.UUID
	for _, sender := range senders {
		if contact, found := mail.contacts[sender.Email]; found {
			contacts = append(contacts, contact)
		}
	}
	return contacts
}

// takesSubjectStep reports whether the mail is a reply or a forward by its subject from a person.
func (mail threadMail) takesSubjectStep() bool {
	return !mail.automated && message.CarriesReplyTag(mail.message.Subject)
}

// chooseThread returns the thread a mail joins inside tx, merging threads or starting one as the thread steps say.
func chooseThread(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail threadMail, window time.Duration,
) (uuid.UUID, error) {
	thread, found, err := findThread(ctx, tx, tenant, mail, window)
	if err != nil || found {
		return thread, err
	}
	entry := threadEntry{subject: mail.subject, subjectKey: subjectKeyOf(mail.message.Subject), at: mail.receivedAt}
	return insertThread(ctx, tx, tenant, mail.mailboxID, entry)
}

// findThread returns the thread the first step that finds one answers, and whether a step finds one.
func findThread(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail threadMail, window time.Duration,
) (uuid.UUID, bool, error) {
	search, err := readThreadSearch(ctx, tx, tenant, mail, window)
	if err != nil {
		return uuid.Nil, false, err
	}
	if passing := search.findByProviderThread(); len(passing) > 0 {
		thread, err := search.mergeSharingThreads(ctx, tx, tenant, passing)
		return thread, err == nil, err
	}
	if passing := search.findByReplyIDs(); len(passing) > 0 {
		thread, err := search.mergeSharingThreads(ctx, tx, tenant, passing)
		return thread, err == nil, err
	}
	if thread, found := search.findByThreadIndex(); found {
		return thread, true, nil
	}
	return findBySubject(ctx, tx, tenant, mail, window)
}

// readThreadSearch reads the threads the keys of a mail lead to near its time, and the contacts of those threads.
func readThreadSearch(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail threadMail, window time.Duration,
) (threadSearch, error) {
	keyed, err := readKeyedThreads(ctx, tx, tenant, mail.mailboxID, mail.keys.headerKeys())
	if err != nil {
		return threadSearch{}, err
	}
	var providerThreads []uuid.UUID
	if mail.keys.provider != nil {
		providerThreads, err = readProviderThreads(ctx, tx, tenant, providerSearch{
			mailboxID: mail.mailboxID, key: *mail.keys.provider, at: mail.receivedAt, window: window,
		})
		if err != nil {
			return threadSearch{}, err
		}
	}
	threads := slices.Clone(providerThreads)
	for _, held := range keyed {
		threads = append(threads, held.threadID)
	}
	contacts, err := readThreadContacts(ctx, tx, tenant, threads)
	if err != nil {
		return threadSearch{}, err
	}
	return threadSearch{mail: mail, keyed: keyed, providerThreads: providerThreads, contacts: contacts}, nil
}

// lookup returns the thread key leads to, and false when key is nil or leads nowhere.
func (s threadSearch) lookup(key *threadKey) (keyedThread, bool) {
	if key == nil {
		return keyedThread{}, false
	}
	held, found := s.keyed[*key]
	return held, found
}

// holds reports whether a mail of thread is linked to a contact among contacts.
func (s threadSearch) holds(thread uuid.UUID, contacts []uuid.UUID) bool {
	return sharesContact(s.contacts[thread], contacts)
}

// findByProviderThread returns the threads the provider thread id leads to near the mail that hold a sender.
func (s threadSearch) findByProviderThread() []uuid.UUID {
	senders := s.mail.senderContacts()
	return slices.DeleteFunc(slices.Clone(s.providerThreads), func(thread uuid.UUID) bool {
		return !s.holds(thread, senders)
	})
}

// findByThreadIndex returns the thread the Thread-Index conversation leads to when it holds a sender.
func (s threadSearch) findByThreadIndex() (uuid.UUID, bool) {
	held, found := s.lookup(s.mail.keys.index)
	if !found || !s.holds(held.threadID, s.mail.senderContacts()) {
		return uuid.Nil, false
	}
	return held.threadID, true
}

// replyThreads returns each once the threads the reply ids lead to nearest first, then the thread of an own id mention.
func (s threadSearch) replyThreads() []uuid.UUID {
	var threads []uuid.UUID
	for _, key := range s.mail.keys.replies {
		if held, found := s.keyed[key]; found {
			threads = append(threads, held.threadID)
		}
	}
	if held, found := s.lookup(s.mail.keys.own); found && !held.mailHeld {
		threads = append(threads, held.threadID)
	}
	return distinct(threads)
}

// findByReplyIDs returns the threads the reply ids lead to that hold a named contact, nearest first.
func (s threadSearch) findByReplyIDs() []uuid.UUID {
	named := s.mail.namedContacts()
	return slices.DeleteFunc(s.replyThreads(), func(thread uuid.UUID) bool { return !s.holds(thread, named) })
}

// mergeSharingThreads merges into the first of threads each later one sharing a contact with it, returning the kept.
func (s threadSearch) mergeSharingThreads(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, threads []uuid.UUID,
) (uuid.UUID, error) {
	kept, held := threads[0], s.contacts[threads[0]]
	for _, other := range threads[1:] {
		if !sharesContact(held, s.contacts[other]) {
			continue
		}
		merged, err := mergeThreads(ctx, tx, tenant, kept, other)
		if err != nil {
			return uuid.Nil, err
		}
		kept, held = merged, slices.Concat(held, s.contacts[other])
	}
	return kept, nil
}

// findBySubject returns the thread the subject step finds for a mail, and whether it finds one.
func findBySubject(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail threadMail, window time.Duration,
) (uuid.UUID, bool, error) {
	key := subjectKeyOf(mail.message.Subject)
	if key == nil || !mail.takesSubjectStep() {
		return uuid.Nil, false, nil
	}
	return readSubjectThread(ctx, tx, tenant, subjectSearch{
		mailboxID: mail.mailboxID, subjectKey: key, at: mail.receivedAt, window: window,
		senders: mail.senderContacts(),
	})
}

// claimMailKeys points the keys of a stored mail at its thread as claimThreadKeys does, taking back its own Message-ID.
func claimMailKeys(
	ctx context.Context, tx pgx.Tx, tenant uuid.UUID, mail threadMail, threadID, messageID uuid.UUID,
) error {
	if own := mail.keys.own; own != nil {
		if err := holdMessageIDKey(ctx, tx, tenant, mail.mailboxID, threadID, messageID, own.hash); err != nil {
			return err
		}
	}
	return claimThreadKeys(ctx, tx, tenant, messageID, mail.keys.mentioned())
}

// sharesContact reports whether a and b hold a contact in common.
func sharesContact(a, b []uuid.UUID) bool {
	return slices.ContainsFunc(a, func(contact uuid.UUID) bool { return slices.Contains(b, contact) })
}

// distinct returns ids in order, each once.
func distinct(ids []uuid.UUID) []uuid.UUID {
	var kept []uuid.UUID
	for _, id := range ids {
		if !slices.Contains(kept, id) {
			kept = append(kept, id)
		}
	}
	return kept
}
