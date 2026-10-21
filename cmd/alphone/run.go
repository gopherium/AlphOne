// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gopherium/framework/goncierge"
	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/pluginkit"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/ratelimit"

	"github.com/gopherium/alphone/graph"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/internal/tenant"
	"github.com/gopherium/alphone/internal/version"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/sdk"
)

// run starts the server and serves until ctx is cancelled or serving fails.
func run(
	ctx context.Context,
	getenv func(string) string,
	stderr io.Writer,
	plugins func(sdk.Deps) ([]sdk.Plugin, error),
) error {
	logger := slog.New(slog.NewTextHandler(stderr, nil))

	settings, err := loadRunConfig(getenv)
	if err != nil {
		return err
	}

	built, err := compose(ctx, composeConfig{
		composeSettings: settings.composeSettings,
		databaseURL:     settings.databaseURL,
		getenv:          getenv,
		roles:           role.NewRegistry(),
		logger:          logger,
	}, plugins)
	if built.failed != nil {
		err = errors.Join(fmt.Errorf("register plugins: %w", built.failed), err)
	}
	if err != nil {
		return errors.Join(err, abandon(ctx, built, settings.serving.StopGrace))
	}
	defer built.pool.Close()
	host := pluginkit.NewHost(built.registered...)
	if err := migrate(ctx, settings.databaseURL); err != nil {
		return errors.Join(err, gonsole.StopHost(ctx, host, settings.serving.StopGrace))
	}

	built.worker.Start()
	defer built.worker.Stop()
	reaper := authkit.NewReaper(built.users, authkit.ReaperConfig{Logger: logger})
	reaper.Start()
	defer reaper.Stop()

	if err := host.Start(ctx, settings.serving.StopGrace); err != nil {
		return fmt.Errorf("start plugins: %w", err)
	}

	auth := authkit.New(authConfig(built))
	graphRoot, err := graphroot.FromPlugins(graphResolver(settings, built, auth, logger), built.registered)
	if err != nil {
		return errors.Join(
			fmt.Errorf("compose graph root: %w", err), gonsole.StopHost(ctx, host, settings.serving.StopGrace))
	}

	serverConfig := serverConfigOf(settings, built, host, auth, graphRoot, logger)
	httpServer := httpServerFrom(settings, server.NewServer(serverConfig))
	return gonsole.Serve(ctx, httpServer, settings.serving, host.Stop, logger)
}

// inviteConfigOf returns the invitation and reset lifetimes the settings name over the store and roles compose built.
func inviteConfigOf(settings runConfig, built composed) authkit.InvitesConfig {
	return authkit.InvitesConfig{
		Store:           built.users,
		InviteTTL:       settings.inviteTTL,
		ResetTTL:        settings.reset.ttl,
		ResetTokensLive: settings.reset.links,
		Roles:           built.roles,
	}
}

// graphResolver returns the core graph resolver over what compose built, the login handlers and the settings.
func graphResolver(settings runConfig, built composed, auth *authkit.Handlers, logger *slog.Logger) *graphres.Resolver {
	invites := inviteConfigOf(settings, built)
	return &graphres.Resolver{
		Version:       version.Version(),
		Contacts:      built.contacts,
		Tasks:         built.tasks,
		Webhooks:      built.webhooks,
		WebhookGuard:  built.webhookGuard,
		Tenants:       built.tenants,
		Tokens:        built.tokens,
		Events:        built.events,
		Live:          built.hub,
		Auth:          auth,
		Admin:         authkit.NewAdmin(adminConfig(built)),
		Invites:       authkit.NewInvites(invites),
		Onboarding:    postgres.NewOnboarding(built.pool, invites),
		Accounts:      built.users,
		Mailer:        built.mailer,
		PublicURL:     settings.mail.publicURL,
		Settings:      postgres.NewUserSettingStore(built.pool),
		LoginLimiter:  ratelimit.NewLimiter(ratelimit.Config{}),
		TokenLimiter:  ratelimit.NewLimiter(ratelimit.Config{}),
		ResetLimiter:  ratelimit.NewLimiter(resetBudget(settings)),
		ResetCooldown: ratelimit.NewLimiter(resetCooldownBudget(settings)),
		Logger:        logger,
		Paging:        settings.paging,
		Screens:       settings.screens,
		Roles:         built.roles,
	}
}

