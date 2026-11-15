// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"errors"
	"io"
	"maps"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/introspection"
	"github.com/google/uuid"
	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator/rules"

	"github.com/gopherium/alphone/internal/graphres"
)

// shelfSchema is a small schema whose fields reach every shape the rule tells apart.
const shelfSchema = `
scalar Upload

type Query {
	shelf: Shelf
	item(id: ID!): Item
	items(first: Int = 10): [Item!]!
	tagged(tags: [String!]): [Item!]!
	grid(rows: [[Int!]!]): [Item!]!
	matrix(rows: [[Int]]): [Item!]!
	search: [Result!]!
}

type Mutation {
	shelve(file: Upload!): Item
}

type Shelf {
	label: String
	item(rank: Int): Item
}

type Item {
	name: String
}

type Book {
	name: String
}

union Result = Item | Book
`

// fieldCatalogueDocument is the field catalogue the fields screen asks for, the one shipped document with an alias.
const fieldCatalogueDocument = `query FieldCatalogue {
	fields { id name label kind subFields { name label kind } }
	every: fields(includeArchived: true) { id name label kind archivedAt subFields { name label kind } }
}`

// askedDocument is a document asked with its variables.
type askedDocument struct {
	document  string
	variables map[string]any
}

// ruleAnswer returns what the rule answers the document asked with variables over schema.
func ruleAnswer(t *testing.T, schema *ast.Schema, asked askedDocument) *gqlerror.Error {
	t.Helper()
	query, err := gqlparser.LoadQueryWithRules(schema, asked.document, rules.NewDefaultRules())
	if err != nil {
		t.Fatalf("parsing %s: %v", asked.document, err)
	}
	rc := &graphql.OperationContext{Doc: query, Operation: query.Operations[0], Variables: asked.variables}
	return graphres.RepeatedFields{}.MutateOperationContext(t.Context(), rc)
}

// uploadedBytes are the bytes of the one file the upload tests send under two variables.
const uploadedBytes = "a,b\n"

// twoUploadsDocument stores the uploads of two variables under two names.
const twoUploadsDocument = `mutation($file: Upload!, $another: Upload!) {
	once: shelve(file: $file) { name } twice: shelve(file: $another) { name }
}`

// errUnreadable is the failure of every read of an unreadable upload.
var errUnreadable = errors.New("the upload cannot be read")

// errUnseekable is the failure of every seek of an unseekable upload.
var errUnseekable = errors.New("the upload cannot be rewound")

// errInterrupted is the failure of the first read of an interrupted upload.
var errInterrupted = errors.New("the upload read was interrupted")

// interruptedFile is an upload file whose first read stops with a failure after one byte.
type interruptedFile struct {
	contents    *strings.Reader
	interrupted bool
}

// Read returns one byte and a failure the first time and reads on in full afterwards.
func (f *interruptedFile) Read(p []byte) (int, error) {
	if f.interrupted {
		return f.contents.Read(p)
	}
	f.interrupted = true
	n, _ := f.contents.Read(p[:min(len(p), 1)])
	return n, errInterrupted
}

// Seek moves within the contents of the file.
func (f *interruptedFile) Seek(offset int64, whence int) (int64, error) {
	return f.contents.Seek(offset, whence)
}

// unreadableFile is an upload file whose every read fails.
type unreadableFile struct {
	reason error
}

// Read fails with the reason of the file.
func (f *unreadableFile) Read([]byte) (int, error) {
	return 0, f.reason
}

// Seek stays at the start.
func (*unreadableFile) Seek(int64, int) (int64, error) {
	return 0, nil
}

// unseekableFile is an upload file read in full whose every seek fails.
type unseekableFile struct {
	*strings.Reader
}

// Seek fails without moving.
func (*unseekableFile) Seek(int64, int) (int64, error) {
	return 0, errUnseekable
}

// uploadOf returns an upload of contents under the name, size and type every test upload shares.
func uploadOf(contents string) graphql.Upload {
	return uploadFrom(strings.NewReader(contents))
}

