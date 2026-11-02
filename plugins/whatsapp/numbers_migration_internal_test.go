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

// datedConversation stores one conversation with a customer on the number, nil for no number.
func datedConversation(
	t *testing.T, p *Plugin, tenantID, contactID uuid.UUID, number *string, createdAt, activeAt time.Time,
) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(), insertDatedConversation,
		id, contactID, "184467235", activeAt, createdAt, tenantID, number); err != nil {
		t.Fatalf("seeding the conversation: %v", err)
	}
	return id
}

// oneMessage stores one inbound message on the conversation and returns its id.
func oneMessage(t *testing.T, p *Plugin, tenantID, conversationID uuid.UUID, wamid string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(), insertOneMessage, id, conversationID, wamid, tenantID); err != nil {
		t.Fatalf("seeding the message: %v", err)
	}
	return id
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
	var label string
	var appSecret, digest []byte
	var createdAt time.Time
	if err := p.pool.QueryRow(t.Context(),
		"SELECT label, app_secret, verify_token_digest, created_at FROM plugin_whatsapp.credentials"+
			" WHERE phone_number_id = '5550001'").Scan(&label, &appSecret, &digest, &createdAt); err != nil {
		t.Fatalf("reading the stored number: %v", err)
	}
	if label != "" || appSecret != nil || digest != nil || createdAt.IsZero() {
		t.Errorf("the row from before = (%q, %v, %v, %v), want no label, no secrets and an age",
			label, appSecret, digest, createdAt)
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
			datedConversation(t, p, acme, contactID, test.first, now, now)

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
	for number, age := range map[string]time.Duration{"5550001": time.Hour, "5550002": 2 * time.Hour} {
		createdAt := now.Add(-age)
		if _, err := p.pool.Exec(t.Context(), insertDatedCredentials, acme, number, []byte(number), createdAt); err != nil {
			t.Fatalf("seeding %s: %v", number, err)
		}
	}
	if _, err := p.pool.Exec(t.Context(), insertDatedCredentials, single, "5550003", []byte("5550003"), now); err != nil {
		t.Fatalf("seeding the single number: %v", err)
	}

	if _, err := steps.DownTo(t.Context(), versionBeforeNumbers); err != nil {
		t.Fatalf("DownTo(%d) error = %v, want nil", versionBeforeNumbers, err)
	}

	held := collectStrings(t, p, "SELECT phone_number_id FROM plugin_whatsapp.credentials ORDER BY phone_number_id")
	if want := []string{"5550002", "5550003"}; !slices.Equal(held, want) {
		t.Errorf("the numbers left = %v, want each tenant's oldest %v", held, want)
	}
	_, err := p.pool.Exec(t.Context(), insertOldCredentials, acme, "5550004", []byte("encrypted"))
	if !isUniqueViolation(err) {
		t.Errorf("a second number in one tenant error = %v, want the old key to refuse it", err)
	}
	if got := columnsOf(t, p, "credentials"); slices.Contains(got, "label") || slices.Contains(got, "app_secret") ||
		slices.Contains(got, "verify_token_digest") || slices.Contains(got, "created_at") {
		t.Errorf("the columns left = %v, want the ones from before", got)
	}
}

func TestACustomerKeepsEveryMessageWhenTheNumbersMigrationRunsDown(t *testing.T) {
	t.Parallel()

	p, steps := pluginWithNumbers(t)
	acme, elsewhere := seededTenant(t, p, "Acme"), seededTenant(t, p, "Globex")
	contactID := seededContact(t, p, acme)
	now := time.Now().UTC().Truncate(time.Millisecond)
	keeper := datedConversation(t, p, acme, contactID, nil, now.Add(-3*time.Hour), now.Add(-3*time.Hour))
	onFirst := datedConversation(t, p, acme, contactID, addressOf("5550001"), now.Add(-2*time.Hour), now.Add(-time.Hour))
	onSecond := datedConversation(t, p, acme, contactID, addressOf("5550002"), now.Add(-time.Hour), now.Add(-2*time.Hour))
	apart := datedConversation(t, p, elsewhere, seededContact(t, p, elsewhere), nil, now, now)
	oneMessage(t, p, acme, keeper, "wamid.none")
	oneMessage(t, p, acme, onFirst, "wamid.first")
	withMedia := oneMessage(t, p, acme, onSecond, "wamid.second")
	oneMessage(t, p, elsewhere, apart, "wamid.apart")
	ctx := sdk.WithTenant(t.Context(), acme)
	descriptor := mediaDescriptor{mediaID: "MEDIA1", mimeType: "image/png"}
	if err := insertMediaPending(ctx, p.pool, withMedia, descriptor); err != nil {
		t.Fatalf("seeding the media: %v", err)
	}

	if _, err := steps.DownTo(t.Context(), versionBeforeNumbers); err != nil {
		t.Fatalf("DownTo(%d) error = %v, want nil", versionBeforeNumbers, err)
	}

	if got := tableCount(t, p, "plugin_whatsapp.conversations"); got != 2 {
		t.Fatalf("conversations left = %d, want one per tenant", got)
	}
	var keptMessages, keptMedia int
	if err := p.pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM plugin_whatsapp.messages WHERE conversation_id = $1),
			(SELECT count(*) FROM plugin_whatsapp.media WHERE message_id = $2)`,
		keeper, withMedia).Scan(&keptMessages, &keptMedia); err != nil {
		t.Fatalf("reading what the conversation kept: %v", err)
	}
	if keptMessages != 3 || keptMedia != 1 {
		t.Errorf("the conversation kept %d messages and %d media, want 3 and 1", keptMessages, keptMedia)
	}
	var lastActive time.Time
	if err := p.pool.QueryRow(t.Context(),
		"SELECT last_activity_at FROM plugin_whatsapp.conversations WHERE id = $1", keeper).Scan(&lastActive); err != nil {
		t.Fatalf("reading the last activity: %v", err)
	}
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
