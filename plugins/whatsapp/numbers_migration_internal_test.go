// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"github.com/gopherium/alphone/sdk"
)

// versionBeforeNumbers is the migration an install from before many numbers stands on.
const versionBeforeNumbers = 6

// insertOldCredentials stores one number the way a tenant held it before many numbers.
const insertOldCredentials = "INSERT INTO plugin_whatsapp.credentials (tenant_id, phone_number_id, access_token)" +
	" VALUES ($1, $2, $3)"

// insertDatedCredentials stores one number under a chosen age.
const insertDatedCredentials = "INSERT INTO plugin_whatsapp.credentials" +
	" (tenant_id, phone_number_id, access_token, created_at) VALUES ($1, $2, $3, $4)"

// insertDatedConversation stores one conversation on a number under a chosen age and activity.
const insertDatedConversation = `INSERT INTO plugin_whatsapp.conversations
	(id, contact_id, channel, external_id, status, last_activity_at, created_at, tenant_id, phone_number_id)
	VALUES ($1, $2, 'whatsapp', $3, 'open', $4, $5, $6, $7)`

// insertOneMessage stores one inbound message on a conversation.
const insertOneMessage = `INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
	content_type, sent_at, raw, created_at, tenant_id)
	VALUES ($1, $2, $3, 'inbound', 'hello', 'text', now(), '{}', now(), $4)`

// migrationSteps returns a provider that steps the plugin's migrations one by one on its pool.
func migrationSteps(t *testing.T, p *Plugin) *goose.Provider {
	t.Helper()
	db := stdlib.OpenDBFromPool(p.pool)
	t.Cleanup(func() { _ = db.Close() })
	versions, err := database.NewStore(database.DialectPostgres, "plugin_whatsapp.goose_db_version")
	if err != nil {
		t.Fatalf("building the version store: %v", err)
	}
	provider, err := goose.NewProvider("", db, migrationSource, goose.WithStore(versions))
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}
	return provider
}

// pluginBeforeNumbers returns a plugin migrated up to the version before many numbers, beside its steps.
func pluginBeforeNumbers(t *testing.T) (*Plugin, *goose.Provider) {
	t.Helper()
	p, _ := newUnmigratedPlugin(t)
	if _, err := p.pool.Exec(t.Context(), "CREATE SCHEMA plugin_whatsapp"); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}
	steps := migrationSteps(t, p)
	if _, err := steps.UpTo(t.Context(), versionBeforeNumbers); err != nil {
		t.Fatalf("UpTo(%d) error = %v, want nil", versionBeforeNumbers, err)
	}
	return p, steps
}

// pluginWithNumbers returns a plugin migrated through the numbers migration, beside its steps.
func pluginWithNumbers(t *testing.T) (*Plugin, *goose.Provider) {
	t.Helper()
	p, steps := pluginBeforeNumbers(t)
	if _, err := steps.Up(t.Context()); err != nil {
		t.Fatalf("Up() error = %v, want nil", err)
	}
	return p, steps
}

// seededContact stores one contact under the tenant.
func seededContact(t *testing.T, p *Plugin, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, $2, now(), $3)",
		contactID, "Maria Perez", tenantID); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	return contactID
}

// conversationSeed is one conversation with a customer under a chosen age and activity, nil number for no number.
type conversationSeed struct {
	id        uuid.UUID
	tenantID  uuid.UUID
	contactID uuid.UUID
	number    *string
	createdAt time.Time
	activeAt  time.Time
}

// datedConversation stores the conversation and returns it with its new id.
func datedConversation(t *testing.T, p *Plugin, row conversationSeed) conversationSeed {
	t.Helper()
	row.id = uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(), insertDatedConversation,
		row.id, row.contactID, "184467235", row.activeAt, row.createdAt, row.tenantID, row.number); err != nil {
		t.Fatalf("seeding the conversation: %v", err)
	}
	return row
}

// freshConversation stores a conversation of a new contact on the number, opened and active now.
func freshConversation(t *testing.T, p *Plugin, tenantID uuid.UUID, number *string) conversationSeed {
	t.Helper()
	now := time.Now().UTC()
	return datedConversation(t, p, conversationSeed{
		tenantID: tenantID, contactID: seededContact(t, p, tenantID), number: number, createdAt: now, activeAt: now,
	})
}

