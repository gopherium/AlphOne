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
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/gouncer/authkit/ratelimit"

	"github.com/gopherium/pluginkit"

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
	deliveries := webhook.NewWorker(webhooks, logger)
	deliveries.Start()
	defer deliveries.Stop()
	hub := event.NewHub()
	events := nudgingPublisher{dispatcher: dispatcher, worker: deliveries, hub: hub}
	reaper := authkit.NewReaper(userStore, authkit.ReaperConfig{Logger: logger})
	reaper.Start()
	defer reaper.Stop()

	resolver := contact.NewResolver(contacts, contact.WithEvents(events))
	registered, err := plugins(sdk.Deps{
		DatabaseURL: settings.databaseURL,
		PublicURL:   settings.mail.publicURL,
		Resolver:    resolverBridge{resolver: resolver},
		Contacts:    directoryBridge{resolver: resolver},
		Events:      pluginPublisher{publisher: events},
		Getenv:      getenv,
	})
	if err != nil {
		return fmt.Errorf("register plugins: %w", err)
	}
	if err := declareRoles(role.Default, registered); err != nil {
		return fmt.Errorf("declare plugin roles: %w", err)
	}
	mailSender, mailer, err := buildMail(settings.mail, logger)
	if err != nil {
		return err
	}
	tenants := postgres.NewTenantStore(pool)
	wireFieldProviders(registered)
	wireCredentialProviders(registered)
	wireTenantGate(registered, tenantGateBridge{tenants: tenants, grace: settings.machineGrace})
	wireMailSenderFrom(registered, mailSender)

	host := pluginkit.NewHost(registered...)
	if err := host.Start(ctx); err != nil {
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
	}, registered)
	if err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return errors.Join(fmt.Errorf("compose graph root: %w", err), host.Stop(stopCtx))
	}

	cfg := server.Config{
		Version:           version.Version(),
		Users:             userStore,
		Tenants:           tenants,
		Auth:              auth,
		GraphRoot:         graphRoot,
		Tokens:            tokens,
		Plugins:           host.Routes(),
		PluginPublicPaths: host.PublicPaths(),
		PluginAreas:       pluginAreas(registered),
		FieldSources:      fieldSources(registered),
		TrustedProxies:    settings.trustedProxies,
		GraphiQL:          settings.graphiql,
	}
	if settings.webDir != "" {
		cfg.Web = os.DirFS(settings.webDir)
	}

	httpServer := &http.Server{
		Addr:              settings.addr,
		Handler:           server.NewServer(cfg),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return serveUntilDone(ctx, httpServer, host, logger)
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
	registered, err := plugins(sdk.Deps{
		DatabaseURL: getenv("ALPHONE_DATABASE_URL"),
		Getenv:      getenv,
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
	databaseURL    string
	addr           string
	webDir         string
	trustedProxies []string
	graphiql       bool
	machineGrace   time.Duration
	mail           mailSettings
	inviteTTL      time.Duration
	reset          resetSettings
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

// parseMailPort reads the relay port, empty applying the submission default.
func parseMailPort(raw string) (int, error) {
	if raw == "" {
		return 587, nil
	}
	held, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse ALPHONE_SMTP_PORT: %w", err)
	}
	if held < 1 || held > 65535 {
		return 0, errors.New("ALPHONE_SMTP_PORT must be between 1 and 65535")
	}
	return held, nil
}

// parseMailTLS reads the transport security policy, empty applying mandatory.
func parseMailTLS(raw string) (string, error) {
	switch raw {
	case "":
		return "mandatory", nil
	case "mandatory", "opportunistic", "none":
		return raw, nil
	}
	return "", fmt.Errorf("ALPHONE_SMTP_TLS must be mandatory, opportunistic or none, got %q", raw)
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

// parsePositiveCount reads a positive count named by the variable, empty applying the fallback.
func parsePositiveCount(name, raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	held, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if held <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return held, nil
}

// parseTokenTTL reads one token lifetime, empty applying the fallback.
func parseTokenTTL(name, raw string, fallback time.Duration) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	held, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if held <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return held, nil
}

// parsePublicURL reads the address email links lead back to.
func parsePublicURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	held, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse ALPHONE_PUBLIC_URL: %w", err)
	}
	if (held.Scheme != "http" && held.Scheme != "https") || held.Host == "" {
		return "", fmt.Errorf("ALPHONE_PUBLIC_URL must be an http or https address, got %q", raw)
	}
	if strings.ContainsAny(raw, "?#") {
		return "", fmt.Errorf("ALPHONE_PUBLIC_URL must carry no query or fragment, got %q", raw)
	}
	if escaped := held.EscapedPath(); escaped != "" && escaped != "/" {
		return "", fmt.Errorf("ALPHONE_PUBLIC_URL must name a site root, got %q", raw)
	}
	return strings.TrimSuffix(raw, "/"), nil
}

