// SPDX-License-Identifier: Elastic-2.0

// Package testdb reads the test settings and provides fresh PostgreSQL databases on the server they name.
package testdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/goosemigrator"

	"github.com/gopherium/framework/dbkit/postgres/pgtest"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/postgres"
)

const (
	// authVersionTable is where the auth schema records its own migrations.
	authVersionTable = "auth.goose_db_version"
	// addressKey names the variable that holds the address of the test database server.
	addressKey = "ALPHONE_TEST_DATABASE_URL"
	// maxAgeKey names the variable that holds the age past which a stale test database is swept.
	maxAgeKey = "ALPHONE_TEST_DATABASE_MAX_AGE"
	// whereToSet tells the reader of a refused setting where to set it.
	whereToSet = "in the environment or .env, see .env.example"
)

// rootDotenv reads the .env entries of the module the test process started in, once per process.
var rootDotenv = dotenvOnce(os.Getwd())

// New returns the address of a fresh database that migrator builds on the test server, after a sweep of stale ones.
func New(t testing.TB, migrator pgtestdb.Migrator) string {
	t.Helper()
	address, maxAge, err := settings(os.LookupEnv, rootDotenv)
	if err != nil {
		t.Fatal(err)
	}
	return pgtest.NewSwept(t, address, maxAge, migrator)
}

// Setting returns the test setting key, read from the environment first and from .env second, failing t when unset.
func Setting(t testing.TB, key string) string {
	t.Helper()
	text, err := required(os.LookupEnv, rootDotenv, key)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// Config returns the pgtestdb configuration of the test server, and panics when its address is missing or unreadable.
func Config() pgtestdb.Config {
	address, err := addressOf(os.LookupEnv, rootDotenv)
	if err != nil {
		panic(err)
	}
	server, err := url.Parse(address)
	if err != nil {
		panic(fmt.Errorf("testdb: set %s to a URL, %s", addressKey, whereToSet))
	}
	user, password, _ := strings.Cut(server.User.String(), ":")
	return pgtestdb.Config{
		DriverName: "pgx",
		Host:       server.Hostname(),
		Port:       server.Port(),
		User:       user,
		Password:   password,
		Database:   strings.TrimPrefix(server.EscapedPath(), "/"),
		Options:    server.RawQuery,
	}
}

// settings returns the test server address and the sweep age, each read from environ first and from file second.
func settings(
	environ func(string) (string, bool), file func() (map[string]string, error),
) (string, time.Duration, error) {
	address, err := addressOf(environ, file)
	if err != nil {
		return "", 0, err
	}
	text, err := value(environ, file, maxAgeKey)
	if err != nil {
		return "", 0, err
	}
	maxAge, err := time.ParseDuration(text)
	if err != nil || maxAge <= 0 {
		return "", 0, fmt.Errorf("testdb: set %s to a Go duration above zero, such as 1h, %s, got %q",
			maxAgeKey, whereToSet, text)
	}
	return address, maxAge, nil
}

// addressOf returns the test server address, read from environ first and from file second.
func addressOf(environ func(string) (string, bool), file func() (map[string]string, error)) (string, error) {
	return required(environ, file, addressKey)
}

// required returns the trimmed value of key, read from environ first and from file second, refusing an empty one.
func required(
	environ func(string) (string, bool), file func() (map[string]string, error), key string,
) (string, error) {
	text, err := value(environ, file, key)
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("testdb: set %s %s", key, whereToSet)
	}
	return text, nil
}

// value returns the trimmed value of key from environ, or from file when environ does not set it.
func value(environ func(string) (string, bool), file func() (map[string]string, error), key string) (string, error) {
	if text, ok := environ(key); ok {
		return strings.TrimSpace(text), nil
	}
	entries, err := file()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(entries[key]), nil
}

// dotenvOnce returns a reader of the .env entries above dir that reads the file on its first call only.
func dotenvOnce(dir string, dirErr error) func() (map[string]string, error) {
	return sync.OnceValues(func() (map[string]string, error) {
		if dirErr != nil {
			return nil, fmt.Errorf("testdb: find the working directory: %w", dirErr)
		}
		return dotenv(dir)
	})
}

// dotenv returns the entries of the .env file beside the nearest go.mod at or above dir, or none without one.
func dotenv(dir string) (map[string]string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dotenvAt(filepath.Join(dir, ".env"))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return map[string]string{}, nil
		}
		dir = parent
	}
}

// dotenvAt returns the entries of the .env file at path, none when it is missing, with no line of it in an error.
func dotenvAt(path string) (map[string]string, error) {
	entries, err := godotenv.Read(path)
	var pathErr *fs.PathError
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return map[string]string{}, nil
	case errors.As(err, &pathErr):
		return nil, fmt.Errorf("testdb: read %s: %w", path, err)
	case err != nil:
		return nil, fmt.Errorf("testdb: %s holds a line that does not read as KEY=value", path)
	}
	return entries, nil
}

// CoreMigrator returns a migrator applying the core schema migrations.
func CoreMigrator() *goosemigrator.GooseMigrator {
	return goosemigrator.New("migrations", goosemigrator.WithFS(postgres.Migrations))
}

// authMigrator returns a migrator applying the auth schema migrations.
func authMigrator() *goosemigrator.GooseMigrator {
	return goosemigrator.New("migrations",
		goosemigrator.WithFS(authkitpg.Migrations),
		goosemigrator.WithTableName(authVersionTable))
}

// schemaMigrator applies the auth schema before the core schema, the order the binary uses.
type schemaMigrator struct{}

// Migrator returns a migrator applying every schema one AlphOne database holds.
func Migrator() pgtestdb.Migrator {
	return schemaMigrator{}
}

// Hash returns the template identity of both migration sets together.
func (schemaMigrator) Hash() (string, error) {
	auth, err := authMigrator().Hash()
	if err != nil {
		return "", fmt.Errorf("testdb: hash the auth migrations: %w", err)
	}
	core, err := CoreMigrator().Hash()
	if err != nil {
		return "", fmt.Errorf("testdb: hash the core migrations: %w", err)
	}
	return auth + core, nil
}

// Migrate applies the auth schema and then the core schema to the template database.
func (schemaMigrator) Migrate(ctx context.Context, db *sql.DB, cfg pgtestdb.Config) error {
	if err := authkitpg.Migrate(ctx, cfg.URL()); err != nil {
		return fmt.Errorf("testdb: migrate the auth schema: %w", err)
	}
	if err := CoreMigrator().Migrate(ctx, db, cfg); err != nil {
		return fmt.Errorf("testdb: migrate the core schema: %w", err)
	}
	return nil
}
