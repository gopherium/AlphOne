// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/gouncer/authkit/ratelimit"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// ownerEmail addresses the user every scenario signs in as.
const ownerEmail = "owner@example.com"

// ownerName names the user every scenario signs in as.
const ownerName = "Maria Perez"

// ownerPassword is the password the seeded user holds.
const ownerPassword = "correct horse battery"

// worldKey carries the scenario world through the godog context.
type worldKey struct{}

// world holds everything one scenario needs, torn down when it ends.
type world struct {
	t       *testing.T
	pool    *pgxpool.Pool
	tasks   *postgres.TaskStore
	users   *authkitpg.UserStore
	server  *httptest.Server
	ownerID uuid.UUID
	secret  string
	tokenID uuid.UUID
	session *mcp.ClientSession
	connErr error
	called  *mcp.CallToolResult
}

// newWorld boots an isolated database and a real server for one scenario.
func newWorld(t *testing.T) *world {
	t.Helper()
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.CoreMigrator())
	if err := authkitpg.Migrate(context.Background(), cfg.URL()); err != nil {
		t.Fatalf("migrating the auth schema: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), cfg.URL())
	if err != nil {
		t.Fatalf("connecting the scenario pool: %v", err)
	}
	t.Cleanup(pool.Close)

	users := authkitpg.NewUserStore(pool)
	contacts := postgres.NewContactStore(pool)
	tasks := postgres.NewTaskStore(pool)
	tokens := postgres.NewTokenStore(pool)
	webhooks := postgres.NewWebhookStore(pool)
	hub := event.NewHub()
	auth := authkit.New(authkit.Config{Store: users, CookieName: server.SessionCookieName})

	registered := inertPlugins(t)
	root, err := graphroot.FromPlugins(&graphres.Resolver{
		Version:      "test",
		Contacts:     contacts,
		Tasks:        tasks,
		Webhooks:     webhooks,
		Live:         hub,
		Auth:         auth,
		Admin:        authkit.NewAdmin(users),
		LoginLimiter: ratelimit.NewLimiter(ratelimit.Config{}),
	}, registered)
	if err != nil {
		t.Fatalf("composing the graph root: %v", err)
	}
	if _, err := authkit.EnsureAdmin(context.Background(), users, ownerEmail, ownerName, ownerPassword); err != nil {
		t.Fatalf("seeding the owner: %v", err)
	}
	owner, err := users.UserByEmail(context.Background(), ownerEmail)
	if err != nil {
		t.Fatalf("reading the owner: %v", err)
	}
	minted, err := apitoken.Mint(owner.ID, "mcp scenario")
	if err != nil {
		t.Fatalf("minting the token: %v", err)
	}
	if err := tokens.Create(context.Background(), minted.Token); err != nil {
		t.Fatalf("storing the token: %v", err)
	}

	srv := httptest.NewServer(server.NewServer(server.Config{
		Users:     users,
		Auth:      auth,
		GraphRoot: root,
		Tokens:    tokens,
		Version:   "test",
	}))
	t.Cleanup(srv.Close)

	return &world{
		t:       t,
		pool:    pool,
		tasks:   tasks,
		users:   users,
		server:  srv,
		ownerID: owner.ID,
		secret:  minted.Secret,
		tokenID: minted.Token.ID,
	}
}

// worldFrom returns the world the scenario carries.
func worldFrom(ctx context.Context) *world {
	return ctx.Value(worldKey{}).(*world)
}

// inertPlugins registers every graph contributing plugin against a lazy pool.
func inertPlugins(t *testing.T) []sdk.Plugin {
	t.Helper()
	const unreachable = "postgres://plugin:plugin@localhost:1/plugin"
	whatsappPlugin, err := whatsapp.Register(sdk.Deps{DatabaseURL: unreachable})
	if err != nil {
		t.Fatalf("registering whatsapp: %v", err)
	}
	t.Cleanup(func() { _ = whatsappPlugin.Stop(context.Background()) })
	importerPlugin, err := importer.Register(sdk.Deps{DatabaseURL: unreachable})
	if err != nil {
		t.Fatalf("registering importer: %v", err)
	}
	t.Cleanup(func() { _ = importerPlugin.Stop(context.Background()) })
	return []sdk.Plugin{whatsappPlugin, importerPlugin}
}

// addUser stores another user and returns its id.
func (w *world) addUser(ctx context.Context, email, name string) (uuid.UUID, error) {
	if _, err := authkit.EnsureAdmin(ctx, w.users, email, name, ownerPassword); err != nil {
		return uuid.Nil, fmt.Errorf("seeding %s: %w", email, err)
	}
	stored, err := w.users.UserByEmail(ctx, email)
	if err != nil {
		return uuid.Nil, fmt.Errorf("reading %s: %w", email, err)
	}
	return stored.ID, nil
}
