// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"
	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
	authtest "github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

var errEntropy = errors.New("entropy source failed")

type failingReader struct{}

// Read returns the entropy failure instead of any bytes.
func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropy
}

// testPool returns a pool on the given database URL, closed when the test ends.
func testPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// countRows returns the number of rows in the named table.
func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return count
}

// demoCounts returns the row counts of the seven tables the demo data fills.
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

// developmentWarning is the line seed -yes prints on stderr once every seed succeeded.
const developmentWarning = "alphone: demo data is for development only, never seed a production database\n"

// errSeedRefused is the failure of a plugin that refuses its demo data.
var errSeedRefused = errors.New("demo data refused")

// seedRefusingPlugin refuses to store its demo data.
type seedRefusingPlugin struct{ inertPlugin }

// Seed refuses the demo data.
func (seedRefusingPlugin) Seed(context.Context) error {
	return errSeedRefused
}

// compiledProgram returns the command line over env and the compiled plugins, their roles in a registry of its own.
func compiledProgram(env map[string]string) gonsole.Program {
	return programOver(role.NewRegistry(), testkit.Getenv(env), registerPlugins)
}

var _ sdk.Seeder = (*stepRecordingPlugin)(nil)

// stepRecordingPlugin records every step the host asks of it, in order.
type stepRecordingPlugin struct {
	inertPlugin
	asked []string
}

// Start records the start.
func (p *stepRecordingPlugin) Start(context.Context) error {
	p.asked = append(p.asked, "start")
	return nil
}

// Seed records the seed.
func (p *stepRecordingPlugin) Seed(context.Context) error {
	p.asked = append(p.asked, "seed")
	return nil
}

// Stop records the stop.
func (p *stepRecordingPlugin) Stop(context.Context) error {
	p.asked = append(p.asked, "stop")
	return nil
}

func TestSeedYesStoresThePluginDemoDataWithoutStartingAPlugin(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	recording := &stepRecordingPlugin{inertPlugin: inertPlugin{id: "recording"}}
	registeringOne := func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{recording}, nil
	}
	env := map[string]string{"ALPHONE_DATABASE_URL": databaseURL}

	got := testkit.Run(t, programOver(role.NewRegistry(), testkit.Getenv(env), registeringOne), "", "seed", "-yes")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("seed -yes = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	if want := []string{"seed", "stop"}; !slices.Equal(recording.asked, want) {
		t.Errorf("the plugin was asked %v, want %v, its demo data stored and the plugin stopped, never started",
			recording.asked, want)
	}
	isPlugin := func(schema string) bool { return strings.HasPrefix(schema, "plugin_") }
	if schemas := extraSchemas(t, databaseURL); slices.ContainsFunc(schemas, isPlugin) {
		t.Errorf("the database holds the schemas %v, want no plugin started beside the one the command line registered",
			schemas)
	}
}

func TestSeedYesPopulatesTheDemoData(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)

	got := testkit.Run(t, compiledProgram(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}), "", "seed", "-yes")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("seed -yes = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	if !strings.Contains(got.Stdout, "admin@example.com / password1234") {
		t.Errorf("output = %q, want it to print the demo credentials", got.Stdout)
	}
	pool := testPool(t, databaseURL)
	admin, err := authkitpg.NewUserStore(pool).UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the seeded admin", err)
	}
	if !gouncer.VerifyPassword(admin.PasswordHash, "password1234") {
		t.Error("stored password hash does not verify against the demo password")
	}
	if got, want := demoCounts(t, pool), [7]int{71, 7, 3, 8, 1, 1, 6}; got != want {
		t.Errorf("demo counts = %v, want %v", got, want)
	}
	var adas int
	err = pool.QueryRow(t.Context(),
		"SELECT count(*) FROM core.contacts WHERE name = 'Ada Lovelace'").Scan(&adas)
	if err != nil || adas != 1 {
		t.Errorf("Ada Lovelace contacts = %d (err %v), want 1", adas, err)
	}
}

