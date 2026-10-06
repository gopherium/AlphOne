// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"net/netip"
	"reflect"
	"testing"

	"github.com/gopherium/alphone/internal/webhook"
)

func TestTheWebhookAllowListIsEmptyWhenUnset(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(held.webhookHosts, webhook.AllowList{}) {
		t.Errorf("webhookHosts = %+v, want nothing internal allowed", held.webhookHosts)
	}
}

func TestTheWebhookAllowListIsReadFromTheSetting(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":          "postgres://localhost/x",
		"ALPHONE_WEBHOOK_ALLOWED_HOSTS": "127.0.0.1/32, n8n:5678",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	want := webhook.AllowList{Ranges: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}, Hosts: []string{"n8n:5678"}}
	if !reflect.DeepEqual(held.webhookHosts, want) {
		t.Errorf("webhookHosts = %+v, want %+v", held.webhookHosts, want)
	}
}