// loadMailSettings reads the mail relay settings from the environment.
func loadMailSettings(getenv func(string) string) (mailSettings, error) {
	host := getenv("ALPHONE_SMTP_HOST")
	if host == "" {
		for _, name := range []string{
			"ALPHONE_SMTP_PORT",
			"ALPHONE_SMTP_USERNAME",
			"ALPHONE_SMTP_PASSWORD",
			"ALPHONE_SMTP_FROM",
			"ALPHONE_SMTP_TLS",
		} {
			if getenv(name) != "" {
				return mailSettings{}, fmt.Errorf("%s is set but ALPHONE_SMTP_HOST is not", name)
			}
		}
		return mailSettings{}, nil
	}
	port, err := parseMailPort(getenv("ALPHONE_SMTP_PORT"))
	if err != nil {
		return mailSettings{}, err
	}
	tlsPolicy, err := parseMailTLS(getenv("ALPHONE_SMTP_TLS"))
	if err != nil {
		return mailSettings{}, err
	}
	from := getenv("ALPHONE_SMTP_FROM")
	if from == "" {
		return mailSettings{}, errors.New("ALPHONE_SMTP_FROM is required when ALPHONE_SMTP_HOST is set")
	}
	publicURL, err := parsePublicURL(getenv("ALPHONE_PUBLIC_URL"))
	if err != nil {
		return mailSettings{}, err
	}
	if publicURL == "" {
		return mailSettings{}, errors.New("ALPHONE_PUBLIC_URL is required when ALPHONE_SMTP_HOST is set")
	}
	return mailSettings{
		host:        host,
		port:        port,
		username:    getenv("ALPHONE_SMTP_USERNAME"),
		password:    getenv("ALPHONE_SMTP_PASSWORD"),
		from:        from,
		tls:         tlsPolicy,
		publicURL:   publicURL,
		templateDir: getenv("ALPHONE_MAIL_TEMPLATE_DIR"),
	}, nil
}

// parseMachineGrace reads the grace window a deactivated tenant keeps recording for.
func parseMachineGrace(raw string) (time.Duration, error) {
	if raw == "" {
		return tenant.DefaultMachineGrace, nil
	}
	held, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse ALPHONE_TENANT_MACHINE_GRACE: %w", err)
	}
	if held < 0 {
		return 0, errors.New("ALPHONE_TENANT_MACHINE_GRACE must not be negative")
	}
	return held, nil
}

// loadRunConfig reads the server settings from the environment.
func loadRunConfig(getenv func(string) string) (runConfig, error) {
	databaseURL := getenv("ALPHONE_DATABASE_URL")
	if databaseURL == "" {
		return runConfig{}, errors.New("ALPHONE_DATABASE_URL is required")
	}
	addr := getenv("ALPHONE_ADDR")
	if addr == "" {
		addr = "localhost:8080"
	}
	trustedProxies, err := parseTrustedProxies(getenv("ALPHONE_TRUSTED_PROXIES"))
	if err != nil {
		return runConfig{}, err
	}
	machineGrace, err := parseMachineGrace(getenv("ALPHONE_TENANT_MACHINE_GRACE"))
	if err != nil {
		return runConfig{}, err
	}
	mail, err := loadMailSettings(getenv)
	if err != nil {
		return runConfig{}, err
	}
	inviteTTL, err := parseTokenTTL("ALPHONE_INVITE_TTL", getenv("ALPHONE_INVITE_TTL"), authkit.DefaultInviteTTL)
	if err != nil {
		return runConfig{}, err
	}
	reset, err := loadResetSettings(getenv)
	if err != nil {
		return runConfig{}, err
	}
	return runConfig{
		databaseURL:    databaseURL,
		addr:           addr,
		webDir:         getenv("ALPHONE_WEB_DIR"),
		trustedProxies: trustedProxies,
		graphiql:       getenv("ALPHONE_DEV_GRAPHIQL") != "",
		machineGrace:   machineGrace,
		mail:           mail,
		inviteTTL:      inviteTTL,
		reset:          reset,
	}, nil
}

// loadResetSettings reads the reset link lifetime, stack and rates from the environment.
func loadResetSettings(getenv func(string) string) (resetSettings, error) {
	ttl, err := parseTokenTTL("ALPHONE_RESET_TTL", getenv("ALPHONE_RESET_TTL"), authkit.DefaultResetTTL)
	if err != nil {
		return resetSettings{}, err
	}
	attempts, err := parsePositiveCount("ALPHONE_RESET_ATTEMPTS", getenv("ALPHONE_RESET_ATTEMPTS"), defaultResetAttempts)
	if err != nil {
		return resetSettings{}, err
	}
	links, err := parsePositiveCount("ALPHONE_RESET_LINKS", getenv("ALPHONE_RESET_LINKS"), defaultResetLinks)
	if err != nil {
		return resetSettings{}, err
	}
	cooldown, err := parseTokenTTL("ALPHONE_RESET_COOLDOWN", getenv("ALPHONE_RESET_COOLDOWN"), defaultResetCooldown)
	if err != nil {
		return resetSettings{}, err
	}
	return resetSettings{ttl: ttl, attempts: attempts, links: links, cooldown: cooldown}, nil
}

// serveUntilDone serves HTTP until ctx is cancelled or serving fails, then
// stops the plugin host.
func serveUntilDone(
	ctx context.Context,
	httpServer *http.Server,
	host *pluginkit.Host,
	logger *slog.Logger,
) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServer.ListenAndServe()
	}()
	logger.Info("listening", "addr", httpServer.Addr)

	select {
	case err := <-serveErr:
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return errors.Join(fmt.Errorf("http server: %w", err), host.Stop(stopCtx))
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return errors.Join(httpServer.Shutdown(shutdownCtx), host.Stop(shutdownCtx))
}

// parseTrustedProxies parses raw into trusted-proxy CIDR ranges.
func parseTrustedProxies(raw string) ([]string, error) {
	prefixes, err := ratelimit.ParseTrustedProxies(raw)
	if err != nil {
		return nil, fmt.Errorf("ALPHONE_TRUSTED_PROXIES: %w", err)
	}
	return prefixes, nil
}
