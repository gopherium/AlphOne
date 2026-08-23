// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
)

var errEntropy = errors.New("entropy source failed")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropy
}

func testPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return count
}

func demoCounts(t *testing.T, pool *pgxpool.Pool) [7]int {
	t.Helper()
	return [7]int{
		countRows(t, pool, "core.contacts"),
		countRows(t, pool, "core.tasks"),
		countRows(t, pool, "plugin_whatsapp.conversations"),
		countRows(t, pool, "plugin_whatsapp.messages"),
		countRows(t, pool, "plugin_whatsapp.media"),
		countRows(t, pool, "plugin_importer.imports"),
		countRows(t, pool, "plugin_importer.import_rows"),
	}
}

func TestSeedPopulatesTheDemoData(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	var stdout strings.Builder

	if err := seed(t.Context(), getenv, &stdout); err != nil {
		t.Fatalf("seed() error = %v, want nil", err)
	}

	if !strings.Contains(stdout.String(), "admin@example.com / password1234") {
		t.Errorf("output = %q, want it to print the demo credentials", stdout.String())
	}
	pool := testPool(t, databaseURL)
	admin, err := authkitpg.NewUserStore(pool).UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the seeded admin", err)
	}
	if !gouncer.VerifyPassword(admin.PasswordHash, "password1234") {
		t.Error("stored password hash does not verify against the demo password")
	}
	if got, want := demoCounts(t, pool), [7]int{7, 6, 3, 8, 1, 1, 6}; got != want {
		t.Errorf("demo counts = %v, want %v", got, want)
	}
	var adas int
	err = pool.QueryRow(t.Context(),
		"SELECT count(*) FROM core.contacts WHERE name = 'Ada Lovelace'").Scan(&adas)
	if err != nil || adas != 1 {
		t.Errorf("Ada Lovelace contacts = %d (err %v), want 1", adas, err)
	}
}

func TestSeedStandsAMemberBesideTheAdmin(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	var stdout strings.Builder

	if err := seed(t.Context(), getenv, &stdout); err != nil {
		t.Fatalf("seed() error = %v, want nil", err)
	}

	pool := testPool(t, databaseURL)
	member, err := authkitpg.NewUserStore(pool).UserByEmail(t.Context(), seedMemberEmail)
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the seeded member", err)
	}
	if tier := role.Of(member.Role); tier != role.Member {
		t.Errorf("the seeded colleague stands in %v, want %v", tier, role.Member)
	}
	if !strings.Contains(stdout.String(), seedMemberEmail) {
		t.Errorf("output = %q, want it to name the member the demo can sign in as", stdout.String())
	}
}

func TestSeedGivesTheMemberADayOfItsOwn(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err != nil {
		t.Fatalf("seed() error = %v, want nil", err)
	}

	pool := testPool(t, databaseURL)
	member, err := authkitpg.NewUserStore(pool).UserByEmail(t.Context(), seedMemberEmail)
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the seeded member", err)
	}
	var held int
	if err := pool.QueryRow(t.Context(),
		"SELECT count(*) FROM core.tasks WHERE assignee_id = $1", member.ID).Scan(&held); err != nil {
		t.Fatalf("counting the member's tasks: %v", err)
	}
	if held == 0 {
		t.Error("the seeded member holds no task, want a day the demo can actually show")
	}
}

func TestSeedNamesEveryLoginItCreates(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	pool := testPool(t, databaseURL)
	if _, err := authkit.EnsureAdmin(t.Context(), authkitpg.NewUserStore(pool),
		seedAdminEmail, seedAdminName, seedAdminPassword, role.Admin.String()); err != nil {
		t.Fatalf("seeding the admin ahead of the run: %v", err)
	}
	var stdout strings.Builder

	if err := seed(t.Context(), getenv, &stdout); err != nil {
		t.Fatalf("seed() error = %v, want nil", err)
	}

	if !strings.Contains(stdout.String(), seedMemberEmail) {
		t.Errorf("output = %q, want the member named even though the admin already existed",
			stdout.String())
	}
}

func TestSeedMigratesEveryPluginSchema(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err != nil {
		t.Fatalf("seed() error = %v, want nil", err)
	}

	pool := testPool(t, databaseURL)
	for _, table := range []string{"plugin_whatsapp.conversations", "plugin_importer.imports"} {
		var exists bool
		if err := pool.QueryRow(t.Context(),
			"SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil {
			t.Fatalf("looking for %s: %v", table, err)
		}
		if !exists {
			t.Errorf("%s is missing, want the host to migrate every plugin", table)
		}
	}
}

