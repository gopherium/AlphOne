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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/pluginkit"
	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/gouncer/authkit/ratelimit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
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

	pool, err := pgxpool.New(ctx, settings.databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()

	if err := migrateSchemas(ctx, settings.databaseURL); err != nil {
		return err
	}

	userStore := authkitpg.NewUserStore(pool)
	contacts := postgres.NewContactStore(pool)
	tasks := postgres.NewTaskStore(pool)
	tokens := postgres.NewTokenStore(pool)
	webhooks := postgres.NewWebhookStore(pool)
	dispatcher := webhook.NewDispatcher(webhooks, logger)
	deliveries := webhook.NewWorker(webhooks, logger, webhook.AddressGuard{Allowed: settings.webhookHosts})
	deliveries.Start()
	defer deliveries.Stop()
	hub := event.NewHub()
	events := nudgingPublisher{dispatcher: dispatcher, worker: deliveries, hub: hub}
	reaper := authkit.NewReaper(userStore, authkit.ReaperConfig{Logger: logger})
	reaper.Start()
	defer reaper.Stop()

	resolver := contact.NewResolver(contacts, contact.WithEvents(events))
	registered, err := plugins(sdk.Deps{
		DatabaseURL:    settings.databaseURL,
		PublicURL:      settings.mail.publicURL,
		MachineGrace:   settings.machineGrace,
		TenantsHeld:    settings.tenants.held,
		TenantsRefresh: settings.tenants.refresh,
		Resolver:       resolverBridge{resolver: resolver},
		Contacts:       directoryBridge{resolver: resolver},
		Events:         pluginPublisher{publisher: events},
		Getenv:         getenv,
		Env:            settingsEnv(getenv),
	})
	host := pluginkit.NewHost(registered...)
	if err != nil {
		return errors.Join(
			fmt.Errorf("register plugins: %w", err), gonsole.StopHost(ctx, host, settings.serving.StopGrace))
	}
	if err := declareRoles(role.Default, registered); err != nil {
		return errors.Join(
			fmt.Errorf("declare plugin roles: %w", err), gonsole.StopHost(ctx, host, settings.serving.StopGrace))
	}
	mailSender, mailer, err := buildMail(settings.mail, logger)
	if err != nil {
		return errors.Join(err, gonsole.StopHost(ctx, host, settings.serving.StopGrace))
	}
	tenants := postgres.NewTenantStore(pool)
	wireFieldProviders(registered)
	wireCredentialProviders(registered)
	wireTenantGate(registered, tenantGateBridge{tenants: tenants, grace: settings.machineGrace})
	wireMailSenderFrom(registered, mailSender)

	if err := host.Start(ctx, settings.serving.StopGrace); err != nil {
		return fmt.Errorf("start plugins: %w", err)
	}

	auth := authkit.New(authConfig(userStore))
	admin := authkit.NewAdmin(adminConfig(userStore))
	inviteConfig := authkit.InvitesConfig{
		Store:           userStore,
		InviteTTL:       settings.inviteTTL,
		ResetTTL:        settings.reset.ttl,
		ResetTokensLive: settings.reset.links,
	}
	graphRoot, err := graphroot.FromPlugins(&graphres.Resolver{
		Version:       version.Version(),
		Contacts:      contacts,
		Tasks:         tasks,
		Webhooks:      webhooks,
		Tenants:       tenants,
		Tokens:        tokens,
		Events:        events,
		Live:          hub,
		Auth:          auth,
		Admin:         admin,
		Invites:       authkit.NewInvites(inviteConfig),
		Onboarding:    postgres.NewOnboarding(pool, inviteConfig),
		Accounts:      userStore,
		Mailer:        mailer,
		PublicURL:     settings.mail.publicURL,
		Settings:      postgres.NewUserSettingStore(pool),
		LoginLimiter:  ratelimit.NewLimiter(ratelimit.Config{}),
		TokenLimiter:  ratelimit.NewLimiter(ratelimit.Config{}),
		ResetLimiter:  ratelimit.NewLimiter(resetBudget(settings)),
		ResetCooldown: ratelimit.NewLimiter(resetCooldownBudget(settings)),
		Logger:        logger,
		Paging:        settings.lists.paging,
		Screens:       settings.lists.screens,
	}, registered)
	if err != nil {
		return errors.Join(
			fmt.Errorf("compose graph root: %w", err), gonsole.StopHost(ctx, host, settings.serving.StopGrace))
	}

	cfg := settings.serverConfig()
	cfg.Version = version.Version()
	cfg.Users = userStore
	cfg.Tenants = tenants
	cfg.Auth = auth
	cfg.GraphRoot = graphRoot
	cfg.Tokens = tokens
	cfg.Plugins = host.Routes()
	cfg.PluginPublicPaths = host.PublicPaths()
	cfg.PluginAreas = pluginAreas(registered)
	cfg.FieldSources = fieldSources(registered)
	if settings.webDir != "" {
		cfg.Web = os.DirFS(settings.webDir)
	}

	httpServer := httpServerFrom(settings, server.NewServer(cfg))
	return gonsole.Serve(ctx, httpServer, settings.serving, host.Stop, logger)
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

// authConfig returns the login configuration the server serves sessions under.
func authConfig(store *authkitpg.UserStore) authkit.Config {
	return authkit.Config{
		Store:      store,
		CookieName: server.SessionCookieName,
		Privileged: role.Privileged(),
	}
}

// adminConfig returns the administration configuration guarding the privileged cover.
func adminConfig(store *authkitpg.UserStore) authkit.AdminConfig {
	return authkit.AdminConfig{Store: store, Privileged: role.Privileged()}
}

// declarePluginRoles registers the plugins over the settings a role declaration needs and grants
// the registry every role they declare.
func declarePluginRoles(
	registry *role.Registry,
	getenv func(string) string,
	plugins func(sdk.Deps) ([]sdk.Plugin, error),
) error {
	env := settingsEnv(getenv)
	registered, err := plugins(sdk.Deps{
		DatabaseURL: env.Value("DATABASE_URL"),
		Getenv:      getenv,
		Env:         env,
	})
	if err != nil {
		return fmt.Errorf("register plugins: %w", err)
	}
	return declareRoles(registry, registered)
}

// declareRoles grants the registry every role a registered plugin declares.
func declareRoles(registry *role.Registry, registered []sdk.Plugin) error {
	for _, plugin := range registered {
		provider, ok := plugin.(sdk.RoleProvider)
		if !ok {
			continue
		}
		for _, declared := range provider.Roles() {
			capabilities := make([]role.Capability, 0, len(declared.Capabilities))
			for _, capability := range declared.Capabilities {
				capabilities = append(capabilities, role.Capability(capability))
			}
			if err := registry.Grant(role.Role(declared.Name), capabilities...); err != nil {
				return fmt.Errorf("declare role %q for %s: %w", declared.Name, plugin.ID(), err)
			}
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
	lists          listSettings
	serving        gonsole.Timeouts
}

// composeSettings carries the machine grace, the tenant bounds, the mail settings and the webhook allow list.
type composeSettings struct {
	machineGrace time.Duration
	tenants      tenantSettings
	mail         mailSettings
	webhookHosts webhook.AllowList
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
		TrustedProxies: c.trustedProxies,
		GraphiQL:       c.graphiql,
		TenantsHeld:    c.tenants.held,
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

// loadComposeSettings reads the machine grace, the tenant bounds, the mail settings and the webhook allow list.
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
	return composeSettings{machineGrace: machineGrace, tenants: tenants, mail: mail, webhookHosts: webhookHosts}, nil
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
	composed, err := loadComposeSettings(env)
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
	lists, err := loadListSettings(env)
	if err != nil {
		return runConfig{}, err
	}
	serving, err := env.Timeouts(servingDefaults)
	if err != nil {
		return runConfig{}, err
	}
	return runConfig{
		composeSettings: composed,
		databaseURL:     databaseURL,
		addr:            valueOr(env, "ADDR", "localhost:8080"),
		webDir:          env.Value("WEB_DIR"),
		trustedProxies:  trustedProxies,
		graphiql:        env.Value("DEV_GRAPHIQL") != "",
		inviteTTL:       inviteTTL,
		reset:           reset,
		lists:           lists,
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
