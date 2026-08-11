// SPDX-License-Identifier: Elastic-2.0

package mcp

import (
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/validator/rules"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// toolDocuments pairs every tool document with the costliest variables it sends.
func toolDocuments() map[string]struct {
	document  string
	variables map[string]any
} {
	return map[string]struct {
		document  string
		variables map[string]any
	}{
		"workload_summary": {workloadDocument, map[string]any{
			"today": "2026-08-11", "first": workloadPage,
		}},
		"list_my_tasks": {tasksDocument, taskVariables(TasksInput{Limit: taskPageMax})},
		"find_contacts": {contactsDocument, contactVariables(ContactsInput{Limit: contactPageMax})},
		"get_contact": {contactDocument, map[string]any{
			"id": "0198c000-0000-7000-8000-000000000001", "first": contactTaskPage,
		}},
	}
}

// composedSchema builds the executable schema every tool document runs against.
func composedSchema(t *testing.T) graphql.ExecutableSchema {
	t.Helper()
	deps := sdk.Deps{DatabaseURL: "postgres://graph:graph@localhost:1/graph"}
	whatsappPlugin, err := whatsapp.Register(deps)
	if err != nil {
		t.Fatalf("whatsapp.Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = whatsappPlugin.Stop(t.Context()) })
	importerPlugin, err := importer.Register(deps)
	if err != nil {
		t.Fatalf("importer.Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = importerPlugin.Stop(t.Context()) })
	root, err := graphroot.FromPlugins(&graphres.Resolver{},
		[]sdk.Plugin{whatsappPlugin, importerPlugin})
	if err != nil {
		t.Fatalf("graphroot.FromPlugins() error = %v, want nil", err)
	}
	return graphres.ExecutableSchema(root)
}

func TestEveryToolDocumentMatchesTheLiveSchema(t *testing.T) {
	t.Parallel()

	schema := composedSchema(t)

	for name, tool := range toolDocuments() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := gqlparser.LoadQueryWithRules(schema.Schema(), tool.document, rules.NewDefaultRules())

			if err != nil {
				t.Errorf("the %s document no longer matches the schema: %v", name, err)
			}
		})
	}
}

func TestEveryToolDocumentFitsUnderTheComplexityCap(t *testing.T) {
	t.Parallel()

	schema := composedSchema(t)

	for name, tool := range toolDocuments() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			query, err := gqlparser.LoadQueryWithRules(schema.Schema(), tool.document, rules.NewDefaultRules())
			if err != nil {
				t.Fatalf("parsing the %s document: %v", name, err)
			}
			total := 0
			for _, op := range query.Operations {
				total += complexity.Calculate(t.Context(), schema, op, tool.variables)
			}

			t.Logf("%s prices at %d of %d", name, total, graphres.ComplexityLimit)
			if total >= graphres.ComplexityLimit {
				t.Errorf("%s prices at %d, want under the cap of %d", name, total, graphres.ComplexityLimit)
			}
		})
	}
}
