// SPDX-License-Identifier: Elastic-2.0

package fields_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/plugins/fields"
	"github.com/gopherium/alphone/sdk"
)

// unreachable addresses a database the inert plugins never connect to.
const unreachable = "postgres://graph:graph@localhost:1/graph"

// newFieldsClient returns a graph client over a migrated fields plugin.
func newFieldsClient(t *testing.T) *gqlclient.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.CoreMigrator())
	pool, err := pgxpool.New(t.Context(), cfg.URL())
	if err != nil {
		t.Fatalf("connecting the test pool: %v", err)
	}
	t.Cleanup(pool.Close)
	plugin, err := fields.Register(sdk.Deps{DatabaseURL: cfg.URL()})
	if err != nil {
		t.Fatalf("fields.Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = plugin.Stop(context.Background()) })
	if err := plugin.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	if err := plugin.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	inert, err := graphroot.All(sdk.Deps{DatabaseURL: unreachable})
	if err != nil {
		t.Fatalf("graphroot.All() error = %v, want nil", err)
	}
	for _, registered := range inert {
		t.Cleanup(func() { _ = registered.Stop(context.Background()) })
	}
	root, err := graphroot.FromPlugins(&graphres.Resolver{
		Version:  "9.9.9",
		Contacts: postgres.NewContactStore(pool),
	}, append(inert, plugin))
	if err != nil {
		t.Fatalf("FromPlugins() error = %v, want nil", err)
	}
	srv := handler.New(graphres.ExecutableSchema(root))
	srv.AddTransport(transport.POST{})
	srv.SetErrorPresenter(graphres.PresentError)
	scoped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.ServeHTTP(w, r.WithContext(sdk.WithRequestScope(r.Context(), sdk.NewRequestScope())))
	})
	return gqlclient.New(scoped)
}

// newValuesClient returns a scoped graph client beside a seeded contact.
func newValuesClient(t *testing.T) (*gqlclient.Client, uuid.UUID) {
	t.Helper()
	client := newFieldsClient(t)
	return client, seedGraphContact(t, client, "Maria Perez")
}

// seedGraphContact stores a contact through the graph and returns its id.
func seedGraphContact(t *testing.T, client *gqlclient.Client, name string) uuid.UUID {
	t.Helper()
	var created struct {
		CreateContact struct {
			ID string `json:"id"`
		}
	}
	client.MustPost(`mutation($name: String!) { createContact(name: $name) { id } }`, &created,
		gqlclient.Var("name", name))
	id, err := uuid.Parse(created.CreateContact.ID)
	if err != nil {
		t.Fatalf("parsing the contact id: %v", err)
	}
	return id
}

// defineDate declares the birthDate field every value test writes into.
func defineDate(t *testing.T, client *gqlclient.Client) {
	t.Helper()
	var created struct {
		DefineField struct {
			ID string `json:"id"`
		}
	}
	client.MustPost(`mutation { defineField(name: "birthDate", label: "Birth date", kind: DATE) { id } }`, &created)
}

// definition is one catalogue entry as the graph answers it.
type definition struct {
	ID         string
	Name       string
	Label      string
	Kind       string
	ArchivedAt *string
}

func TestGraphDefinesAndListsAField(t *testing.T) {
	t.Parallel()

	client := newFieldsClient(t)

	var created struct{ DefineField definition }
	client.MustPost(
		`mutation { defineField(name: "birthDate", label: "Birth date", kind: DATE) {
			id name label kind archivedAt
		} }`, &created)

	if created.DefineField.Name != "birthDate" || created.DefineField.Kind != "DATE" {
		t.Errorf("defineField = %+v, want birthDate of kind DATE", created.DefineField)
	}
	if created.DefineField.ArchivedAt != nil {
		t.Error("archivedAt is set, want a live definition")
	}
	var listed struct{ Fields []definition }
	client.MustPost(`{ fields { id name label kind archivedAt } }`, &listed)
	if len(listed.Fields) != 1 || listed.Fields[0].Label != "Birth date" {
		t.Errorf("fields = %+v, want the created definition listed", listed.Fields)
	}
}

func TestGraphRefusesAMalformedName(t *testing.T) {
	t.Parallel()

	client := newFieldsClient(t)

	var created struct{ DefineField definition }
	err := client.Post(
		`mutation { defineField(name: "birth date", label: "Birth date", kind: DATE) { id } }`, &created)

	if err == nil || !strings.Contains(err.Error(), "camelCase") {
		t.Errorf("error = %v, want the malformed name refused", err)
	}
}

func TestGraphRefusesANameTheSchemaOwns(t *testing.T) {
	t.Parallel()

	client := newFieldsClient(t)

	var created struct{ DefineField definition }
	err := client.Post(`mutation { defineField(name: "name", label: "Name", kind: TEXT) { id } }`, &created)

	if err == nil || !strings.Contains(err.Error(), "already a field") {
		t.Errorf("error = %v, want the reserved name refused", err)
	}
}

func TestGraphRefusesADuplicateName(t *testing.T) {
	t.Parallel()

	client := newFieldsClient(t)
	var created struct{ DefineField definition }
	client.MustPost(`mutation { defineField(name: "birthDate", label: "Birth date", kind: DATE) { id } }`, &created)

	err := client.Post(
		`mutation { defineField(name: "birthDate", label: "Another", kind: TEXT) { id } }`, &created)

	if err == nil || !strings.Contains(err.Error(), "holds that name") {
		t.Errorf("error = %v, want the duplicate refused", err)
	}
}

func TestGraphArchivesAField(t *testing.T) {
	t.Parallel()

	client := newFieldsClient(t)
	var created struct{ DefineField definition }
	client.MustPost(`mutation { defineField(name: "birthDate", label: "Birth date", kind: DATE) { id } }`, &created)

	var archived struct{ ArchiveField bool }
	client.MustPost(`mutation($id: UUID!) { archiveField(id: $id) }`, &archived,
		gqlclient.Var("id", created.DefineField.ID))

	if !archived.ArchiveField {
		t.Error("archiveField = false, want true")
	}
	var live struct{ Fields []definition }
	client.MustPost(`{ fields { name } }`, &live)
	if len(live.Fields) != 0 {
		t.Errorf("live fields = %+v, want the archived definition hidden", live.Fields)
	}
	var every struct{ Fields []definition }
	client.MustPost(`{ fields(includeArchived: true) { name archivedAt } }`, &every)
	if len(every.Fields) != 1 || every.Fields[0].ArchivedAt == nil {
		t.Errorf("all fields = %+v, want the archived definition timestamped", every.Fields)
	}
}

func TestGraphRefusesArchivingAnUnknownField(t *testing.T) {
	t.Parallel()

	client := newFieldsClient(t)

	var archived struct{ ArchiveField bool }
	err := client.Post(`mutation($id: UUID!) { archiveField(id: $id) }`, &archived,
		gqlclient.Var("id", uuid.Must(uuid.NewV7()).String()))

	if err == nil || !strings.Contains(err.Error(), "no live definition") {
		t.Errorf("error = %v, want the unknown id refused", err)
	}
}
