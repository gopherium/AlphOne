// SPDX-License-Identifier: Elastic-2.0

package testdb_test

import (
	"database/sql"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/testdb"
)

const (
	// addressKey names the variable that holds the address of the test database server.
	addressKey = "ALPHONE_TEST_DATABASE_URL"
	// maxAgeKey names the variable that holds the age past which a stale test database is swept.
	maxAgeKey = "ALPHONE_TEST_DATABASE_MAX_AGE"
	// where is the hint every refused setting gives about where to set it.
	where = "in the environment or .env, see .env.example"
)

// fatal is a test that records its fatal message and ends its goroutine on a fatal call.
type fatal struct {
	testing.TB
	// message is the text of the fatal call that ended the goroutine.
	message string
}

// Fatal records args and ends the calling goroutine.
func (f *fatal) Fatal(args ...any) {
	f.message = fmt.Sprint(args...)
	runtime.Goexit()
}

// Fatalf records the message of format and args and ends the calling goroutine.
func (f *fatal) Fatalf(format string, args ...any) {
	f.message = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// fatalOfNew returns the fatal message New stopped with, or nothing when it returned.
func fatalOfNew(t *testing.T) string {
	t.Helper()
	recorder := &fatal{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		testdb.New(recorder, testdb.Migrator())
	}()
	<-done
	return recorder.message
}

// panicOfConfig returns what Config panicked with, or nothing when it returned.
func panicOfConfig() (recovered any) {
	defer func() { recovered = recover() }()
	testdb.Config()
	return nil
}

// migrated returns a database carrying every schema the migrator applies.
func migrated(t *testing.T) *sql.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	db, err := sql.Open("pgx", testdb.New(t, testdb.Migrator()))
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// tableExists reports whether the named table stands in the database.
func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var present bool
	if err := db.QueryRow("SELECT to_regclass($1) IS NOT NULL", name).Scan(&present); err != nil {
		t.Fatalf("asking for %s: %v", name, err)
	}
	return present
}

func TestNewNamesAMissingAddressAndWhereToSetIt(t *testing.T) {
	t.Setenv(addressKey, "")
	t.Setenv(maxAgeKey, "1h")

	message := fatalOfNew(t)
	if !strings.Contains(message, addressKey) || !strings.Contains(message, where) {
		t.Errorf("New() stopped with %q, want a message naming %s and %q", message, addressKey, where)
	}
}

func TestNewNamesABadMaxAgeAndWhereToSetIt(t *testing.T) {
	t.Setenv(addressKey, "postgres://postgres@db.example.com/postgres")
	t.Setenv(maxAgeKey, "0s")

	message := fatalOfNew(t)
	if !strings.Contains(message, maxAgeKey) || !strings.Contains(message, where) {
		t.Errorf("New() stopped with %q, want a message naming %s and %q", message, maxAgeKey, where)
	}
}

func TestNewShowsTheRefusalOfAMalformedAddress(t *testing.T) {
	t.Setenv(addressKey, "mysql://postgres@db.example.com/postgres")
	t.Setenv(maxAgeKey, "1h")

	if message := fatalOfNew(t); !strings.HasPrefix(message, "dbkit: refused database address") {
		t.Errorf("New() stopped with %q, want the dbkit refusal of the address", message)
	}
}

func TestConfigFollowsTheTestDatabaseAddress(t *testing.T) {
	t.Setenv(addressKey, "postgres://tester:secret@db.example.com:6543/postgres?sslmode=disable")
	t.Setenv(maxAgeKey, "1h")

	want := pgtestdb.Config{
		DriverName: "pgx",
		Host:       "db.example.com",
		Port:       "6543",
		User:       "tester",
		Password:   "secret",
		Database:   "postgres",
		Options:    "sslmode=disable",
	}
	if got := testdb.Config(); got != want {
		t.Errorf("Config() = %+v, want %+v", got, want)
	}
}

func TestConfigKeepsTheEscapesOfTheAddress(t *testing.T) {
	t.Setenv(addressKey, "postgres://tester:p%2Fss%25w%23rd@db.example.com:6543/postgres?sslmode=disable")
	t.Setenv(maxAgeKey, "1h")

	parsed, err := url.Parse(testdb.Config().URL())
	if err != nil {
		t.Fatalf("parsing Config().URL(): %v", err)
	}
	if password, _ := parsed.User.Password(); password != "p/ss%w#rd" {
		t.Errorf("Config().URL() password = %q, want p/ss%%w#rd", password)
	}
}

func TestConfigNeedsNoMaxAge(t *testing.T) {
	t.Setenv(addressKey, "postgres://tester:secret@db.example.com:6543/postgres?sslmode=disable")
	t.Setenv(maxAgeKey, "")

	if recovered := panicOfConfig(); recovered != nil {
		t.Errorf("Config() panicked with %v, want the address alone to be enough", recovered)
	}
}

func TestConfigPanicsNamingAMissingSetting(t *testing.T) {
	t.Setenv(addressKey, "")
	t.Setenv(maxAgeKey, "1h")

	recovered := fmt.Sprint(panicOfConfig())
	if !strings.Contains(recovered, addressKey) || !strings.Contains(recovered, where) {
		t.Errorf("Config() panicked with %q, want a message naming %s and %q", recovered, addressKey, where)
	}
}

func TestConfigPanicsOnAnAddressItCannotRead(t *testing.T) {
	t.Setenv(addressKey, "postgres://tester:secret@db.example.com:port/postgres")
	t.Setenv(maxAgeKey, "1h")

	recovered := fmt.Sprint(panicOfConfig())
	if !strings.Contains(recovered, addressKey) || strings.Contains(recovered, "secret") {
		t.Errorf("Config() panicked with %q, want a message naming %s that keeps the password out", recovered, addressKey)
	}
}

func TestMigratorBuildsBothSchemas(t *testing.T) {
	t.Parallel()

	db := migrated(t)

	if !tableExists(t, db, "auth.users") {
		t.Error("auth.users is missing, the auth schema must migrate before core")
	}
	if !tableExists(t, db, "core.contacts") {
		t.Error("core.contacts is missing, the core schema must migrate")
	}
}

func TestMigratorLetsCoreMigrationsReachTheAuthSchema(t *testing.T) {
	t.Parallel()

	db := migrated(t)

	var users int
	if err := db.QueryRow("SELECT count(*) FROM auth.users").Scan(&users); err != nil {
		t.Fatalf("a core migration could not read auth.users: %v", err)
	}
}

func TestMigratorHashesBothMigrationSets(t *testing.T) {
	t.Parallel()

	hash, err := testdb.Migrator().Hash()

	if err != nil {
		t.Fatalf("Hash() error = %v, want nil", err)
	}
	if hash == "" {
		t.Error("Hash() is empty, want a template identity")
	}
	core, err := testdb.CoreMigrator().Hash()
	if err != nil {
		t.Fatalf("CoreMigrator().Hash() error = %v, want nil", err)
	}
	if hash == core {
		t.Error("Hash() matches the core only hash, want the auth migrations counted too")
	}
}
