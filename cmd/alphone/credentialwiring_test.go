// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"testing"

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