func TestSeedStoresADayOfTasks(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	if err := seed(t.Context(), getenv, &strings.Builder{}); err != nil {
		t.Fatalf("seed() error = %v, want nil", err)
	}
	pool := testPool(t, databaseURL)
	today := time.Now().UTC().Format("2006-01-02")

	rows, err := pool.Query(t.Context(), `
		SELECT t.title, t.status, t.priority, to_char(t.due_on, 'YYYY-MM-DD'),
			t.assignee_id, coalesce(c.name, ''), coalesce(t.origin_source, '')
		FROM core.tasks t
		LEFT JOIN core.contacts c ON c.id = t.contact_id
		ORDER BY t.due_on, t.title`)
	if err != nil {
		t.Fatalf("querying tasks: %v", err)
	}
	defer rows.Close()

	admin, err := authkitpg.NewUserStore(pool).UserByEmail(t.Context(), seedAdminEmail)
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the seeded admin", err)
	}
	var overdue, dueToday, done, linked, withOrigin int
	for rows.Next() {
		var title, status, dueOn, contactName, origin string
		var priority int
		var assignee uuid.UUID
		if err := rows.Scan(&title, &status, &priority, &dueOn, &assignee, &contactName, &origin); err != nil {
			t.Fatalf("scanning task: %v", err)
		}
		if title == "Draft the welcome email" {
			if assignee == admin.ID {
				t.Errorf("task %q assignee = the admin, want the seeded colleague", title)
			}
		} else if assignee != admin.ID {
			t.Errorf("task %q assignee = %v, want the seeded admin %v", title, assignee, admin.ID)
		}
		switch {
		case dueOn < today && status == "open":
			overdue++
		case dueOn == today && status == "done":
			done++
		case dueOn == today:
			dueToday++
		}
		if contactName == "Ada Lovelace" {
			linked++
		}
		if origin != "" {
			withOrigin++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading tasks: %v", err)
	}
	if overdue != 1 || done != 1 || dueToday != 3 {
		t.Errorf("overdue = %d, done = %d, due today = %d, want 1, 1, 3", overdue, done, dueToday)
	}
	if linked != 1 {
		t.Errorf("tasks linked to Ada Lovelace = %d, want 1", linked)
	}
	if withOrigin != 1 {
		t.Errorf("tasks carrying an origin = %d, want 1", withOrigin)
	}
}

func TestSeedRaisesOneTaskAboveTheRest(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	if err := seed(t.Context(), getenv, &strings.Builder{}); err != nil {
		t.Fatalf("seed() error = %v, want nil", err)
	}
	pool := testPool(t, databaseURL)

	var high int
	err := pool.QueryRow(t.Context(),
		"SELECT count(*) FROM core.tasks WHERE priority > 0").Scan(&high)

	if err != nil || high != 1 {
		t.Errorf("high priority tasks = %d (err %v), want 1", high, err)
	}
}

func TestSeedReportsBrokenTaskStorage(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	if err := postgres.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE core.tasks ADD CONSTRAINT seed_sabotage CHECK (false)"); err != nil {
		t.Fatalf("breaking the tasks table: %v", err)
	}

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want a task storage failure")
	}
}

func TestSeedTasksReportsAdminLookupFailure(t *testing.T) {
	t.Parallel()

	users := testkit.NewStore()
	users.LookupErr = errors.New("store down")
	store := postgres.NewTaskStore(testPool(t, testDatabaseURL(t)))

	err := seedTasks(t.Context(), store, users, uuid.Must(uuid.NewV7()))

	if err == nil {
		t.Fatal("seedTasks() error = nil, want an admin lookup failure")
	}
}

func TestSeedTasksReportsLookupFailure(t *testing.T) {
	t.Parallel()

	users := testkit.NewStore()
	users.AddUser(t, seedAdminEmail, seedAdminName, seedAdminPassword)
	users.AddUser(t, seedMemberEmail, seedMemberName, seedAdminPassword)
	pool := testPool(t, testDatabaseURL(t))
	store := postgres.NewTaskStore(pool)
	pool.Close()

	err := seedTasks(t.Context(), store, users, uuid.Must(uuid.NewV7()))

	if err == nil {
		t.Fatal("seedTasks() error = nil, want a lookup failure")
	}
}

func TestSeedTasksReportsAColleagueLookupFailure(t *testing.T) {
	t.Parallel()

	users := testkit.NewStore()
	users.AddUser(t, seedAdminEmail, seedAdminName, seedAdminPassword)
	store := postgres.NewTaskStore(testPool(t, testDatabaseURL(t)))

	err := seedTasks(t.Context(), store, users, uuid.Must(uuid.NewV7()))

	if err == nil {
		t.Fatal("seedTasks() error = nil, want the missing colleague reported")
	}
	if !strings.Contains(err.Error(), "member") {
		t.Errorf("error = %v, want it to name the colleague it could not find", err)
	}
}