// uploadFrom returns an upload reading file under the name, size and type every test upload shares.
func uploadFrom(file io.ReadSeeker) graphql.Upload {
	return graphql.Upload{File: file, Filename: "contacts.csv", Size: int64(len(uploadedBytes)), ContentType: "text/csv"}
}

// sameUploadTwice returns the variables of the two uploads document both holding upload.
func sameUploadTwice(upload graphql.Upload) map[string]any {
	return map[string]any{"file": upload, "another": upload}
}

// composedSchema returns the schema the composed root the binary serves answers.
func composedSchema(t *testing.T) *ast.Schema {
	t.Helper()
	return graphres.ExecutableSchema(composedRoot(t, &graphres.Resolver{})).Schema()
}

// repeatedName returns the field the rule names in its rejection of the document over the shelf schema.
func repeatedName(t *testing.T, asked askedDocument) any {
	t.Helper()
	rejected := ruleAnswer(t, gqlparser.MustLoadSchema(&ast.Source{Input: shelfSchema}), asked)
	if rejected == nil {
		return nil
	}
	presented := graphres.PresentError(t.Context(), rejected)
	meta, _ := presented.Extensions["meta"].(map[string]any)
	return meta["field"]
}

func TestTheRuleRejectsAFieldAskedWithTheSameArgumentsUnderTwoNames(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		asked askedDocument
		field string
	}{
		"two aliases of a scalar": {askedDocument{document: `{ shelf { once: label twice: label } }`}, "Shelf.label"},
		"a name and an alias":     {askedDocument{document: `{ shelf { label again: label } }`}, "Shelf.label"},
		"two fragments": {askedDocument{document: `{ shelf { ...Once ...Twice } }
			fragment Once on Shelf { once: label }
			fragment Twice on Shelf { twice: label }`}, "Shelf.label"},
		"an inline fragment and a spread": {askedDocument{document: `{ shelf { ... on Shelf { once: label } ...Twice } }
			fragment Twice on Shelf { twice: label }`}, "Shelf.label"},
		"a fragment with no type condition": {askedDocument{
			document: `{ shelf { once: label ... { twice: label } } }`}, "Shelf.label"},
		"a variable and a literal of one value": {askedDocument{
			document:  `query($id: ID!) { byVariable: item(id: $id) { name } byLiteral: item(id: "1") { name } }`,
			variables: map[string]any{"id": "1"},
		}, "Query.item"},
		"a number variable and a number literal of one value": {askedDocument{
			document: `query($rank: Int) {
				shelf { byVariable: item(rank: $rank) { name } byLiteral: item(rank: 2) { name } }
			}`,
			variables: map[string]any{"rank": 2},
		}, "Shelf.item"},
		"a default and the same value written out": {askedDocument{
			document: `{ unsized: items { name } sized: items(first: 10) { name } }`}, "Query.items"},
		"a field kept by a false skip": {askedDocument{
			document: `{ shelf { label kept: label @skip(if: false) } }`}, "Shelf.label"},
		"a root field": {askedDocument{document: `{ once: shelf { label } twice: shelf { label } }`}, "Query.shelf"},
		"a nested repeat": {askedDocument{
			document: `{ shelf { item(rank: 1) { once: name twice: name } } }`}, "Item.name"},
		"a repeat across two selections of one name": {askedDocument{
			document: `{ shelf { label } shelf { again: label } }`}, "Shelf.label"},
		"a single value and a list of it": {askedDocument{
			document: `{ once: tagged(tags: "new") { name } twice: tagged(tags: ["new"]) { name } }`}, "Query.tagged"},
		"a single value variable coerced to a typed list and a written list of it": {askedDocument{
			document:  `query($tags: [String!]) { once: tagged(tags: $tags) { name } twice: tagged(tags: ["new"]) { name } }`,
			variables: map[string]any{"tags": []string{"new"}},
		}, "Query.tagged"},
		"a single value and a nested list of it": {askedDocument{
			document: `{ once: grid(rows: 1) { name } twice: grid(rows: [[1]]) { name } }`}, "Query.grid"},
		"a list and the nested list of its values": {askedDocument{
			document: `{ once: grid(rows: [1, 2]) { name } twice: grid(rows: [[1], [2]]) { name } }`}, "Query.grid"},
		"a null argument and the argument left out": {askedDocument{
			document: `{ shelf { bare: item { name } nulled: item(rank: null) { name } } }`}, "Shelf.item"},
		"a null variable and the argument left out": {askedDocument{
			document:  `query($rank: Int) { shelf { bare: item { name } nulled: item(rank: $rank) { name } } }`,
			variables: map[string]any{"rank": nil},
		}, "Shelf.item"},
		"one upload variable under two names": {askedDocument{
			document:  `mutation($file: Upload!) { once: shelve(file: $file) { name } twice: shelve(file: $file) { name } }`,
			variables: map[string]any{"file": uploadOf(uploadedBytes)},
		}, "Mutation.shelve"},
		"one file under two variables": {askedDocument{
			document:  twoUploadsDocument,
			variables: map[string]any{"file": uploadOf(uploadedBytes), "another": uploadOf(uploadedBytes)},
		}, "Mutation.shelve"},
		"one unreadable file under two variables": {askedDocument{
			document: twoUploadsDocument, variables: sameUploadTwice(uploadFrom(&unreadableFile{reason: errUnreadable})),
		}, "Mutation.shelve"},
		"one unseekable file under two variables": {askedDocument{
			document:  twoUploadsDocument,
			variables: sameUploadTwice(uploadFrom(&unseekableFile{strings.NewReader(uploadedBytes)})),
		}, "Mutation.shelve"},
	} {
		if got := repeatedName(t, test.asked); got != test.field {
			t.Errorf("%s: the rule named %v, want %s rejected", name, got, test.field)
		}
	}
}