func TestSeedYesFillsSeveralContactPagesOnSeveralChannels(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)

	got := testkit.Run(t, compiledProgram(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}), "", "seed", "-yes")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("seed -yes = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	pool := testPool(t, databaseURL)
	if held := countRows(t, pool, "core.contacts"); held < 3*graphres.DefaultListPageSize {
		t.Errorf("contacts = %d, want at least three pages of %d", held, graphres.DefaultListPageSize)
	}
	var channels int
	if err := pool.QueryRow(t.Context(),
		"SELECT count(DISTINCT channel) FROM core.contact_identities").Scan(&channels); err != nil {
		t.Fatalf("counting the channels: %v", err)
	}
	if channels < 3 {
		t.Errorf("channels = %d, want contacts reachable on email, phone and whatsapp", channels)
	}
}

func TestSeedStandsAMemberBesideTheAdmin(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
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

func TestSeedShowsEveryAccountStatusOnce(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	for range 2 {
		if err := seed(t.Context(), getenv, &strings.Builder{}); err != nil {
			t.Fatalf("seed() error = %v, want nil", err)
		}
	}

	pool := testPool(t, databaseURL)
	users := authkitpg.NewUserStore(pool)
	tests := map[string]struct {
		email     string
		confirmed bool
		disabled  bool
	}{
		"active":   {email: seedAdminEmail, confirmed: true},
		"invited":  {email: "invited@example.com"},
		"disabled": {email: "disabled@example.com", disabled: true},
	}
	for status, want := range tests {
		held, err := users.UserByEmail(t.Context(), want.email)
		if err != nil {
			t.Errorf("%s account %s: UserByEmail() error = %v, want it seeded", status, want.email, err)
			continue
		}
		if held.Confirmed != want.confirmed || held.Disabled != want.disabled {
			t.Errorf("%s account confirmed %v, disabled %v, want %v, %v",
				status, held.Confirmed, held.Disabled, want.confirmed, want.disabled)
		}
	}
	if held := countRows(t, pool, "auth.users"); held != 4 {
		t.Errorf("accounts after two runs = %d, want 4", held)
	}
}

func TestSeedReportsAStatusAccountItCannotStore(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	if err := authkitpg.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("migrating the auth schema: %v", err)
	}
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE auth.users ADD CONSTRAINT seed_sabotage CHECK (email <> 'invited@example.com')"); err != nil {
		t.Fatalf("refusing the invited account: %v", err)
	}
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	err := seed(t.Context(), getenv, &strings.Builder{})

	if err == nil || !strings.Contains(err.Error(), "invited@example.com") {
		t.Fatalf("seed() error = %v, want the unstored invited account reported", err)
	}
}

func TestSeedStatusesReportsAnAccountItCannotBuild(t *testing.T) {
	t.Parallel()

	err := seedStatuses(t.Context(), authtest.NewStore(), []demoStatus{{email: "not an address", name: "Ana Lopez"}})

	if err == nil || !strings.Contains(err.Error(), "not an address") {
		t.Fatalf("seedStatuses() error = %v, want the malformed account reported", err)
	}
}

func TestSeedGivesTheMemberADayOfItsOwn(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

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
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
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

func TestSeedLeavesTheSchemaToTheCommandLine(t *testing.T) {
	t.Parallel()

	databaseURL := barePostgres(t)

	err := seed(t.Context(), testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}), io.Discard)

	if schemas := extraSchemas(t, databaseURL); err == nil || len(schemas) > 0 {
		t.Errorf("seed() over a bare database = %v with the schemas %v, want a failure and no schema", err, schemas)
	}
}

