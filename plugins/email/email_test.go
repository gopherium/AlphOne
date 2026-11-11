// SPDX-License-Identifier: Elastic-2.0

package email_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob/memblob"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/plugins/email"
	"github.com/gopherium/alphone/sdk"
)

// refusingDatabaseURL names a database address whose every connection attempt is refused at once.
const refusingDatabaseURL = "postgres://alphone@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"

// wiredBudget bounds how long a test waits for the started plugin to reach a host part.
const wiredBudget = 10 * time.Second

// overrunGrace is the context a test stops the plugin under while its sweep is held.
const overrunGrace = 100 * time.Millisecond

// requestMail is a mail of Maria Perez asking for a quote.
const requestMail = "From: Maria Perez <maria.perez@example.com>\r\nTo: info@example.com\r\n" +
	"Subject: Quote for the new lamp\r\nDate: Mon, 02 Nov 2026 09:00:00 +0000\r\n" +
	"Message-ID: <lamp-quote-request@example.com>\r\nContent-Type: text/plain\r\n\r\n" +
	"Could you send me a quote for the new lamp?\r\n"

// knownContacts lists the emails the wired tests store as contacts of the default workspace.
var knownContacts = []string{"maria.perez@example.com", "ada@example.com"}

// countingGate takes mail for every workspace and counts the calls.
type countingGate struct {
	mu    sync.Mutex
	calls int
}

// AcceptsMachineTraffic counts the call and answers true.
func (g *countingGate) AcceptsMachineTraffic(context.Context, uuid.UUID) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	return true, nil
}

// knownMatcher answers the contacts it holds by email.
type knownMatcher map[string]uuid.UUID

// MatchIdentities answers each identifier the matcher holds.
func (m knownMatcher) MatchIdentities(_ context.Context, _ sdk.Channel, identifiers []string) (
	map[string]sdk.Contact, error) {
	matched := map[string]sdk.Contact{}
	for _, identifier := range identifiers {
		if id, found := m[identifier]; found {
			matched[identifier] = sdk.Contact{ID: id}
		}
	}
	return matched, nil
}

// recordingStore hands every call to the host's file store, recording the type of each save and each delete.
type recordingStore struct {
	sdk.FileStore
	mu      sync.Mutex
	saved   []string
	deleted chan uuid.UUID
}

// SaveAs records the type and saves body through the host's store.
func (s *recordingStore) SaveAs(ctx context.Context, id uuid.UUID, contentType string, body io.Reader) (
	sdk.StoredFile, error) {
	s.mu.Lock()
	s.saved = append(s.saved, contentType)
	s.mu.Unlock()
	return s.FileStore.SaveAs(ctx, id, contentType, body)
}

// Delete sends id to deleted and deletes it through the host's store.
func (s *recordingStore) Delete(ctx context.Context, id uuid.UUID) error {
	s.deleted <- id
	return s.FileStore.Delete(ctx, id)
}

// savedTypes returns the type of every save so far.
func (s *recordingStore) savedTypes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.saved)
}

// recordingSettings hands every call to the host's settings store, recording each key looked up.
type recordingSettings struct {
	sdk.SettingStore
	mu     sync.Mutex
	lookup []string
}

// Lookup records key and reads it through the host's store.
func (s *recordingSettings) Lookup(ctx context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	s.lookup = append(s.lookup, key)
	s.mu.Unlock()
	return s.SettingStore.Lookup(ctx, key)
}

// recordingPublisher records the name of every event it is handed.
type recordingPublisher struct {
	mu    sync.Mutex
	names []string
}

// Publish records name.
func (p *recordingPublisher) Publish(_ context.Context, name string, _ map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.names = append(p.names, name)
}

// heldHandler is a log handler that holds every record until release closes, closing entered at the first.
type heldHandler struct {
	entered chan struct{}
	release chan struct{}
	once    *sync.Once
}