// oneMessage stores one inbound message on the conversation and returns its id.
func oneMessage(t *testing.T, p *Plugin, conversation conversationSeed, wamid string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(), insertOneMessage,
		id, conversation.id, wamid, conversation.tenantID); err != nil {
		t.Fatalf("seeding the message: %v", err)
	}
	return id
}

// threeConversations stores a customer on no number and two numbers, one message each, media on the last.
func threeConversations(
	t *testing.T, p *Plugin, customer conversationSeed, now time.Time,
) (conversationSeed, uuid.UUID) {
	t.Helper()
	onNoNumber, onFirst, onSecond := customer, customer, customer
	onNoNumber.createdAt, onNoNumber.activeAt = now.Add(-3*time.Hour), now.Add(-3*time.Hour)
	onFirst.number = addressOf("5550001")
	onFirst.createdAt, onFirst.activeAt = now.Add(-2*time.Hour), now.Add(-time.Hour)
	onSecond.number = addressOf("5550002")
	onSecond.createdAt, onSecond.activeAt = now.Add(-time.Hour), now.Add(-2*time.Hour)
	keeper := datedConversation(t, p, onNoNumber)
	oneMessage(t, p, keeper, "wamid.none")
	oneMessage(t, p, datedConversation(t, p, onFirst), "wamid.first")
	withMedia := oneMessage(t, p, datedConversation(t, p, onSecond), "wamid.second")
	ctx := sdk.WithTenant(t.Context(), customer.tenantID)
	descriptor := mediaDescriptor{mediaID: "MEDIA1", mimeType: "image/png"}
	if err := insertMediaPending(ctx, p.pool, withMedia, descriptor); err != nil {
		t.Fatalf("seeding the media: %v", err)
	}
	return keeper, withMedia
}

// keptRows counts what one conversation holds after the numbers migration ran down.
type keptRows struct {
	messages int
	media    int
}

