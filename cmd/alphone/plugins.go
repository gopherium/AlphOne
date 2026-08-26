// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/sdk"
)

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
