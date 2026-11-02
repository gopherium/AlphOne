// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// numberSigner is what the webhook reads of one stored number to prove a delivery.
type numberSigner struct {
	phoneNumberID string
	tenantID      uuid.UUID
	appSecret     []byte
}

// numberRoute is the tenant the items of a proven number land in and the number their conversation stores.
type numberRoute struct {
	tenantID uuid.UUID
	number   *string
}

// numberSigners reads every stored number among the given ones in one query, across every tenant.
func (s *store) numberSigners(ctx context.Context, numbers []string) ([]numberSigner, error) {
	if len(numbers) == 0 {
		return nil, nil
	}
	rows, _ := s.pool.Query(ctx,
		"SELECT phone_number_id, tenant_id, app_secret FROM plugin_whatsapp.credentials"+
			" WHERE phone_number_id = ANY($1)",
		numbers)
	signers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (numberSigner, error) {
		var signer numberSigner
		err := row.Scan(&signer.phoneNumberID, &signer.tenantID, &signer.appSecret)
		return signer, err
	})
	if err != nil {
		return nil, fmt.Errorf("whatsapp: read numbers: %w", err)
	}
	return signers, nil
}

// parseSignature answers the digest a signature header carries and whether the header has the shape Meta sends.
func parseSignature(header string) ([]byte, bool) {
	encoded, prefixed := strings.CutPrefix(header, "sha256=")
	if !prefixed || len(encoded) != hex.EncodedLen(sha256.Size) {
		return nil, false
	}
	digest, err := hex.DecodeString(encoded)
	return digest, err == nil
}

// signedBody is a delivery's raw body beside the digest its signature header carries.
type signedBody struct {
	digest []byte
	body   []byte
}

// signedWith reports whether the digest is the HMAC-SHA256 of the body under secret, an empty secret proving nothing.
func (s signedBody) signedWith(secret []byte) bool {
	if len(secret) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(s.body)
	return hmac.Equal(s.digest, mac.Sum(nil))
}

// proveDelivery answers the routes of the numbers a signature proves and whether any secret proved the body.
func (p *Plugin) proveDelivery(
	ctx context.Context, signed signedBody, named []string,
) (map[string]numberRoute, bool, error) {
	signers, err := p.store.numberSigners(ctx, named)
	if err != nil {
		return nil, false, err
	}
	if ownSecrets(signers) > p.deliveryNumbersMax {
		return nil, false, nil
	}
	envProven := signed.signedWith([]byte(p.appSecret))
	routes := p.envRoutes(envProven)
	proven := envProven
	for _, signer := range signers {
		delete(routes, signer.phoneNumberID)
		if p.signerProves(ctx, signer, signed, envProven) {
			routes[signer.phoneNumberID] = numberRoute{tenantID: signer.tenantID, number: &signer.phoneNumberID}
			proven = true
		}
	}
	return routes, proven, nil
}

// ownSecrets counts the signers holding a secret of their own.
func ownSecrets(signers []numberSigner) int {
	held := 0
	for _, signer := range signers {
		if signer.appSecret != nil {
			held++
		}
	}
	return held
}

// envRoutes answers the routes the environment secret opens, the default tenant's when it proved the body.
func (p *Plugin) envRoutes(envProven bool) map[string]numberRoute {
	if !envProven || p.envCredentials.phoneNumberID == "" {
		return map[string]numberRoute{}
	}
	route := numberRoute{tenantID: sdk.DefaultTenantID}
	return map[string]numberRoute{"": route, p.envCredentials.phoneNumberID: route}
}

// signerProves reports whether a stored number's own secret, or the setting for a row holding none, signed the body.
func (p *Plugin) signerProves(ctx context.Context, signer numberSigner, signed signedBody, envProven bool) bool {
	if signer.appSecret == nil {
		return envProven
	}
	secret, err := p.encrypter.Decrypt(sdk.WithTenant(ctx, signer.tenantID), appSecretPurpose, signer.appSecret)
	if err != nil {
		return false
	}
	return signed.signedWith(secret)
}

// openRoutes keeps the routes whose tenant still records what a channel delivers.
func (p *Plugin) openRoutes(ctx context.Context, routes map[string]numberRoute) (map[string]numberRoute, error) {
	open := make(map[string]numberRoute, len(routes))
	for number, route := range routes {
		recording, err := p.recordsTraffic(sdk.WithTenant(ctx, route.tenantID), route.tenantID)
		if err != nil {
			return nil, err
		}
		if recording {
			open[number] = route
		}
	}
	return open, nil
}
