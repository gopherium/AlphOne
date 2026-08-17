// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
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
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/plugins/fields"
	"github.com/gopherium/alphone/plugins/importer"
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
	t            *testing.T
	pool         *pgxpool.Pool
	tasks        *postgres.TaskStore
	contacts     *postgres.ContactStore
	resolver     *contact.Resolver
	lastContact  uuid.UUID
	lastImport   uuid.UUID
	users        *authkitpg.UserStore
	server       *httptest.Server
	ownerID      uuid.UUID
	secret       string
	tokenID      uuid.UUID
	session      *mcp.ClientSession
	connErr      error
	called       *mcp.CallToolResult
	captured     []byte
	answered     []byte
	lastField    uuid.UUID
	altSecret    string
	scopedSecret string
	scopedID     uuid.UUID
	sessionValue string
	status       int
}

// newWorld boots an isolated database and a real server for one scenario.
func newWorld(t *testing.T) *world {
	t.Helper()
	return bootWorld(t, false)
}

// newImportWorld boots a scenario whose importer runs against the real database.
func newImportWorld(t *testing.T) *world {
	t.Helper()
	return bootWorld(t, true)
}

// bootWorld boots one scenario, running the importer live only when asked.
func bootWorld(t *testing.T, liveImports bool) *world {
	t.Helper()
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.CoreMigrator())
	if err := authkitpg.Migrate(context.Background(), cfg.URL()); err != nil {
		t.Fatalf("migrating the auth schema: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), sparingly(cfg.URL()))
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

	resolver := contact.NewResolver(contacts)
	fieldsPlugin := livePlugin(t, cfg.URL())
	registered := append(inertPlugins(t), fieldsPlugin)
	if liveImports {
		importerPlugin := liveImporter(t, cfg.URL(), scenarioDirectory{resolver: resolver})
		importerPlugin.UseFieldProviders([]sdk.FieldProvider{fieldsPlugin})
		registered = append(registered, importerPlugin)
	}
	root, err := graphroot.FromPlugins(&graphres.Resolver{
		Version:      "test",
		Contacts:     contacts,
		Tasks:        tasks,
		Webhooks:     webhooks,
		Tenants:      postgres.NewTenantStore(pool),
		Tokens:       tokens,
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
	minted, err := apitoken.Mint(owner.ID, "mcp scenario", apitoken.Full(), apitoken.Never)
	if err != nil {
		t.Fatalf("minting the token: %v", err)
	}
	if err := tokens.Create(context.Background(), minted.Token); err != nil {
		t.Fatalf("storing the token: %v", err)
	}

	srv := httptest.NewServer(server.NewServer(server.Config{
		Users:        users,
		Auth:         auth,
		GraphRoot:    root,
		Tokens:       tokens,
		FieldSources: []sdk.FieldSource{fieldsPlugin},
		Version:      "test",
	}))
	t.Cleanup(srv.Close)

	return &world{
		t:        t,
		pool:     pool,
		tasks:    tasks,
		contacts: contacts,
		resolver: resolver,
		users:    users,
		server:   srv,
		ownerID:  owner.ID,
		secret:   minted.Secret,
		tokenID:  minted.Token.ID,
	}
}

// sparingly returns the database URL with a pool small enough for a whole suite.
func sparingly(databaseURL string) string {
	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}
	return databaseURL + separator + "pool_max_conns=2"
}

// livePlugin registers the fields plugin against the scenario's own database.
func livePlugin(t *testing.T, databaseURL string) *fields.Plugin {
	t.Helper()
	plugin, err := fields.Register(sdk.Deps{DatabaseURL: sparingly(databaseURL)})
	if err != nil {
		t.Fatalf("registering fields: %v", err)
	}
	t.Cleanup(func() { _ = plugin.Stop(context.Background()) })
	if err := plugin.Migrate(context.Background()); err != nil {
		t.Fatalf("migrating fields: %v", err)
	}
	if err := plugin.Start(context.Background()); err != nil {
		t.Fatalf("starting fields: %v", err)
	}
	return plugin
}

// liveImporter registers the importer plugin against the scenario's own database.
func liveImporter(t *testing.T, databaseURL string, directory sdk.ContactDirectory) *importer.Plugin {
	t.Helper()
	plugin, err := importer.Register(sdk.Deps{
		DatabaseURL: sparingly(databaseURL), Contacts: directory,
	})
	if err != nil {
		t.Fatalf("registering importer: %v", err)
	}
	t.Cleanup(func() { _ = plugin.Stop(context.Background()) })
	if err := plugin.Migrate(context.Background()); err != nil {
		t.Fatalf("migrating importer: %v", err)
	}
	if err := plugin.Start(context.Background()); err != nil {
		t.Fatalf("starting importer: %v", err)
	}
	return plugin
}

// invalidContactErrors lists the domain errors a plugin reads as unusable details.
var invalidContactErrors = []error{
	contact.ErrEmptyName,
	contact.ErrEmptyChannel,
	contact.ErrEmptyIdentifier,
	contact.ErrChannelNotWritable,
}

// markInvalid returns err with unusable contact details marked for the plugin.
func markInvalid(err error) error {
	for _, invalid := range invalidContactErrors {
		if errors.Is(err, invalid) {
			return fmt.Errorf("%w: %w", sdk.ErrInvalidContact, err)
		}
	}
	return err
}

// scenarioDirectory bridges the scenario's contact resolver to the plugin contract.
type scenarioDirectory struct {
	resolver *contact.Resolver
}

// FindByIdentity returns the [sdk.Contact] owning an identity, reporting whether one exists.
func (d scenarioDirectory) FindByIdentity(
	ctx context.Context, channel sdk.Channel, identifier string,
) (sdk.Contact, bool, error) {
	owner, found, err := d.resolver.FindByIdentity(ctx, contact.Channel(channel), identifier)
	if err != nil || !found {
		return sdk.Contact{}, false, markInvalid(err)
	}
	return sdk.Contact{ID: owner.ID, Name: owner.Name}, true, nil
}

// CreateWithIdentities stores an [sdk.Contact] owning every identity, reporting whether it was created.
func (d scenarioDirectory) CreateWithIdentities(
	ctx context.Context, name string, identities []sdk.Identity,
) (sdk.Contact, bool, error) {
	addresses := make([]contact.Address, 0, len(identities))
	for _, identity := range identities {
		addresses = append(addresses, contact.Address{
			Channel:     contact.Channel(identity.Channel),
			Identifier:  identity.Identifier,
			DisplayName: identity.DisplayName,
		})
	}
	owner, created, err := d.resolver.CreateWithIdentities(ctx, name, addresses)
	if err != nil {
		return sdk.Contact{}, false, markInvalid(err)
	}
	return sdk.Contact{ID: owner.ID, Name: owner.Name}, created, nil
}

// worldFrom returns the world the scenario carries.
func worldFrom(ctx context.Context) *world {
	return ctx.Value(worldKey{}).(*world)
}

// inertPlugins registers every graph contributing plugin against a lazy pool.
func inertPlugins(t *testing.T) []sdk.Plugin {
	t.Helper()
	registered, err := graphroot.All(sdk.Deps{DatabaseURL: "postgres://plugin:plugin@localhost:1/plugin"})
	if err != nil {
		t.Fatalf("registering the inert plugins: %v", err)
	}
	for _, plugin := range registered {
		t.Cleanup(func() { _ = plugin.Stop(context.Background()) })
	}
	return registered
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

// seedContact stores a contact and remembers it for the steps that follow.
func (w *world) seedContact(ctx context.Context, name string) (uuid.UUID, error) {
	created, err := contact.New(name)
	if err != nil {
		return uuid.Nil, fmt.Errorf("building the contact: %w", err)
	}
	if err := w.contacts.Create(ctx, created); err != nil {
		return uuid.Nil, fmt.Errorf("storing the contact: %w", err)
	}
	w.lastContact = created.ID
	return created.ID, nil
}

// seedTenant stores a tenant and returns its id.
func (w *world) seedTenant(ctx context.Context, name string) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generating the tenant id: %w", err)
	}
	if _, err := w.pool.Exec(ctx,
		"INSERT INTO core.tenants (id, name, created_at) VALUES ($1, $2, now())", id, name); err != nil {
		return uuid.Nil, fmt.Errorf("storing the tenant: %w", err)
	}
	return id, nil
}

// placeMember upserts one user's membership into the named tenant.
func (w *world) placeMember(ctx context.Context, userID uuid.UUID, tenantName string) error {
	tag, err := w.pool.Exec(ctx,
		`INSERT INTO core.tenant_members (user_id, tenant_id, created_at)
		SELECT $1, id, now() FROM core.tenants WHERE name = $2
		ON CONFLICT (user_id) DO UPDATE SET tenant_id = EXCLUDED.tenant_id`,
		userID, tenantName)
	if err != nil {
		return fmt.Errorf("placing the member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no tenant named %q holds a row", tenantName)
	}
	return nil
}

// mintSecretFor mints and stores an API token for the given user.
func (w *world) mintSecretFor(ctx context.Context, userID uuid.UUID, name string) (string, error) {
	return w.mintScopedSecretFor(ctx, userID, name, apitoken.Full())
}

// mintScopedSecretFor mints and stores an API token granted the given scopes.
func (w *world) mintScopedSecretFor(
	ctx context.Context, userID uuid.UUID, name string, scopes apitoken.Scopes,
) (string, error) {
	minted, err := apitoken.Mint(userID, name, scopes, apitoken.Never)
	if err != nil {
		return "", fmt.Errorf("minting the token: %w", err)
	}
	w.scopedID = minted.Token.ID
	if err := postgres.NewTokenStore(w.pool).Create(ctx, minted.Token); err != nil {
		return "", fmt.Errorf("storing the token: %w", err)
	}
	return minted.Secret, nil
}

// expireSecret ends the life of the token behind secret an hour ago.
func (w *world) expireSecret(ctx context.Context, secret string) error {
	tag, err := w.pool.Exec(ctx,
		"UPDATE core.api_tokens SET expires_at = now() - interval '1 hour' WHERE token_hash = $1",
		apitoken.HashSecret(secret))
	if err != nil {
		return fmt.Errorf("expiring the token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no token is stored under the given secret")
	}
	return nil
}

// seedReachableContact stores a contact owning an email identity.
func (w *world) seedReachableContact(ctx context.Context, name, email string) error {
	created, _, err := w.resolver.CreateWithIdentities(ctx, name, []contact.Address{
		{Channel: contact.Channel("email"), Identifier: email},
	})
	if err != nil {
		return fmt.Errorf("storing the reachable contact: %w", err)
	}
	w.lastContact = created.ID
	return nil
}