func TestSeedYesWarnsOnStderrAfterEverySeedSucceeded(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		plugins  func(sdk.Deps) ([]sdk.Plugin, error)
		code     int
		warnings int
		last     string
	}{
		"every seed succeeds": {
			plugins: registeringNothing, code: gonsole.ExitDone, warnings: 1, last: developmentWarning,
		},
		"a plugin refuses its demo data": {
			plugins: func(sdk.Deps) ([]sdk.Plugin, error) {
				return []sdk.Plugin{seedRefusingPlugin{inertPlugin{id: "refusing"}}}, nil
			},
			code: gonsole.ExitFailed, warnings: 0, last: errSeedRefused.Error() + "\n",
		},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			env := map[string]string{"ALPHONE_DATABASE_URL": testDatabaseURL(t)}

			got := testkit.Run(t, programOver(role.NewRegistry(), testkit.Getenv(env), tc.plugins), "", "seed", "-yes")

			if got.Code != tc.code || !strings.HasSuffix(got.Stderr, tc.last) ||
				strings.Count(got.Stdout+got.Stderr, "development only") != tc.warnings {
				t.Errorf("seed -yes = %d, stdout %q, stderr %q, want %d, %d development warnings and stderr ending %q",
					got.Code, got.Stdout, got.Stderr, tc.code, tc.warnings, tc.last)
			}
		})
	}
}

func TestSeedYesMigratesEveryPluginSchema(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)

	got := testkit.Run(t, compiledProgram(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}), "", "seed", "-yes")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("seed -yes = %d with stderr %q, want 0", got.Code, got.Stderr)
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
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
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

func TestSeedLinksATaskDueInThreeDaysToTheHistoryContact(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	if err := seed(t.Context(), getenv, &strings.Builder{}); err != nil {
		t.Fatalf("seed() error = %v, want nil", err)
	}
	pool := testPool(t, databaseURL)

	var dueOn string
	err := pool.QueryRow(t.Context(), `
		SELECT to_char(t.due_on, 'YYYY-MM-DD')
		FROM core.tasks t
		JOIN core.contacts c ON c.id = t.contact_id
		WHERE c.name = 'Maria Perez'`).Scan(&dueOn)

	want := time.Now().UTC().AddDate(0, 0, 3).Format(time.DateOnly)
	if err != nil || dueOn != want {
		t.Errorf("the history contact's task is due %q (err %v), want %q", dueOn, err, want)
	}
}

func TestSeedRaisesOneTaskAboveTheRest(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
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
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
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

	users := authtest.NewStore()
	users.LookupErr = errors.New("store down")
	store := postgres.NewTaskStore(testPool(t, testDatabaseURL(t)))

	err := seedTasks(t.Context(), store, users, nil)

	if err == nil {
		t.Fatal("seedTasks() error = nil, want an admin lookup failure")
	}
}

func TestSeedTasksReportsLookupFailure(t *testing.T) {
	t.Parallel()

	users := authtest.NewStore()
	users.AddUser(t, seedAdminEmail, seedAdminName, seedAdminPassword)
	users.AddUser(t, seedMemberEmail, seedMemberName, seedAdminPassword)
	pool := testPool(t, testDatabaseURL(t))
	store := postgres.NewTaskStore(pool)
	pool.Close()

	err := seedTasks(t.Context(), store, users, nil)

	if err == nil {
		t.Fatal("seedTasks() error = nil, want a lookup failure")
	}
}

func TestSeedTasksReportsAColleagueLookupFailure(t *testing.T) {
	t.Parallel()

	users := authtest.NewStore()
	users.AddUser(t, seedAdminEmail, seedAdminName, seedAdminPassword)
	store := postgres.NewTaskStore(testPool(t, testDatabaseURL(t)))

	err := seedTasks(t.Context(), store, users, nil)

	if err == nil {
		t.Fatal("seedTasks() error = nil, want the missing colleague reported")
	}
	if !strings.Contains(err.Error(), "member") {
		t.Errorf("error = %v, want it to name the colleague it could not find", err)
	}
}