func TestTheRuleAnswersAFieldAskedOnceOrApart(t *testing.T) {
	t.Parallel()

	for name, asked := range map[string]askedDocument{
		"one field under two type conditions": {document: `{ search {
			... on Item { asItem: name } ... on Book { asBook: name }
		} }`},
		"different arguments": {document: `{ first: item(id: "1") { name } second: item(id: "2") { name } }`},
		"different nested arguments": {document: `{ shelf {
			low: item(rank: 1) { name } high: item(rank: 2) { name }
		} }`},
		"a skipped field":                 {document: `{ shelf { label hidden: label @skip(if: true) } }`},
		"a field left out by its include": {document: `{ shelf { label hidden: label @include(if: false) } }`},
		"a field skipped by a variable": {
			document:  `query($hide: Boolean!) { shelf { label hidden: label @skip(if: $hide) } }`,
			variables: map[string]any{"hide": true},
		},
		"a skipped spread": {document: `{ shelf { label ...Hidden @skip(if: true) } }
			fragment Hidden on Shelf { hidden: label }`},
		"one response name twice":       {document: `{ shelf { label label } }`},
		"one alias twice":               {document: `{ shelf { same: label same: label } }`},
		"one spread twice":              {document: `{ shelf { ...Once ...Once } } fragment Once on Shelf { once: label }`},
		"the same name in two branches": {document: `{ shelf { item(rank: 1) { name } } item(id: "1") { name } }`},
		"two lists of different values": {
			document: `{ few: tagged(tags: ["new"]) { name } more: tagged(tags: ["new", "old"]) { name } }`,
		},
		"a single value and a list of another": {
			document: `{ once: tagged(tags: "new") { name } twice: tagged(tags: ["old"]) { name } }`,
		},
		"a null argument and its default": {document: `{ nulled: items(first: null) { name } sized: items { name } }`},
		"a null inner list and a list of null": {
			document: `{ once: matrix(rows: [null]) { name } twice: matrix(rows: [[null]]) { name } }`,
		},
		"two uploads alike in name, size and type": {
			document:  twoUploadsDocument,
			variables: map[string]any{"file": uploadOf(uploadedBytes), "another": uploadOf("c,d\n")},
		},
		"two unreadable uploads alike in name, size and type": {
			document: twoUploadsDocument,
			variables: map[string]any{
				"file":    uploadFrom(&unreadableFile{reason: errUnreadable}),
				"another": uploadFrom(&unreadableFile{reason: errUnreadable}),
			},
		},
		"two unseekable uploads alike in name, size, type and bytes": {
			document: twoUploadsDocument,
			variables: map[string]any{
				"file":    uploadFrom(&unseekableFile{strings.NewReader(uploadedBytes)}),
				"another": uploadFrom(&unseekableFile{strings.NewReader(uploadedBytes)}),
			},
		},
	} {
		if got := repeatedName(t, asked); got != nil {
			t.Errorf("%s: the rule rejected %v, want it answered", name, got)
		}
	}
}