// Enabled reports every level as enabled.
func (heldHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

// Handle closes entered once and waits for release.
func (h heldHandler) Handle(context.Context, slog.Record) error {
	h.once.Do(func() { close(h.entered) })
	<-h.release
	return nil
}

// WithAttrs returns the handler itself.
func (h heldHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

// WithGroup returns the handler itself.
func (h heldHandler) WithGroup(string) slog.Handler {
	return h
}

// wiredPlugin is the email plugin over a migrated database, handed every host part it takes.
type wiredPlugin struct {
	plugin    *email.Plugin
	pool      *pgxpool.Pool
	gate      *countingGate
	store     *recordingStore
	settings  *recordingSettings
	publisher *recordingPublisher
}

// newWiredPlugin registers the plugin over a core migrated database knowing every known contact, then migrates it.
func newWiredPlugin(t *testing.T, env map[string]string) wiredPlugin {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	address := testdb.New(t, testdb.Migrator())
	pool, err := pgxpool.New(t.Context(), address)
	if err != nil {
		t.Fatalf("opening a pool over the test database: %v", err)
	}
	t.Cleanup(pool.Close)
	matcher := knownMatcher{}
	for _, known := range knownContacts {
		matcher[known] = uuid.Must(uuid.NewV7())
		if _, err := pool.Exec(t.Context(),
			"INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, $2, now(), $3)",
			matcher[known], known, sdk.DefaultTenantID); err != nil {
			t.Fatalf("storing the contact %s: %v", known, err)
		}
	}
	w := wiredPlugin{
		pool: pool, gate: &countingGate{}, publisher: &recordingPublisher{},
		store: &recordingStore{
			FileStore: storage.NewFileStore(memblob.OpenBucket(nil), "email", 1<<20), deleted: make(chan uuid.UUID, 8),
		},
		settings: &recordingSettings{SettingStore: postgres.NewSettingStore(pool, nil).ForPlugin("email")},
	}
	w.plugin, err = email.Register(sdk.Deps{
		DatabaseURL: address, Matcher: matcher, ContactMatchMax: 100, Events: w.publisher, Env: settingsOf(env),
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = w.plugin.Stop(context.Background()) })
	w.plugin.UseTenantGate(w.gate)
	w.plugin.UseFileStore(w.store)
	w.plugin.UseSettingStore(w.settings)
	if err := w.plugin.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return w
}

// count reads the one count statement answers for the default workspace.
func (w wiredPlugin) count(t *testing.T, statement string) int {
	t.Helper()
	var held int
	if err := w.pool.QueryRow(t.Context(), statement, sdk.DefaultTenantID).Scan(&held); err != nil {
		t.Fatalf("counting %q: %v", statement, err)
	}
	return held
}

// settingsOf returns a reader under the program prefix answering only the given settings.
func settingsOf(held map[string]string) sdk.Env {
	return sdk.Env{Prefix: "ALPHONE_", Getenv: func(name string) string { return held[name] }}
}

func TestIDIsEmail(t *testing.T) {
	t.Parallel()

	p := newPlugin(t)

	if got := p.ID(); got != "email" {
		t.Errorf("ID() = %q, want %q", got, "email")
	}
}

func TestRegisterAcceptsEmptyDeps(t *testing.T) {
	t.Parallel()

	p, err := email.Register(sdk.Deps{})

	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	if p == nil {
		t.Fatal("Register() = nil, want a plugin")
	}
}

func TestRegisterNamesASettingThatDoesNotRead(t *testing.T) {
	t.Parallel()

	p, err := email.Register(sdk.Deps{Env: settingsOf(map[string]string{"ALPHONE_EMAIL_SWEEP_BATCH": "many"})})

	if !strings.Contains(fmt.Sprint(err), "ALPHONE_EMAIL_SWEEP_BATCH") || p != nil {
		t.Errorf("Register() = %v, %v, want no plugin and an error naming ALPHONE_EMAIL_SWEEP_BATCH", p, err)
	}
}

func TestRegisterNamesTheEmailPluginForADatabaseAddressThatDoesNotParse(t *testing.T) {
	t.Parallel()

	p, err := email.Register(sdk.Deps{DatabaseURL: "://not-a-url"})

	if !strings.HasPrefix(fmt.Sprint(err), "email: ") || p != nil {
		t.Errorf("Register() = %v, %v, want no plugin and an error that names the email plugin", p, err)
	}
}

func TestStartAndStopAnswerNil(t *testing.T) {
	t.Parallel()

	p := newPlugin(t)

	if err := p.Start(t.Context()); err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
	if err := p.Stop(t.Context()); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestThePluginTakesEachOptionalPartItUses(t *testing.T) {
	t.Parallel()

	var p sdk.Plugin = newPlugin(t)
	parts := []struct {
		name  string
		takes bool
		uses  bool
	}{
		{"Migrator", implements[sdk.Migrator](p), true},
		{"Seeder", implements[sdk.Seeder](p), true},
		{"RouteProvider", implements[sdk.RouteProvider](p), false},
		{"PublicPathProvider", implements[sdk.PublicPathProvider](p), false},
		{"CommandProvider", implements[sdk.CommandProvider](p), true},
		{"EventProvider", implements[sdk.EventProvider](p), true},
		{"AreaProvider", implements[sdk.AreaProvider](p), false},
		{"RoleProvider", implements[sdk.RoleProvider](p), true},
		{"TopRoleProvider", implements[sdk.TopRoleProvider](p), false},
		{"FieldSource", implements[sdk.FieldSource](p), false},
		{"FieldProvider", implements[sdk.FieldProvider](p), false},
		{"FieldConsumer", implements[sdk.FieldConsumer](p), false},
		{"CredentialProvider", implements[sdk.CredentialProvider](p), false},
		{"CredentialConsumer", implements[sdk.CredentialConsumer](p), false},
		{"MailSenderConsumer", implements[sdk.MailSenderConsumer](p), false},
		{"TenantGateConsumer", implements[sdk.TenantGateConsumer](p), true},
		{"RoleCheckerConsumer", implements[sdk.RoleCheckerConsumer](p), false},
		{"SettingStoreConsumer", implements[sdk.SettingStoreConsumer](p), true},
		{"EncrypterConsumer", implements[sdk.EncrypterConsumer](p), false},
		{"FileStoreConsumer", implements[sdk.FileStoreConsumer](p), true},
	}

	for _, part := range parts {
		if part.takes != part.uses {
			t.Errorf("the plugin implements sdk.%s is %v, want %v", part.name, part.takes, part.uses)
		}
	}
}

func TestEventsDeclaresTheMessageReceivedEvent(t *testing.T) {
	t.Parallel()

	if got := newPlugin(t).Events(); !slices.Equal(got, []string{"email.message.received"}) {
		t.Errorf("Events() = %v, want email.message.received alone", got)
	}
}

func TestRolesGiveAdminTheMailboxCapability(t *testing.T) {
	t.Parallel()

	got := newPlugin(t).Roles()

	want := []sdk.RoleDeclaration{{Name: "admin", Capabilities: []string{"email.manage_mailbox"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Roles() = %+v, want admin carrying email.manage_mailbox alone", got)
	}
}

func TestCommandsOfferEmailIngestUnderTheMailboxCapability(t *testing.T) {
	t.Parallel()

	got := newPlugin(t).Commands()

	if len(got) != 1 {
		t.Fatalf("Commands() = %+v, want email:ingest alone", got)
	}
	if got[0].Name != "email:ingest" || got[0].Capability != "email.manage_mailbox" {
		t.Errorf("Commands() = %+v, want email:ingest under email.manage_mailbox", got)
	}
}

func TestEmailIngestStoresAMailThroughTheHostPartsThePluginIsHanded(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)
	var answered bytes.Buffer

	err := w.plugin.Commands()[0].Run(t.Context(), sdk.Call{
		Args:  []string{sdk.DefaultTenantID.String()},
		Flags: map[string]string{"mailbox": "info@example.com", "received": "2026-11-02T09:00:00Z"},
		Stdin: strings.NewReader(requestMail), Stdout: &answered, Stderr: io.Discard, Apply: true,
	})

	if err != nil || answered.String() != "stored\n" {
		t.Fatalf("email:ingest = %q, %v, want stored", answered.String(), err)
	}
	if w.gate.calls != 2 {
		t.Errorf("the gate was asked %d times, want once before the mailbox is stored and once by the ingest",
			w.gate.calls)
	}
	if got := w.store.savedTypes(); !slices.Equal(got, []string{"message/rfc822"}) {
		t.Errorf("the file store saved %v, want the raw copy", got)
	}
	if !slices.Equal(w.settings.lookup, []string{"workspace_max_bytes"}) {
		t.Errorf("the settings store read %v, want the workspace's own mail limit", w.settings.lookup)
	}
	if !slices.Equal(w.publisher.names, []string{"email.message.received"}) {
		t.Errorf("the plugin published %v, want email.message.received", w.publisher.names)
	}
}

func TestSeedStoresTheSampleMailboxInTheDefaultWorkspaceOnce(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, nil)

	if err := w.plugin.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}
	saved := len(w.store.savedTypes())
	if err := w.plugin.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() again error = %v, want nil", err)
	}

	if boxes := w.count(t, "SELECT count(*) FROM plugin_email.mailboxes WHERE tenant_id = $1"); boxes != 1 {
		t.Errorf("the default workspace holds %d mailboxes, want 1", boxes)
	}
	if mails := w.count(t, "SELECT count(*) FROM plugin_email.messages WHERE tenant_id = $1"); mails != 5 {
		t.Errorf("the default workspace holds %d mails, want 5", mails)
	}
	if again := len(w.store.savedTypes()); saved != 7 || again != saved {
		t.Errorf("the seeds saved %d files and then %d more, want five raw copies and two attachments, then none",
			saved, again-saved)
	}
	if len(w.publisher.names) != 0 {
		t.Errorf("the seed published %v, want no event", w.publisher.names)
	}
}

func TestStartSweepsTheQueueThroughTheFileStoreThePluginIsHanded(t *testing.T) {
	t.Parallel()

	w := newWiredPlugin(t, map[string]string{"ALPHONE_EMAIL_SWEEP_INTERVAL": "20ms"})
	queued := uuid.Must(uuid.NewV7())
	if _, err := w.pool.Exec(t.Context(), `INSERT INTO plugin_email.pending_files (tenant_id, file_id, state, size)
		VALUES ($1, $2, 'deleting', 10)`, sdk.DefaultTenantID, queued); err != nil {
		t.Fatalf("queueing a file: %v", err)
	}

	if err := w.plugin.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	select {
	case deleted := <-w.store.deleted:
		if deleted != queued {
			t.Errorf("the sweep deleted %v, want the queued %v", deleted, queued)
		}
	case <-time.After(wiredBudget):
		t.Fatalf("the sweep deleted nothing within %v, want the queued file", wiredBudget)
	}
	if err := w.plugin.Stop(t.Context()); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestStopAnswersTheErrorOfItsContextWhenTheSweepOverrunsIt(t *testing.T) {
	t.Parallel()

	held := heldHandler{entered: make(chan struct{}), release: make(chan struct{}), once: &sync.Once{}}
	p, err := email.Register(sdk.Deps{
		DatabaseURL: refusingDatabaseURL, Logger: slog.New(held),
		Env: settingsOf(map[string]string{"ALPHONE_EMAIL_SWEEP_INTERVAL": "1ms"}),
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		close(held.release)
		_ = p.Stop(context.Background())
	})
	if err := p.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	select {
	case <-held.entered:
	case <-time.After(wiredBudget):
		t.Fatalf("the sweep logged nothing within %v, want its failed read held in the log", wiredBudget)
	}
	ctx, cancel := context.WithTimeout(t.Context(), overrunGrace)
	defer cancel()

	err = p.Stop(ctx)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop() error = %v, want the deadline of its context", err)
	}
}

// implements reports whether p satisfies the interface T.
func implements[T any](p sdk.Plugin) bool {
	_, ok := p.(T)
	return ok
}

// newPlugin registers the email plugin on empty deps, stopped at cleanup.
func newPlugin(t *testing.T) *email.Plugin {
	t.Helper()
	p, err := email.Register(sdk.Deps{})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	return p
}