func TestSeedTasksReportsIDGenerationFailure(t *testing.T) {
	users := testkit.NewStore()
	users.AddUser(t, seedAdminEmail, seedAdminName, seedAdminPassword)
	users.AddUser(t, seedMemberEmail, seedMemberName, seedAdminPassword)
	store := postgres.NewTaskStore(testPool(t, testDatabaseURL(t)))
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	err := seedTasks(t.Context(), store, users, uuid.Nil)

	if !errors.Is(err, errEntropy) {
		t.Fatalf("seedTasks() error = %v, want the entropy failure in its chain", err)
	}
}

func TestSeedIsIdempotentAcrossRuns(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err != nil {
		t.Fatalf("first seed() error = %v, want nil", err)
	}
	var second strings.Builder
	if err := seed(t.Context(), getenv, &second); err != nil {
		t.Fatalf("second seed() error = %v, want nil", err)
	}

	pool := testPool(t, databaseURL)
	if got, want := demoCounts(t, pool), [7]int{7, 6, 3, 8, 1, 1, 6}; got != want {
		t.Errorf("demo counts after two runs = %v, want %v", got, want)
	}
	if !strings.Contains(second.String(), "admin@example.com already exists") {
		t.Errorf("second output = %q, want it to report the existing admin", second.String())
	}
	if strings.Contains(second.String(), "password1234") {
		t.Errorf("second output = %q, want it to not repeat the demo password", second.String())
	}
}

func TestSeedValidatesItsInput(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]string{
		"missing database url":   nil,
		"malformed database url": {"ALPHONE_DATABASE_URL": "not a url \x00"},
		"unreachable database":   {"ALPHONE_DATABASE_URL": unreachableDatabaseURL},
	}

	for testName, env := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			if err := seed(t.Context(), testGetenv(env), &strings.Builder{}); err == nil {
				t.Fatal("seed() error = nil, want a failure")
			}
		})
	}
}

func TestSeedReportsCoreMigrationFailure(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(), "ALTER TABLE goose_db_version DROP COLUMN version_id"); err != nil {
		t.Fatalf("breaking the core migration table: %v", err)
	}
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want a core migration failure")
	}
}

func TestSeedReportsInvalidPluginConfiguration(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":             testDatabaseURL(t),
		"ALPHONE_WHATSAPP_MEDIA_MAX_BYTES": "not a number",
	})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want a plugin configuration failure")
	}
}

func TestSeedReportsBrokenContactStorage(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	if err := seed(t.Context(), getenv, &strings.Builder{}); err != nil {
		t.Fatalf("first seed() error = %v, want nil", err)
	}
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(), "DROP TABLE core.contact_identities"); err != nil {
		t.Fatalf("dropping the identities table: %v", err)
	}

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want a contact storage failure")
	}
}

func TestSeedReportsTheColleagueItCannotStore(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	pool := testPool(t, databaseURL)
	if _, err := authkit.EnsureAdmin(t.Context(), authkitpg.NewUserStore(pool),
		seedAdminEmail, seedAdminName, seedAdminPassword, role.Admin.String()); err != nil {
		t.Fatalf("seeding the admin: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE auth.users ADD CONSTRAINT seed_sabotage CHECK (email <> '"+
			seedMemberEmail+"')"); err != nil {
		t.Fatalf("refusing the colleague: %v", err)
	}

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want the unstored colleague reported")
	}
}

func TestSeedReportsAdminStorageFailure(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	if err := authkitpg.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("migrating the auth schema: %v", err)
	}
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE auth.users ADD CONSTRAINT seed_sabotage CHECK (false)"); err != nil {
		t.Fatalf("breaking the users table: %v", err)
	}
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want an admin storage failure")
	}
}

func TestSeedReportsAnUnstorableAccount(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE auth.users ADD CONSTRAINT seed_sabotage CHECK (false)"); err != nil {
		t.Fatalf("breaking the users table: %v", err)
	}
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want the unstored account reported")
	}
}

func TestSeedPluginsReportsMigrationFailure(t *testing.T) {
	t.Parallel()

	resolver := contact.NewResolver(postgres.NewContactStore(testPool(t, unreachableDatabaseURL)))

	err := seedPlugins(t.Context(), unreachableDatabaseURL, testGetenv(nil), resolver)

	if err == nil {
		t.Fatal("seedPlugins() error = nil, want a migration failure")
	}
}

func TestSeedPluginsReportsSeedFailure(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	resolver := contact.NewResolver(postgres.NewContactStore(testPool(t, unreachableDatabaseURL)))

	err := seedPlugins(t.Context(), databaseURL, testGetenv(nil), resolver)

	if err == nil {
		t.Fatal("seedPlugins() error = nil, want a seed failure")
	}
}