func TestSeedTasksReportsIDGenerationFailure(t *testing.T) {
	users := authtest.NewStore()
	users.AddUser(t, seedAdminEmail, seedAdminName, seedAdminPassword)
	users.AddUser(t, seedMemberEmail, seedMemberName, seedAdminPassword)
	store := postgres.NewTaskStore(testPool(t, testDatabaseURL(t)))
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	err := seedTasks(t.Context(), store, users, nil)

	if !errors.Is(err, errEntropy) {
		t.Fatalf("seedTasks() error = %v, want the entropy failure in its chain", err)
	}
}

func TestSeedYesIsIdempotentAcrossRuns(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	env := map[string]string{"ALPHONE_DATABASE_URL": databaseURL}

	if first := testkit.Run(t, compiledProgram(env), "", "seed", "-yes"); first.Code != gonsole.ExitDone {
		t.Fatalf("first seed -yes = %d with stderr %q, want 0", first.Code, first.Stderr)
	}
	second := testkit.Run(t, compiledProgram(env), "", "seed", "-yes")
	if second.Code != gonsole.ExitDone {
		t.Fatalf("second seed -yes = %d with stderr %q, want 0", second.Code, second.Stderr)
	}

	pool := testPool(t, databaseURL)
	if got, want := demoCounts(t, pool), [7]int{71, 7, 3, 8, 1, 1, 6}; got != want {
		t.Errorf("demo counts after two runs = %v, want %v", got, want)
	}
	if !strings.Contains(second.Stdout, "admin@example.com already exists") {
		t.Errorf("second output = %q, want it to report the existing admin", second.Stdout)
	}
	if strings.Contains(second.Stdout, "password1234") {
		t.Errorf("second output = %q, want it to not repeat the demo password", second.Stdout)
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

			if err := seed(t.Context(), testkit.Getenv(env), &strings.Builder{}); err == nil {
				t.Fatal("seed() error = nil, want a failure")
			}
		})
	}
}

func TestSeedYesReportsACoreMigrationFailure(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(), "ALTER TABLE goose_db_version DROP COLUMN version_id"); err != nil {
		t.Fatalf("breaking the core migration table: %v", err)
	}

	got := testkit.Run(t, bareProgram(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}), "", "seed", "-yes")

	if got.Code != gonsole.ExitFailed || !strings.Contains(got.Stderr, "migrate core") {
		t.Errorf("seed -yes over a broken core migration table = %d with stderr %q, want 1 and the core step named",
			got.Code, got.Stderr)
	}
}

func TestSeedYesReportsInvalidPluginConfiguration(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ALPHONE_DATABASE_URL":             testDatabaseURL(t),
		"ALPHONE_WHATSAPP_MEDIA_MAX_BYTES": "not a number",
	}

	got := testkit.Run(t, compiledProgram(env), "", "seed", "-yes")

	if got.Code != gonsole.ExitFailed || !strings.Contains(got.Stderr, "ALPHONE_WHATSAPP_MEDIA_MAX_BYTES") {
		t.Errorf("seed -yes = %d with stderr %q, want 1 and the plugin setting named", got.Code, got.Stderr)
	}
}

func TestSeedReportsBrokenContactStorage(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
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

func TestSeedReportsTheHistoryContactItCannotStore(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	if err := postgres.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE core.contacts ADD CONSTRAINT seed_sabotage CHECK (name <> 'Maria Perez')"); err != nil {
		t.Fatalf("refusing the history contact: %v", err)
	}

	err := seed(t.Context(), getenv, &strings.Builder{})

	if err == nil || !strings.Contains(err.Error(), "seed contact") {
		t.Fatalf("seed() error = %v, want the unstored history contact reported", err)
	}
	if adas := countRows(t, pool, "core.contacts"); adas != 1 {
		t.Errorf("contacts = %d, want Ada Lovelace stored before the refused one", adas)
	}
}

func TestSeedReportsTheColleagueItCannotStore(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
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
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

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
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want the unstored account reported")
	}
}
