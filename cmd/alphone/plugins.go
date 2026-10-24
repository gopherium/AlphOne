// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/framework/goncierge"
	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/pluginkit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/sdk"
)

// loadPlugins returns the registration the command line runs the compiled plugins through, starting none.
func loadPlugins(
	registry *goncierge.Registry, plugins func(sdk.Deps) ([]sdk.Plugin, error),
) func(context.Context, gonsole.Call) (gonsole.Loaded, error) {
	return func(ctx context.Context, call gonsole.Call) (gonsole.Loaded, error) {
		grace, err := call.Env.Duration("SHUTDOWN_STOP_GRACE", servingDefaults.StopGrace)
		if err != nil {
			return gonsole.Loaded{}, err
		}
		databaseURL, err := call.DatabaseURL()
		if err != nil {
			return gonsole.Loaded{}, err
		}
		settings, err := loadComposeSettings(call.Env)
		if err != nil {
			return gonsole.Loaded{}, err
		}
		built, err := compose(ctx, composeConfig{
			composeSettings: settings,
			databaseURL:     databaseURL,
			getenv:          call.Env.Getenv,
			roles:           registry,
			logger:          slog.New(slog.NewTextHandler(call.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		}, plugins)
		if err != nil {
			return gonsole.Loaded{}, errors.Join(built.failed, err, abandon(ctx, built, grace))
		}
		host := pluginkit.NewHost(built.registered...)
		return gonsole.Hosted(built.registered, host, built.failed, grace, built.pool.Close), nil
	}
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

type tenantGateBridge struct {
	tenants *postgres.TenantStore
	grace   time.Duration
}

// AcceptsMachineTraffic reports whether the tenant still records what a channel delivers.
func (b tenantGateBridge) AcceptsMachineTraffic(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	held, err := b.tenants.TenantByID(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return held.AcceptsMachineTraffic(time.Now(), b.grace), nil
}

type resolverBridge struct {
	resolver *contact.Resolver
}

// Resolve resolves a channel identifier to an [sdk.Contact] via the underlying contact resolver.
func (b resolverBridge) Resolve(
	ctx context.Context,
	channel sdk.Channel,
	identifier, displayName string,
) (sdk.Contact, error) {
	owner, err := b.resolver.Resolve(ctx, contact.Channel(channel), identifier, displayName)
	if err != nil {
		return sdk.Contact{}, markInvalid(err)
	}
	return sdk.Contact{ID: owner.ID, Name: owner.Name}, nil
}

type directoryBridge struct {
	resolver *contact.Resolver
}

// FindByIdentity returns the [sdk.Contact] owning an identity, reporting whether one exists.
func (b directoryBridge) FindByIdentity(
	ctx context.Context, channel sdk.Channel, identifier string,
) (sdk.Contact, bool, error) {
	owner, found, err := b.resolver.FindByIdentity(ctx, contact.Channel(channel), identifier)
	if err != nil || !found {
		return sdk.Contact{}, false, markInvalid(err)
	}
	return sdk.Contact{ID: owner.ID, Name: owner.Name}, true, nil
}

// CreateWithIdentities stores an [sdk.Contact] owning every identity, reporting whether it was created.
func (b directoryBridge) CreateWithIdentities(
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
	owner, created, err := b.resolver.CreateWithIdentities(ctx, name, addresses)
	if err != nil {
		return sdk.Contact{}, false, markInvalid(err)
	}
	return sdk.Contact{ID: owner.ID, Name: owner.Name}, created, nil
}

type matcherBridge struct {
	matcher *contact.Matcher
}

// MatchIdentities returns the [sdk.Contact] owning each identifier one owns, keyed by its canonical form.
func (b matcherBridge) MatchIdentities(
	ctx context.Context, channel sdk.Channel, identifiers []string,
) (map[string]sdk.Contact, error) {
	owners, err := b.matcher.MatchIdentities(ctx, contact.Channel(channel), identifiers)
	if errors.Is(err, contact.ErrTooManyIdentifiers) {
		return nil, fmt.Errorf("%w: %w", sdk.ErrTooManyIdentifiers, err)
	}
	if err != nil {
		return nil, markInvalid(err)
	}
	matched := make(map[string]sdk.Contact, len(owners))
	for identifier, owner := range owners {
		matched[identifier] = sdk.Contact{ID: owner.ID, Name: owner.Name}
	}
	return matched, nil
}

type identityFeedBridge struct {
	feed *contact.Feed
}

// IdentitiesAddedSince returns at most limit [sdk.AddedIdentity] values past the cursor and the next cursor.
func (b identityFeedBridge) IdentitiesAddedSince(
	ctx context.Context, channel sdk.Channel, cursor sdk.IdentityCursor, limit int,
) ([]sdk.AddedIdentity, sdk.IdentityCursor, error) {
	added, next, err := b.feed.IdentitiesAddedSince(ctx, contact.Channel(channel), contact.Cursor(cursor), limit)
	if err != nil {
		return nil, cursor, markFeedRefusal(err)
	}
	answered := make([]sdk.AddedIdentity, len(added))
	for i, identity := range added {
		answered[i] = sdk.AddedIdentity(identity)
	}
	return answered, sdk.IdentityCursor(next), nil
}

// markFeedRefusal returns err with a refused feed read marked for the plugin.
func markFeedRefusal(err error) error {
	switch {
	case errors.Is(err, contact.ErrCursorStale):
		return fmt.Errorf("%w: %w", sdk.ErrCursorStale, err)
	case errors.Is(err, contact.ErrInvalidLimit):
		return fmt.Errorf("%w: %w", sdk.ErrInvalidLimit, err)
	}
	return markInvalid(err)
}

// Horizon returns the cursor a new reader starts from.
func (b identityFeedBridge) Horizon(ctx context.Context) (sdk.IdentityCursor, error) {
	horizon, err := b.feed.Horizon(ctx)
	return sdk.IdentityCursor(horizon), err
}