func TestTheRuleLeavesEveryUploadToBeReadInFullFromItsStart(t *testing.T) {
	t.Parallel()

	file, another := uploadOf(uploadedBytes), uploadFrom(&interruptedFile{contents: strings.NewReader(uploadedBytes)})

	repeatedName(t, askedDocument{
		document: twoUploadsDocument, variables: map[string]any{"file": file, "another": another},
	})

	for name, upload := range map[string]graphql.Upload{"the readable upload": file, "the interrupted upload": another} {
		read, err := io.ReadAll(upload.File)
		if err != nil || string(read) != uploadedBytes {
			t.Errorf("%s read %q, %v after the rule, want %q", name, read, err, uploadedBytes)
		}
	}
}

func TestTheRuleRejectsWithTheValidationReasonNamingTheField(t *testing.T) {
	t.Parallel()

	rejected := ruleAnswer(t, gqlparser.MustLoadSchema(&ast.Source{Input: shelfSchema}),
		askedDocument{document: `{ shelf { once: label twice: label } }`})
	if rejected == nil {
		t.Fatal("the rule answered a field asked under two names, want it rejected")
	}
	presented := graphres.PresentError(t.Context(), rejected)

	if got := code(t, presented); got != "VALIDATION" {
		t.Errorf("code = %q, want VALIDATION", got)
	}
	if got := presented.Extensions["reason"]; got != "repeated_field" {
		t.Errorf("reason = %v, want repeated_field", got)
	}
	meta, _ := presented.Extensions["meta"].(map[string]any)
	if !maps.Equal(meta, map[string]any{"field": "Shelf.label"}) {
		t.Errorf("meta = %v, want the field Shelf.label", presented.Extensions["meta"])
	}
	if want := "graph: the field Shelf.label is asked under the names once and twice"; presented.Message != want {
		t.Errorf("message = %q, want %q", presented.Message, want)
	}
}

func TestTheRepeatedFieldRuleRejectsAtTheEndpoint(t *testing.T) {
	t.Parallel()

	resolver := &graphres.Resolver{Version: "9.9.9", Contacts: &stubContactStore{}, Tasks: &stubTaskStore{}}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	response, err := client.RawPost(`{ once: version twice: version }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorReason(t, response.Errors); got != "repeated_field" {
		t.Errorf("reason = %q, want repeated_field", got)
	}
}

func TestTheRuleNamesItselfAndTakesAnySchema(t *testing.T) {
	t.Parallel()

	rule := graphres.RepeatedFields{}

	if got := rule.ExtensionName(); got != "RepeatedFields" {
		t.Errorf("ExtensionName() = %q, want RepeatedFields", got)
	}
	if err := rule.Validate(graphres.ExecutableSchema(nil)); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestTheShippedDocumentsThatNameAFieldTwiceOrIntrospectPassTheRule(t *testing.T) {
	t.Parallel()

	schema := composedSchema(t)

	for name, document := range map[string]string{
		"the field catalogue":              fieldCatalogueDocument,
		"the GraphiQL introspection query": introspection.Query,
	} {
		if rejected := ruleAnswer(t, schema, askedDocument{document: document}); rejected != nil {
			t.Errorf("%s answered %v, want it passed", name, rejected)
		}
	}
}