// keptOn reads how many messages the conversation holds and how many media rows the message holds.
func keptOn(t *testing.T, p *Plugin, conversationID, messageID uuid.UUID) keptRows {
	t.Helper()
	var kept keptRows
	if err := p.pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM plugin_whatsapp.messages WHERE conversation_id = $1),
			(SELECT count(*) FROM plugin_whatsapp.media WHERE message_id = $2)`,
		conversationID, messageID).Scan(&kept.messages, &kept.media); err != nil {
		t.Fatalf("reading what the conversation kept: %v", err)
	}
	return kept
}

// lastActivityOf reads when the conversation last moved.
func lastActivityOf(t *testing.T, p *Plugin, conversationID uuid.UUID) time.Time {
	t.Helper()
	var lastActive time.Time
	if err := p.pool.QueryRow(t.Context(),
		"SELECT last_activity_at FROM plugin_whatsapp.conversations WHERE id = $1", conversationID,
	).Scan(&lastActive); err != nil {
		t.Fatalf("reading the last activity: %v", err)
	}
	return lastActive
}

// datedNumber is one stored number under a chosen age.
type datedNumber struct {
	tenantID  uuid.UUID
	number    string
	createdAt time.Time
}

// seedDatedNumbers stores each number under its age, its own id standing in for its token.
func seedDatedNumbers(t *testing.T, p *Plugin, numbers ...datedNumber) {
	t.Helper()
	for _, held := range numbers {
		if _, err := p.pool.Exec(t.Context(), insertDatedCredentials,
			held.tenantID, held.number, []byte(held.number), held.createdAt); err != nil {
			t.Fatalf("seeding %s: %v", held.number, err)
		}
	}
}

// numberRowState says what a stored number holds beside its token.
type numberRowState struct {
	label        string
	hasAppSecret bool
	hasDigest    bool
	hasAge       bool
}

// numberStateOf reads what the stored number holds beside its token.
func numberStateOf(t *testing.T, p *Plugin, number string) numberRowState {
	t.Helper()
	var label string
	var appSecret, digest []byte
	var createdAt time.Time
	if err := p.pool.QueryRow(t.Context(),
		"SELECT label, app_secret, verify_token_digest, created_at FROM plugin_whatsapp.credentials"+
			" WHERE phone_number_id = $1", number).Scan(&label, &appSecret, &digest, &createdAt); err != nil {
		t.Fatalf("reading the stored number: %v", err)
	}
	return numberRowState{
		label: label, hasAppSecret: appSecret != nil, hasDigest: digest != nil, hasAge: !createdAt.IsZero(),
	}
}

// runNumbersMigrationDown steps the plugin back to the version before many numbers.
func runNumbersMigrationDown(t *testing.T, steps *goose.Provider) {
	t.Helper()
	if _, err := steps.DownTo(t.Context(), versionBeforeNumbers); err != nil {
		t.Fatalf("DownTo(%d) error = %v, want nil", versionBeforeNumbers, err)
	}
}

// holdsAnyOf reports whether got holds one of the names.
func holdsAnyOf(got []string, names ...string) bool {
	return slices.ContainsFunc(names, func(name string) bool { return slices.Contains(got, name) })
}

// storedNumberOf returns the number a conversation came in on, nil for none.
func storedNumberOf(t *testing.T, p *Plugin, conversationID uuid.UUID) *string {
	t.Helper()
	var number *string
	if err := p.pool.QueryRow(t.Context(),
		"SELECT phone_number_id FROM plugin_whatsapp.conversations WHERE id = $1", conversationID,
	).Scan(&number); err != nil {
		t.Fatalf("reading the number of the conversation: %v", err)
	}
	return number
}

// columnsOf returns the column names of a plugin table.
func columnsOf(t *testing.T, p *Plugin, table string) []string {
	t.Helper()
	return collectStrings(t, p, "SELECT column_name::text FROM information_schema.columns"+
		" WHERE table_schema = 'plugin_whatsapp' AND table_name = '"+table+"'")
}

// isUniqueViolation reports whether err says Postgres refused a duplicate key.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// addressOf returns a pointer to number, for a nullable column.
func addressOf(number string) *string {
	return &number
}

func TestTheNumbersMigrationAdoptsTheOneNumberATenantHeld(t *testing.T) {
	t.Parallel()

	p, steps := pluginBeforeNumbers(t)
	acme, bare := seededTenant(t, p, "Acme"), seededTenant(t, p, "Globex")
	if _, err := p.pool.Exec(t.Context(), insertOldCredentials, acme, "5550001", []byte("encrypted")); err != nil {
		t.Fatalf("seeding the credentials: %v", err)
	}
	mine := seededConversation(t, p, "184467235", acme)
	theirs := seededConversation(t, p, "184467235", bare)

	if _, err := steps.Up(t.Context()); err != nil {
		t.Fatalf("Up() error = %v, want nil", err)
	}

	if number := storedNumberOf(t, p, mine); number == nil || *number != "5550001" {
		t.Errorf("the conversation of a tenant with a stored number reads %v, want 5550001", number)
	}
	if number := storedNumberOf(t, p, theirs); number != nil {
		t.Errorf("the conversation of a tenant with no stored number reads %q, want none", *number)
	}
	if got := numberStateOf(t, p, "5550001"); got != (numberRowState{hasAge: true}) {
		t.Errorf("the row from before = %+v, want no label, no secrets and an age", got)
	}
}

func TestAStoredNumberStaysUniqueAcrossTheInstall(t *testing.T) {
	t.Parallel()

	p, _ := pluginWithNumbers(t)
	acme, elsewhere := seededTenant(t, p, "Acme"), seededTenant(t, p, "Globex")
	if _, err := p.pool.Exec(t.Context(), insertOldCredentials, acme, "5550001", []byte("encrypted")); err != nil {
		t.Fatalf("the first number: %v", err)
	}
	if _, err := p.pool.Exec(t.Context(), insertOldCredentials, acme, "5550002", []byte("encrypted")); err != nil {
		t.Errorf("a second number in one tenant: %v, want it admitted", err)
	}

	_, err := p.pool.Exec(t.Context(), insertOldCredentials, elsewhere, "5550001", []byte("encrypted"))

	if !isUniqueViolation(err) {
		t.Errorf("the same number in another tenant error = %v, want the key to refuse it", err)
	}
}

func TestAConversationIsUniquePerTenantNumberAndCustomer(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tests := map[string]struct {
		first, second *string
		wantRefused   bool
	}{
		"two numbers":            {first: addressOf("5550001"), second: addressOf("5550002")},
		"a number and no number": {first: addressOf("5550001"), second: nil},
		"one number twice":       {first: addressOf("5550001"), second: addressOf("5550001"), wantRefused: true},
		"no number twice":        {first: nil, second: nil, wantRefused: true},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p, _ := pluginWithNumbers(t)
			acme := seededTenant(t, p, "Acme")
			contactID := seededContact(t, p, acme)
			datedConversation(t, p, conversationSeed{
				tenantID: acme, contactID: contactID, number: test.first, createdAt: now, activeAt: now,
			})

			_, err := p.pool.Exec(t.Context(), insertDatedConversation,
				uuid.Must(uuid.NewV7()), contactID, "184467235", now, now, acme, test.second)

			if test.wantRefused && !isUniqueViolation(err) {
				t.Errorf("the second conversation error = %v, want the key to refuse it", err)
			}
			if !test.wantRefused && err != nil {
				t.Errorf("the second conversation error = %v, want it admitted", err)
			}
		})
	}
}

func TestATenantKeepsItsOldestNumberWhenTheNumbersMigrationRunsDown(t *testing.T) {
	t.Parallel()

	p, steps := pluginWithNumbers(t)
	acme, single := seededTenant(t, p, "Acme"), seededTenant(t, p, "Globex")
	now := time.Now().UTC()
	seedDatedNumbers(t, p,
		datedNumber{tenantID: acme, number: "5550001", createdAt: now.Add(-time.Hour)},
		datedNumber{tenantID: acme, number: "5550002", createdAt: now.Add(-2 * time.Hour)},
		datedNumber{tenantID: single, number: "5550003", createdAt: now},
	)

	runNumbersMigrationDown(t, steps)

	held := collectStrings(t, p, "SELECT phone_number_id FROM plugin_whatsapp.credentials ORDER BY phone_number_id")
	if want := []string{"5550002", "5550003"}; !slices.Equal(held, want) {
		t.Errorf("the numbers left = %v, want each tenant's oldest %v", held, want)
	}
	_, err := p.pool.Exec(t.Context(), insertOldCredentials, acme, "5550004", []byte("encrypted"))
	if !isUniqueViolation(err) {
		t.Errorf("a second number in one tenant error = %v, want the old key to refuse it", err)
	}
	got := columnsOf(t, p, "credentials")
	if holdsAnyOf(got, "label", "app_secret", "verify_token_digest", "created_at") {
		t.Errorf("the columns left = %v, want the ones from before", got)
	}
}

func TestACustomerKeepsEveryMessageWhenTheNumbersMigrationRunsDown(t *testing.T) {
	t.Parallel()

	p, steps := pluginWithNumbers(t)
	acme, elsewhere := seededTenant(t, p, "Acme"), seededTenant(t, p, "Globex")
	contactID := seededContact(t, p, acme)
	now := time.Now().UTC().Truncate(time.Millisecond)
	keeper, withMedia := threeConversations(t, p, conversationSeed{tenantID: acme, contactID: contactID}, now)
	apart := datedConversation(t, p, conversationSeed{
		tenantID: elsewhere, contactID: seededContact(t, p, elsewhere), createdAt: now, activeAt: now,
	})
	oneMessage(t, p, apart, "wamid.apart")

	runNumbersMigrationDown(t, steps)

	if got := tableCount(t, p, "plugin_whatsapp.conversations"); got != 2 {
		t.Fatalf("conversations left = %d, want one per tenant", got)
	}
	if got := keptOn(t, p, keeper.id, withMedia); got != (keptRows{messages: 3, media: 1}) {
		t.Errorf("the conversation kept %d messages and %d media, want 3 and 1", got.messages, got.media)
	}
	lastActive := lastActivityOf(t, p, keeper.id)
	if want := now.Add(-time.Hour); !lastActive.Equal(want) {
		t.Errorf("last activity = %v, want the newest of the merged conversations %v", lastActive, want)
	}
	if got := columnsOf(t, p, "conversations"); slices.Contains(got, "phone_number_id") {
		t.Errorf("the columns left = %v, want the number gone", got)
	}
	_, err := p.pool.Exec(t.Context(),
		"INSERT INTO plugin_whatsapp.conversations"+
			" (id, contact_id, channel, external_id, status, last_activity_at, created_at, tenant_id)"+
			" VALUES ($1, $2, 'whatsapp', '184467235', 'open', now(), now(), $3)",
		uuid.Must(uuid.NewV7()), contactID, acme)
	if !isUniqueViolation(err) {
		t.Errorf("a second conversation with one customer error = %v, want the old key to refuse it", err)
	}
	if _, err := steps.Up(t.Context()); err != nil {
		t.Errorf("Up() after the way down error = %v, want nil", err)
	}
}