// serverConfigOf returns the server settings over what compose built, the host, the logins, the graph root and logger.
func serverConfigOf(
	settings runConfig, built composed, host *pluginkit.Host, auth *authkit.Handlers, graphRoot graph.ResolverRoot,
	logger *slog.Logger,
) server.Config {
	cfg := settings.serverConfig()
	cfg.Version = version.Version()
	cfg.Users = built.users
	cfg.Tenants = built.tenants
	cfg.Auth = auth
	cfg.GraphRoot = graphRoot
	cfg.Tokens = built.tokens
	cfg.Plugins = host.Routes()
	cfg.PluginPublicPaths = host.PublicPaths()
	cfg.PluginAreas = pluginAreas(built.registered)
	cfg.FieldSources = fieldSources(built.registered)
	cfg.Roles = built.roles
	cfg.Logger = logger
	if settings.webDir != "" {
		cfg.Web = os.DirFS(settings.webDir)
	}
	return cfg
}

// pluginAreas returns the scope area every registered plugin holds its routes to.
func pluginAreas(registered []sdk.Plugin) map[string]string {
	areas := map[string]string{}
	for _, plugin := range registered {
		if named, ok := plugin.(sdk.AreaProvider); ok {
			areas[plugin.ID()] = named.Area()
		}
	}
	return areas
}

// authConfig returns the login configuration the server serves sessions under, over the store and roles compose built.
func authConfig(built composed) authkit.Config {
	return authkit.Config{
		Store:        built.users,
		CookieName:   server.SessionCookieName,
		Privileged:   built.roles.HoldersOf(role.ManageUsers),
		Capabilities: built.roles,
	}
}

// adminConfig returns the administration configuration guarding the privileged cover over the roles compose built.
func adminConfig(built composed) authkit.AdminConfig {
	return authkit.AdminConfig{
		Store:      built.users,
		Privileged: built.roles.HoldersOf(role.ManageUsers),
		Roles:      built.roles,
	}
}

// errReservedID reports a plugin under the id the core grants its roles under.
var errReservedID = errors.New("the id is reserved for the core roles")

// declareRoles declares every role a registered plugin declares, refusing a plugin under the core id.
func declareRoles(registry *goncierge.Registry, registered []sdk.Plugin) error {
	for _, plugin := range registered {
		if plugin.ID() == role.Source {
			return fmt.Errorf("plugin %s: %w", plugin.ID(), errReservedID)
		}
		provider, ok := plugin.(sdk.RoleProvider)
		if !ok {
			continue
		}
		if err := role.Declare(registry, plugin.ID(), provider.Roles()); err != nil {
			return fmt.Errorf("plugin %s: %w", plugin.ID(), err)
		}
	}
	return nil
}

// fieldSources returns every registered plugin serving runtime defined fields.
func fieldSources(registered []sdk.Plugin) []sdk.FieldSource {
	var sources []sdk.FieldSource
	for _, plugin := range registered {
		if source, ok := plugin.(sdk.FieldSource); ok {
			sources = append(sources, source)
		}
	}
	return sources
}

// wireFieldProviders hands every registered field provider to every consumer.
func wireFieldProviders(registered []sdk.Plugin) {
	var providers []sdk.FieldProvider
	for _, plugin := range registered {
		if provider, ok := plugin.(sdk.FieldProvider); ok {
			providers = append(providers, provider)
		}
	}
	for _, plugin := range registered {
		if consumer, ok := plugin.(sdk.FieldConsumer); ok {
			consumer.UseFieldProviders(providers)
		}
	}
}

// wireTenantGate hands the host's tenant gate to every plugin taking one.
func wireTenantGate(registered []sdk.Plugin, gate sdk.TenantGate) {
	for _, plugin := range registered {
		if consumer, ok := plugin.(sdk.TenantGateConsumer); ok {
			consumer.UseTenantGate(gate)
		}
	}
}

// wireCredentialProviders hands every registered credential provider to every consumer.
func wireCredentialProviders(registered []sdk.Plugin) {
	var providers []sdk.CredentialProvider
	for _, plugin := range registered {
		if provider, ok := plugin.(sdk.CredentialProvider); ok {
			providers = append(providers, provider)
		}
	}
	for _, plugin := range registered {
		if consumer, ok := plugin.(sdk.CredentialConsumer); ok {
			consumer.UseCredentialProviders(providers)
		}
	}
}

// runConfig carries the environment-derived settings of the server.
type runConfig struct {
	composeSettings
	databaseURL    string
	addr           string
	webDir         string
	trustedProxies []string
	graphiql       bool
	inviteTTL      time.Duration
	reset          resetSettings
	screens        graphres.Screens
	graph          graphSettings
	serving        gonsole.Timeouts
}

// composeSettings carries the settings compose hands the plugins and the stores it builds.
type composeSettings struct {
	machineGrace time.Duration
	tenants      tenantSettings
	mail         mailSettings
	webhookHosts webhook.AllowList
	paging       graphres.Paging
}

// servingDefaults are the HTTP timeouts and shutdown graces the server runs under when the environment names none.
var servingDefaults = gonsole.Timeouts{
	ReadHeader: 10 * time.Second, Read: 30 * time.Second, Idle: 2 * time.Minute,
	Grace: 10 * time.Second, CancelGrace: 5 * time.Second, StopGrace: 5 * time.Second,
}

