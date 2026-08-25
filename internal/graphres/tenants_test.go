// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/tenant"
)

var errTenantStore = errors.New("the tenant store is unreachable")

// failingTenantStore refuses every lookup.
type failingTenantStore struct{}

// TenantForUser reports the store failure.
func (failingTenantStore) TenantForUser(context.Context, uuid.UUID) (tenant.Tenant, error) {
	return tenant.Tenant{}, errTenantStore
}

// TenantsOf reports the store failure.
func (failingTenantStore) TenantsOf(
	context.Context, []uuid.UUID,
) (map[uuid.UUID]uuid.UUID, error) {
	return nil, errTenantStore
}

// tenantRead is the answer shape of the tenant query.
type tenantRead struct {
	Tenant struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"tenant"`
}

func TestTenantAnswersTheDefaultForAnUnplacedCaller(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	resolver := &graphres.Resolver{Version: "test", Tenants: postgres.NewTenantStore(pool)}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	var read tenantRead
	client.MustPost(`{ tenant { id name } }`, &read)

	if read.Tenant.ID != tenant.DefaultID.String() {
		t.Errorf("id = %s, want the default tenant", read.Tenant.ID)
	}
	if read.Tenant.Name != "Default" {
		t.Errorf("name = %q, want Default", read.Tenant.Name)
	}
}

func TestTenantAnswersThePlacedTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	caller := uuid.Must(uuid.NewV7())
	acme := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)", acme, "Acme"); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenant_members (user_id, tenant_id) VALUES ($1, $2)", caller, acme); err != nil {
		t.Fatalf("placing the member: %v", err)
	}
	resolver := &graphres.Resolver{Version: "test", Tenants: postgres.NewTenantStore(pool)}
	client := newGraphClient(t, resolver, caller)

	var read tenantRead
	client.MustPost(`{ tenant { id name } }`, &read)

	if read.Tenant.ID != acme.String() || read.Tenant.Name != "Acme" {
		t.Errorf("tenant = %+v, want the placed Acme tenant", read.Tenant)
	}
}

func TestTenantReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	resolver := &graphres.Resolver{Version: "test", Tenants: failingTenantStore{}}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	var read tenantRead
	err := client.Post(`{ tenant { id name } }`, &read)

	if err == nil {
		t.Error("Post() error = nil, want the store failure surfaced")
	}
}
