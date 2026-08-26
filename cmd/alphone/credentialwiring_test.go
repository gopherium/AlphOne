// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/tenant"
	"github.com/gopherium/alphone/sdk"
)

// inertPlugin carries a plugin identity and does nothing.
type inertPlugin struct{ id string }

// ID reports the plugin identifier.
func (p inertPlugin) ID() string { return p.id }

// Start does nothing.
func (p inertPlugin) Start(context.Context) error { return nil }

// Stop does nothing.
func (p inertPlugin) Stop(context.Context) error { return nil }

// credentialServingPlugin offers one channel's credentials.
type credentialServingPlugin struct{ inertPlugin }

// Channel names the channel the credentials send on.
func (credentialServingPlugin) Channel() sdk.Channel { return "whatsapp" }

// SetChannelCredentials stores nothing.
func (credentialServingPlugin) SetChannelCredentials(context.Context, string, string) error {
	return nil
}

// ChannelIdentifier answers no configured identifier.
func (credentialServingPlugin) ChannelIdentifier(context.Context) (string, bool, error) {
	return "", false, nil
}

// credentialTakingPlugin records the providers the host hands it.
type credentialTakingPlugin struct {
	inertPlugin
	received []sdk.CredentialProvider
}

// UseCredentialProviders keeps the handed providers.
func (p *credentialTakingPlugin) UseCredentialProviders(providers []sdk.CredentialProvider) {
	p.received = providers
}

// gateTakingPlugin records the tenant gate the host hands it.
type gateTakingPlugin struct {
	inertPlugin
	received sdk.TenantGate
}

// UseTenantGate keeps the handed gate.
func (p *gateTakingPlugin) UseTenantGate(gate sdk.TenantGate) {
	p.received = gate
}

// sentinelGate is a tenant gate a wiring test recognises by identity.
type sentinelGate struct{ mark int }

// AcceptsMachineTraffic accepts every tenant.
func (sentinelGate) AcceptsMachineTraffic(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}

func TestWiringHandsTheTenantGateToEveryConsumer(t *testing.T) {
	t.Parallel()

	taking := &gateTakingPlugin{inertPlugin: inertPlugin{id: "whatsapp"}}
	gate := sentinelGate{mark: 7}

	wireTenantGate([]sdk.Plugin{taking, inertPlugin{id: "bystander"}}, gate)

	if taking.received != sdk.TenantGate(gate) {
		t.Errorf("the plugin received %#v, want the very gate the host handed it", taking.received)
	}
}

func TestTheTenantGateAnswersFromTheStore(t *testing.T) {
	t.Parallel()

	pool := testPool(t, testDatabaseURL(t))
	gate := tenantGateBridge{tenants: postgres.NewTenantStore(pool)}
	closed := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name, deactivated_at) VALUES ($1, $2, now() - interval '20 days')",
		closed, "Acme"); err != nil {
		t.Fatalf("seeding the closed tenant: %v", err)
	}

	live, err := gate.AcceptsMachineTraffic(t.Context(), tenant.DefaultID)
	if err != nil {
		t.Fatalf("AcceptsMachineTraffic() error = %v, want nil", err)
	}
	if !live {
		t.Error("the default tenant refuses machine traffic, want it accepted")
	}

	past, err := gate.AcceptsMachineTraffic(t.Context(), closed)
	if err != nil {
		t.Fatalf("AcceptsMachineTraffic() error = %v, want nil", err)
	}
	if past {
		t.Error("a tenant closed twenty days ago accepts machine traffic, want it refused")
	}
}

func TestTheTenantGateReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	pool := testPool(t, testDatabaseURL(t))
	gate := tenantGateBridge{tenants: postgres.NewTenantStore(pool)}
	pool.Close()

	if _, err := gate.AcceptsMachineTraffic(t.Context(), tenant.DefaultID); err == nil {
		t.Error("AcceptsMachineTraffic() error = nil, want the closed pool reported")
	}
}

func TestWiringHandsEveryCredentialProviderToEveryConsumer(t *testing.T) {
	t.Parallel()

	serving := credentialServingPlugin{inertPlugin{id: "whatsapp"}}
	taking := &credentialTakingPlugin{inertPlugin: inertPlugin{id: "tenancy"}}

	wireCredentialProviders([]sdk.Plugin{serving, taking, inertPlugin{id: "bystander"}})

	if len(taking.received) != 1 {
		t.Fatalf("received %d providers, want the one serving plugin", len(taking.received))
	}
	if taking.received[0].Channel() != "whatsapp" {
		t.Errorf("channel = %q, want whatsapp", taking.received[0].Channel())
	}
}