// settingsEnv returns the reader of the settings under the program prefix.
func settingsEnv(getenv func(string) string) gonsole.Env {
	return gonsole.Env{Prefix: "ALPHONE_", Getenv: getenv}
}

// httpServerFrom returns the HTTP server for the handler at the address and under the timeouts the settings name.
func httpServerFrom(settings runConfig, handler http.Handler) *http.Server {
	return gonsole.NewServer(settings.addr, handler, settings.serving)
}

// tenantSettings bounds the per-tenant state plugins and the graph keep in memory.
type tenantSettings struct {
	held    int
	refresh time.Duration
}

// serverConfig returns the server settings the run config carries, for run to complete.
func (c runConfig) serverConfig() server.Config {
	return server.Config{
		TrustedProxies:    c.trustedProxies,
		GraphiQL:          c.graphiql,
		TenantsHeld:       c.tenants.held,
		Graph:             c.graph.bounds,
		MaxStreamLifetime: c.graph.streamLifetime,
		MaxStreamsPerUser: c.graph.streamsPerUser,
	}
}

// loadTenantSettings reads how many tenants' state to keep in memory and how long to keep one.
func loadTenantSettings(env gonsole.Env) (tenantSettings, error) {
	held, err := env.Count("TENANTS_HELD", sdk.DefaultTenantsHeld)
	if err != nil {
		return tenantSettings{}, err
	}
	refresh, err := env.Duration("TENANTS_REFRESH", sdk.DefaultTenantsRefresh)
	if err != nil {
		return tenantSettings{}, err
	}
	return tenantSettings{held: held, refresh: refresh}, nil
}

// resetSettings names the lifetime, stack and rate the reset links ride under.
type resetSettings struct {
	ttl      time.Duration
	attempts int
	links    int
	cooldown time.Duration
}

// mailSettings names the relay, sender identity and link base mail rides on.
type mailSettings struct {
	host        string
	port        int
	username    string
	password    string
	from        string
	tls         string
	publicURL   string
	templateDir string
}

// defaultMailPort is the submission port a relay listens on when the environment names none.
const defaultMailPort = 587

// highestPort is the highest TCP port a relay can listen on.
const highestPort = 65535

// parseMailTLS reads the transport security policy.
func parseMailTLS(value string) (string, error) {
	switch value {
	case "mandatory", "opportunistic", "none":
		return value, nil
	}
	return "", fmt.Errorf("must be mandatory, opportunistic or none, got %q", value)
}

// resetCooldownBudget answers the rate limit reset mail to one address rides under.
func resetCooldownBudget(settings runConfig) ratelimit.Config {
	return ratelimit.Config{Limit: 1, Window: settings.reset.cooldown}
}

// resetBudget answers the rate limit reset requests ride under.
func resetBudget(settings runConfig) ratelimit.Config {
	return ratelimit.Config{Limit: settings.reset.attempts, Window: settings.reset.ttl}
}

// defaultResetAttempts caps reset requests per client within one token lifetime.
const defaultResetAttempts = 3

// defaultResetLinks caps the reset links standing for one account at once.
const defaultResetLinks = 3

// defaultResetCooldown spaces the reset mail one address may receive.
const defaultResetCooldown = time.Minute

// parsePublicURL reads the address email links lead back to, without its trailing slash.
func parsePublicURL(value string) (string, error) {
	held, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("must be a URL: %w", err)
	}
	if (held.Scheme != "http" && held.Scheme != "https") || held.Host == "" {
		return "", fmt.Errorf("must be an http or https address, got %q", value)
	}
	if strings.ContainsAny(value, "?#") {
		return "", fmt.Errorf("must carry no query or fragment, got %q", value)
	}
	if escaped := held.EscapedPath(); escaped != "" && escaped != "/" {
		return "", fmt.Errorf("must name a site root, got %q", value)
	}
	return strings.TrimSuffix(value, "/"), nil
}

// loadMailSettings reads the mail relay settings from the environment.
func loadMailSettings(env gonsole.Env) (mailSettings, error) {
	host := env.Value("SMTP_HOST")
	if host == "" {
		for _, name := range []string{"SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_TLS"} {
			if env.Value(name) != "" {
				return mailSettings{}, fmt.Errorf("%s is set but %s is not", env.Key(name), env.Key("SMTP_HOST"))
			}
		}
		return mailSettings{}, nil
	}
	port, err := env.Count("SMTP_PORT", defaultMailPort, gonsole.AtMost(highestPort))
	if err != nil {
		return mailSettings{}, err
	}
	tlsPolicy, err := gonsole.Parse(env, "SMTP_TLS", "mandatory", parseMailTLS)
	if err != nil {
		return mailSettings{}, err
	}
	from := env.Value("SMTP_FROM")
	if from == "" {
		return mailSettings{}, fmt.Errorf("%s is required when %s is set", env.Key("SMTP_FROM"), env.Key("SMTP_HOST"))
	}
	publicURL, err := gonsole.Parse(env, "PUBLIC_URL", "", parsePublicURL)
	if err != nil {
		return mailSettings{}, err
	}
	if publicURL == "" {
		return mailSettings{}, fmt.Errorf("%s is required when %s is set", env.Key("PUBLIC_URL"), env.Key("SMTP_HOST"))
	}
	return mailSettings{
		host:        host,
		port:        port,
		username:    env.Value("SMTP_USERNAME"),
		password:    env.Value("SMTP_PASSWORD"),
		from:        from,
		tls:         tlsPolicy,
		publicURL:   publicURL,
		templateDir: env.Value("MAIL_TEMPLATE_DIR"),
	}, nil
}

// loadComposeSettings reads the settings compose hands the plugins and the stores it builds.
func loadComposeSettings(env gonsole.Env) (composeSettings, error) {
	machineGrace, err := env.Duration("TENANT_MACHINE_GRACE", tenant.DefaultMachineGrace, gonsole.AllowZero())
	if err != nil {
		return composeSettings{}, err
	}
	tenants, err := loadTenantSettings(env)
	if err != nil {
		return composeSettings{}, err
	}
	mail, err := loadMailSettings(env)
	if err != nil {
		return composeSettings{}, err
	}
	webhookHosts, err := gonsole.Parse(env, "WEBHOOK_ALLOWED_HOSTS", webhook.AllowList{}, webhook.ParseAllowList)
	if err != nil {
		return composeSettings{}, err
	}
	paging, err := loadPaging(env)
	if err != nil {
		return composeSettings{}, err
	}
	return composeSettings{
		machineGrace: machineGrace, tenants: tenants, mail: mail, webhookHosts: webhookHosts, paging: paging,
	}, nil
}

// loadRunConfig reads the server settings from the environment.
func loadRunConfig(getenv func(string) string) (runConfig, error) {
	env := settingsEnv(getenv)
	databaseURL, err := env.Required("DATABASE_URL")
	if err != nil {
		return runConfig{}, err
	}
	trustedProxies, err := gonsole.Parse(env, "TRUSTED_PROXIES", nil, ratelimit.ParseTrustedProxies)
	if err != nil {
		return runConfig{}, err
	}
	shared, err := loadComposeSettings(env)
	if err != nil {
		return runConfig{}, err
	}
	inviteTTL, err := env.Duration("INVITE_TTL", authkit.DefaultInviteTTL)
	if err != nil {
		return runConfig{}, err
	}
	reset, err := loadResetSettings(env)
	if err != nil {
		return runConfig{}, err
	}
	screens, err := loadScreens(env, shared.paging.Cap)
	if err != nil {
		return runConfig{}, err
	}
	graph, err := loadGraphSettings(env)
	if err != nil {
		return runConfig{}, err
	}
	serving, err := env.Timeouts(servingDefaults)
	if err != nil {
		return runConfig{}, err
	}
	return runConfig{
		composeSettings: shared,
		databaseURL:     databaseURL,
		addr:            valueOr(env, "ADDR", "localhost:8080"),
		webDir:          env.Value("WEB_DIR"),
		trustedProxies:  trustedProxies,
		graphiql:        env.Value("DEV_GRAPHIQL") != "",
		inviteTTL:       inviteTTL,
		reset:           reset,
		screens:         screens,
		graph:           graph,
		serving:         serving,
	}, nil
}

// valueOr returns the setting's value, or fallback when it is empty.
func valueOr(env gonsole.Env, name, fallback string) string {
	if value := env.Value(name); value != "" {
		return value
	}
	return fallback
}

// loadResetSettings reads the reset link lifetime, stack and rates from the environment.
func loadResetSettings(env gonsole.Env) (resetSettings, error) {
	ttl, err := env.Duration("RESET_TTL", authkit.DefaultResetTTL)
	if err != nil {
		return resetSettings{}, err
	}
	attempts, err := env.Count("RESET_ATTEMPTS", defaultResetAttempts)
	if err != nil {
		return resetSettings{}, err
	}
	links, err := env.Count("RESET_LINKS", defaultResetLinks)
	if err != nil {
		return resetSettings{}, err
	}
	cooldown, err := env.Duration("RESET_COOLDOWN", defaultResetCooldown)
	if err != nil {
		return resetSettings{}, err
	}
	return resetSettings{ttl: ttl, attempts: attempts, links: links, cooldown: cooldown}, nil
}
