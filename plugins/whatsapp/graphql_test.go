// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// newGraphQLServer returns a gqlgen server over the composed root of p and the core stores.
func newGraphQLServer(t *testing.T, p *whatsapp.Plugin, pool *pgxpool.Pool) *handler.Server {
	t.Helper()
	inert, err := graphroot.All(sdk.Deps{DatabaseURL: "postgres://graph:graph@localhost:1/graph"})
	if err != nil {
		t.Fatalf("graphroot.All() error = %v, want nil", err)
	}
	for _, registered := range inert {
		t.Cleanup(func() { _ = registered.Stop(context.Background()) })
	}
	root, err := graphroot.FromPlugins(&graphres.Resolver{
		Version:  "9.9.9",
		Contacts: postgres.NewContactStore(pool),
	}, append(inert, p))
	if err != nil {
		t.Fatalf("FromPlugins() error = %v, want nil", err)
	}
	srv := handler.New(graphres.ExecutableSchema(root))
	srv.AddTransport(transport.SSE{})
	srv.AddTransport(transport.POST{})
	srv.SetErrorPresenter(graphres.PresentError)
	return srv
}

// newGraphQLClient returns a gqlgen client over the composed root of p and the core stores.
func newGraphQLClient(t *testing.T, p *whatsapp.Plugin, pool *pgxpool.Pool) *gqlclient.Client {
	t.Helper()
	srv := newGraphQLServer(t, p, pool)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.ServeHTTP(w, r.WithContext(sdk.WithRequestScope(r.Context(), sdk.NewRequestScope())))
	})
	return gqlclient.New(wrapped)
}

// newScopelessGraphQLClient returns a client whose requests carry no request scope.
func newScopelessGraphQLClient(t *testing.T, p *whatsapp.Plugin, pool *pgxpool.Pool) *gqlclient.Client {
	t.Helper()
	return gqlclient.New(newGraphQLServer(t, p, pool))
}

// newTenantGraphQLClient returns a client whose requests serve one tenant.
func newTenantGraphQLClient(
	t *testing.T, p *whatsapp.Plugin, pool *pgxpool.Pool, tenantID uuid.UUID,
) *gqlclient.Client {
	t.Helper()
	srv := newGraphQLServer(t, p, pool)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served := sdk.WithRequestScope(sdk.WithTenant(r.Context(), tenantID), sdk.NewRequestScope())
		srv.ServeHTTP(w, r.WithContext(served))
	})
	return gqlclient.New(wrapped)
}

// newNumberedPlugin returns a migrated plugin that can store numbers, with its assertion pool.
func newNumberedPlugin(t *testing.T) (*whatsapp.Plugin, *pgxpool.Pool) {
	t.Helper()
	p, pool := newMessagingPlugin(t)
	p.UseEncrypter(whatsapp.BoundEncrypter())
	return p, pool
}

// connectLabelledNumber stores a number under the tenant with its label.
func connectLabelledNumber(t *testing.T, p *whatsapp.Plugin, tenantID uuid.UUID, identity sdk.ChannelIdentity) {
	t.Helper()
	stored := sdk.ChannelCredentials{
		Identifier: identity.Identifier, Label: identity.Label, Secret: "EAAG-" + identity.Identifier,
	}
	if err := p.SetChannelCredentials(sdk.WithTenant(t.Context(), tenantID), stored); err != nil {
		t.Fatalf("SetChannelCredentials() error = %v, want nil", err)
	}
}

// conversationOnNumber names a customer's conversation in a tenant on a number, nil for none.
type conversationOnNumber struct {
	tenantID uuid.UUID
	number   *string
	customer string
}

// seedNumberedConversation stores a contact's conversation of the tenant on a number, nil for none.
func seedNumberedConversation(t *testing.T, pool *pgxpool.Pool, seeded conversationOnNumber) uuid.UUID {
	t.Helper()
	contactID := uuid.Must(uuid.NewV7())
	conversationID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, 'Maria Perez', now(), $2)`,
		contactID, seeded.tenantID,
	); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO plugin_whatsapp.conversations (id, contact_id, channel, external_id, status,
			last_activity_at, created_at, tenant_id, phone_number_id)
		VALUES ($1, $2, 'whatsapp', $3, 'open', now(), now(), $4, $5)`,
		conversationID, contactID, seeded.customer, seeded.tenantID, seeded.number,
	); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	return conversationID
}

// newMessagingPlugin returns a migrated plugin with its assertion pool.
func newMessagingPlugin(t *testing.T) (*whatsapp.Plugin, *pgxpool.Pool) {
	t.Helper()
	address := newTestDatabase(t)
	pool := newAssertionPool(t, address)
	p := newPlugin(t, address, nil, nil)
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return p, pool
}

// seedGraphContact stores a contact row and returns its id.
func seedGraphContact(t *testing.T, pool *pgxpool.Pool, name string, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)`,
		id, name, createdAt,
	); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	return id
}

// seedGraphConversation stores a conversation row and returns its id.
func seedGraphConversation(
	t *testing.T, pool *pgxpool.Pool, contactID uuid.UUID, externalID string, lastActivityAt time.Time,
) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO plugin_whatsapp.conversations (id, contact_id, channel, external_id, status,
			last_activity_at, created_at)
		VALUES ($1, $2, 'whatsapp', $3, 'open', $4, $4)`,
		id, contactID, externalID, lastActivityAt,
	); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	return id
}

// seedGraphMessage stores a message row and returns its id.
func seedGraphMessage(
	t *testing.T, pool *pgxpool.Pool, conversationID uuid.UUID, externalID, content, contentType string,
	sentAt time.Time,
) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at)
		VALUES ($1, $2, $3, 'inbound', $4, $5, $6, '{}', $6)`,
		id, conversationID, externalID, content, contentType, sentAt,
	); err != nil {
		t.Fatalf("inserting message: %v", err)
	}
	return id
}

// graphConversation is one conversation node of a graph response.
type graphConversation struct {
	ID                 string
	ExternalID         string
	Status             string
	LastMessagePreview *string
	Contact            struct {
		ID         string
		Name       string
		CreatedAt  string
		Identities []struct {
			Identifier string
		}
	}
	Messages []graphMessage
}

// graphMessage is one message node of a graph response.
type graphMessage struct {
	ID          string
	ExternalID  string
	Direction   string
	Content     string
	ContentType string
	Media       *struct {
		Status       string
		MimeType     string
		Filename     *string
		FileSize     *int
		Voice        bool
		Animated     bool
		DownloadPath string
	}
}

func TestGraphConversationsListWithContactAndPreview(t *testing.T) {
	t.Parallel()

	p, pool := newMessagingPlugin(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	maria := seedGraphContact(t, pool, "María Pérez", now.Add(-time.Hour))
	older := seedGraphConversation(t, pool, maria, "184467235@lid", now.Add(-time.Minute))
	newer := seedGraphConversation(t, pool, maria, "184467236@lid", now)
	seedGraphMessage(t, pool, older, "wamid.text", "hello there", "text", now.Add(-time.Minute))
	seedGraphMessage(t, pool, newer, "wamid.photo", "", "image", now)
	client := newGraphQLClient(t, p, pool)

	var response struct{ WhatsAppConversations []graphConversation }
	client.MustPost(`{ whatsAppConversations {
		id externalId status lastMessagePreview contact { id name }
	} }`, &response)

	conversations := response.WhatsAppConversations
	if len(conversations) != 2 {
		t.Fatalf("conversations = %d, want 2", len(conversations))
	}
	if conversations[0].ExternalID != "184467236@lid" {
		t.Errorf("first external id = %q, want the most recently active first", conversations[0].ExternalID)
	}
	if preview := conversations[0].LastMessagePreview; preview == nil || *preview != "[photo]" {
		t.Errorf("newer preview = %v, want [photo]", preview)
	}
	if preview := conversations[1].LastMessagePreview; preview == nil || *preview != "hello there" {
		t.Errorf("older preview = %v, want the text content", preview)
	}
	if conversations[0].Contact.ID != maria.String() || conversations[0].Contact.Name != "María Pérez" {
		t.Errorf("contact = %+v, want María's id and name", conversations[0].Contact)
	}
	if conversations[0].Status != "open" {
		t.Errorf("status = %q, want open", conversations[0].Status)
	}
}

func TestGraphConversationMessagesCarryMedia(t *testing.T) {
	t.Parallel()

	p, pool := newMessagingPlugin(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	maria := seedGraphContact(t, pool, "María Pérez", now)
	conversation := seedGraphConversation(t, pool, maria, "184467235@lid", now)
	seedGraphMessage(t, pool, conversation, "wamid.text", "hello", "text", now.Add(-time.Second))
	voice := seedGraphMessage(t, pool, conversation, "wamid.voice", "", "audio", now)
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO plugin_whatsapp.media (message_id, media_id, status, mime_type, sha256, filename,
			file_size, voice, animated, next_attempt_at, created_at)
		VALUES ($1, 'MEDIA1', 'stored', 'audio/ogg', 'c2hh', NULL, 4096, TRUE, FALSE, $2, $2)`,
		voice, now,
	); err != nil {
		t.Fatalf("inserting media: %v", err)
	}
	client := newGraphQLClient(t, p, pool)

	var response struct{ WhatsAppConversations []graphConversation }
	client.MustPost(`{ whatsAppConversations { id messages {
		id externalId direction content contentType
		media { status mimeType filename fileSize voice animated downloadPath }
	} } }`, &response)

	if len(response.WhatsAppConversations) != 1 {
		t.Fatalf("conversations = %d, want 1", len(response.WhatsAppConversations))
	}
	messages := response.WhatsAppConversations[0].Messages
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2 oldest first", len(messages))
	}
	if messages[0].ExternalID != "wamid.text" || messages[0].Media != nil {
		t.Errorf("first message = %+v, want the plain text without media", messages[0])
	}
	media := messages[1].Media
	if media == nil {
		t.Fatal("voice message media = nil, want the stored descriptor")
	}
	if media.Status != "stored" || media.MimeType != "audio/ogg" || !media.Voice || media.Animated {
		t.Errorf("media = %+v, want the stored voice descriptor", media)
	}
	if media.FileSize == nil || *media.FileSize != 4096 {
		t.Errorf("file size = %v, want 4096", media.FileSize)
	}
	wantPath := "/api/plugins/whatsapp/conversations/" + conversation.String() +
		"/messages/" + messages[1].ID + "/media"
	if media.DownloadPath != wantPath {
		t.Errorf("download path = %q, want %q", media.DownloadPath, wantPath)
	}
}

func TestGraphContactEdgeGroupsConversationsPerContact(t *testing.T) {
	t.Parallel()

	p, pool := newMessagingPlugin(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	maria := seedGraphContact(t, pool, "María Pérez", now.Add(-2*time.Hour))
	noah := seedGraphContact(t, pool, "Noah Chen", now.Add(-time.Hour))
	quiet := seedGraphContact(t, pool, "Quiet Contact", now)
	seedGraphConversation(t, pool, maria, "184467235@lid", now)
	seedGraphConversation(t, pool, noah, "184467236@lid", now)
	client := newGraphQLClient(t, p, pool)

	var response struct {
		Contacts struct {
			Edges []struct {
				Node struct {
					ID                    string
					WhatsAppConversations []struct{ ExternalID string }
				}
			}
		}
	}
	client.MustPost(`{ contacts(first: 50) { edges { node {
		id whatsAppConversations { externalId }
	} } } }`, &response)

	conversationsOf := map[string][]string{}
	for _, edge := range response.Contacts.Edges {
		for _, conversation := range edge.Node.WhatsAppConversations {
			conversationsOf[edge.Node.ID] = append(conversationsOf[edge.Node.ID], conversation.ExternalID)
		}
	}
	if got := conversationsOf[maria.String()]; len(got) != 1 || got[0] != "184467235@lid" {
		t.Errorf("maria conversations = %v, want her single conversation", got)
	}
	if got := conversationsOf[noah.String()]; len(got) != 1 || got[0] != "184467236@lid" {
		t.Errorf("noah conversations = %v, want his single conversation", got)
	}
	if got := conversationsOf[quiet.String()]; len(got) != 0 {
		t.Errorf("quiet contact conversations = %v, want none", got)
	}
}

func TestGraphConversationContactRepairsCoreFields(t *testing.T) {
	t.Parallel()

	p, pool := newMessagingPlugin(t)
	createdAt := time.Date(2026, 7, 1, 10, 30, 0, 0, time.UTC)
	maria := seedGraphContact(t, pool, "María Pérez", createdAt)
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at)
		VALUES ($1, $2, 'whatsapp', '184467235', 'María Pérez', $3)`,
		uuid.Must(uuid.NewV7()), maria, createdAt,
	); err != nil {
		t.Fatalf("inserting identity: %v", err)
	}
	seedGraphConversation(t, pool, maria, "184467235@lid", createdAt)
	client := newGraphQLClient(t, p, pool)

	var response struct{ WhatsAppConversations []graphConversation }
	client.MustPost(`{ whatsAppConversations { contact {
		id name createdAt identities { identifier }
	} } }`, &response)

	if len(response.WhatsAppConversations) != 1 {
		t.Fatalf("conversations = %d, want 1", len(response.WhatsAppConversations))
	}
	contact := response.WhatsAppConversations[0].Contact
	parsed, err := time.Parse(time.RFC3339, contact.CreatedAt)
	if err != nil {
		t.Fatalf("parsing createdAt %q: %v", contact.CreatedAt, err)
	}
	if !parsed.Equal(createdAt) {
		t.Errorf("createdAt = %v, want the stored %v repaired through the core loader", parsed, createdAt)
	}
	if len(contact.Identities) != 1 || contact.Identities[0].Identifier != "184467235" {
		t.Errorf("identities = %+v, want the whatsapp identity", contact.Identities)
	}
}

// firstGraphError returns the first error's message and extensions of a raw post.
func firstGraphError(t *testing.T, client *gqlclient.Client, query string, options ...gqlclient.Option) (
	string, map[string]any,
) {
	t.Helper()
	response, err := client.RawPost(query, options...)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	var parsed []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	}
	if err := json.Unmarshal(response.Errors, &parsed); err != nil || len(parsed) == 0 {
		t.Fatalf("errors = %s, want at least one", response.Errors)
	}
	return parsed[0].Message, parsed[0].Extensions
}

func TestGraphConversationReportsAReadFailure(t *testing.T) {
	t.Parallel()

	p, _ := newMessagingPlugin(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	conversation, err := p.QueryResolvers().WhatsAppConversation(ctx, uuid.Must(uuid.NewV7()))

	if conversation != nil {
		t.Errorf("conversation = %+v, want none when the read failed", conversation)
	}
	if err == nil {
		t.Fatal("WhatsAppConversation() error = nil, want the read failure reported")
	}
}

func TestGraphListLimitsAreValidated(t *testing.T) {
	t.Parallel()

	p, pool := newMessagingPlugin(t)
	now := time.Now().UTC()
	maria := seedGraphContact(t, pool, "María Pérez", now)
	seedGraphConversation(t, pool, maria, "184467235@lid", now)
	client := newGraphQLClient(t, p, pool)

	queries := map[string]string{
		"zero conversations limit": `{ whatsAppConversations(limit: 0) { id } }`,
		"oversized messages limit": `{ whatsAppConversations { messages(limit: 201) { id } } }`,
	}
	for name, query := range queries {
		if _, extensions := firstGraphError(t, client, query); extensions["code"] != "VALIDATION" {
			t.Errorf("%s code = %v, want VALIDATION", name, extensions["code"])
		}
	}
}

func TestGraphSendMessageDeliversAndPersists(t *testing.T) {
	t.Parallel()

	p, stub, pool := newSendingHarness(t, nil)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	conversationID := onlyConversation(t, p).ID
	client := newGraphQLClient(t, p, pool)

	var response struct{ WhatsAppSendMessage graphMessage }
	client.MustPost(
		`mutation($id: UUID!) { whatsAppSendMessage(conversationId: $id, content: "Ready at 5pm") {
			id externalId direction content contentType
		} }`,
		&response,
		gqlclient.Var("id", conversationID.String()),
	)

	sent := response.WhatsAppSendMessage
	if sent.Direction != "outbound" || sent.Content != "Ready at 5pm" || sent.ExternalID != "wamid.out.1" {
		t.Errorf("message = %+v, want the outbound reply delivered as wamid.out.1", sent)
	}
	if stub.lastPath != "/555000222/messages" {
		t.Errorf("graph path = %q, want %q", stub.lastPath, "/555000222/messages")
	}
	var stored int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM plugin_whatsapp.messages WHERE conversation_id = $1 AND direction = 'outbound'`,
		conversationID,
	).Scan(&stored); err != nil {
		t.Fatalf("counting outbound messages: %v", err)
	}
	if stored != 1 {
		t.Errorf("outbound messages = %d, want the reply persisted", stored)
	}
}

func TestGraphSendMessageClassifiesFailures(t *testing.T) {
	t.Parallel()

	p, stub, pool := newSendingHarness(t, nil)
	routes := p.Routes()
	ingestEvent(t, routes, "wamid.1", "184467235", "María Pérez", "1751791000", "hello")
	conversationID := onlyConversation(t, p).ID
	client := newGraphQLClient(t, p, pool)
	sendDocument := `mutation($id: UUID!, $content: String!) {
		whatsAppSendMessage(conversationId: $id, content: $content) { id }
	}`

	t.Run("unknown conversation", func(t *testing.T) {
		_, extensions := firstGraphError(t, client, sendDocument,
			gqlclient.Var("id", uuid.Must(uuid.NewV7()).String()), gqlclient.Var("content", "hey"))
		if extensions["code"] != "NOT_FOUND" {
			t.Errorf("code = %v, want NOT_FOUND", extensions["code"])
		}
	})

	t.Run("blank content", func(t *testing.T) {
		_, extensions := firstGraphError(t, client, sendDocument,
			gqlclient.Var("id", conversationID.String()), gqlclient.Var("content", " \t "))
		if extensions["code"] != "VALIDATION" {
			t.Errorf("code = %v, want VALIDATION", extensions["code"])
		}
	})

	t.Run("meta rejection", func(t *testing.T) {
		stub.status = http.StatusBadRequest
		stub.body = `{"error":{"message":"Re-engagement message","code":131047}}`
		message, extensions := firstGraphError(t, client, sendDocument,
			gqlclient.Var("id", conversationID.String()), gqlclient.Var("content", "hey"))
		if extensions["code"] != "UPSTREAM" {
			t.Errorf("code = %v, want UPSTREAM", extensions["code"])
		}
		if metaCode, _ := extensions["metaCode"].(float64); metaCode != 131047 {
			t.Errorf("metaCode = %v, want 131047", extensions["metaCode"])
		}
		if message == "" {
			t.Error("message is empty, want the rejection surfaced")
		}
	})
}

func TestGraphQueriesSurfaceStoreFailures(t *testing.T) {
	t.Parallel()

	p, pool := newMessagingPlugin(t)
	now := time.Now().UTC()
	maria := seedGraphContact(t, pool, "Maria Perez", now)
	conversationID := seedGraphConversation(t, pool, maria, "184467235@lid", now)
	client := newGraphQLClient(t, p, pool)

	if _, err := pool.Exec(t.Context(), `DROP TABLE plugin_whatsapp.messages CASCADE`); err != nil {
		t.Fatalf("dropping the messages table: %v", err)
	}
	conversation := &model.WhatsAppConversation{ID: conversationID}
	if _, err := p.WhatsAppConversationResolvers().Messages(t.Context(), conversation, nil); err == nil {
		t.Error("Messages() on a dropped table error = nil, want error")
	}

	if _, err := pool.Exec(t.Context(), `DROP TABLE plugin_whatsapp.conversations CASCADE`); err != nil {
		t.Fatalf("dropping the conversations table: %v", err)
	}
	_, extensions := firstGraphError(t, client, `{ whatsAppConversations { id } }`)
	if extensions["code"] != "INTERNAL" {
		t.Errorf("conversations code = %v, want INTERNAL", extensions["code"])
	}
	_, extensions = firstGraphError(t, client, `{ contacts { edges { node { whatsAppConversations { id } } } } }`)
	if extensions["code"] != "INTERNAL" {
		t.Errorf("contact conversations code = %v, want INTERNAL", extensions["code"])
	}
}

func TestGraphContactConversationsRequireTheRequestScope(t *testing.T) {
	t.Parallel()

	p, pool := newMessagingPlugin(t)
	seedGraphContact(t, pool, "Maria Perez", time.Now().UTC())
	client := newScopelessGraphQLClient(t, p, pool)

	_, extensions := firstGraphError(t, client, `{ contacts { edges { node { whatsAppConversations { id } } } } }`)
	if extensions["code"] != "INTERNAL" {
		t.Errorf("code = %v, want INTERNAL", extensions["code"])
	}
}

func TestGraphReadsOneConversationWithItsMessages(t *testing.T) {
	t.Parallel()

	p, pool := newMessagingPlugin(t)
	contactID := seedGraphContact(t, pool, "Maria Perez", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC))
	conversationID := seedGraphConversation(
		t, pool, contactID, "184467235", time.Date(2026, 7, 6, 10, 5, 0, 0, time.UTC),
	)
	seedGraphMessage(
		t, pool, conversationID, "wamid.1", "Hi, is the order ready?", "text",
		time.Date(2026, 7, 6, 9, 5, 0, 0, time.UTC),
	)
	client := newGraphQLClient(t, p, pool)

	var response struct{ WhatsAppConversation *graphConversation }
	client.MustPost(
		`query($id: UUID!) { whatsAppConversation(id: $id) {
			id status contact { id name } messages { id content contentType }
		} }`,
		&response,
		gqlclient.Var("id", conversationID.String()),
	)

	found := response.WhatsAppConversation
	if found == nil || found.ID != conversationID.String() {
		t.Fatalf("conversation = %+v, want %s", found, conversationID)
	}
	if found.Contact.Name != "Maria Perez" {
		t.Errorf("contact name = %q, want Maria Perez", found.Contact.Name)
	}
	if len(found.Messages) != 1 || found.Messages[0].Content != "Hi, is the order ready?" {
		t.Errorf("messages = %+v, want the seeded arrival", found.Messages)
	}
}

func TestGraphReadsNoConversationForAnUnknownID(t *testing.T) {
	t.Parallel()

	p, pool := newMessagingPlugin(t)
	client := newGraphQLClient(t, p, pool)

	var response struct{ WhatsAppConversation *graphConversation }
	client.MustPost(
		`query($id: UUID!) { whatsAppConversation(id: $id) { id } }`,
		&response,
		gqlclient.Var("id", uuid.Must(uuid.NewV7()).String()),
	)

	if response.WhatsAppConversation != nil {
		t.Errorf("conversation = %+v, want none", response.WhatsAppConversation)
	}
}

// graphNumber is one connected number of a graph response.
type graphNumber struct {
	Identifier string
	Label      string
}

// numbersResponse is the answer to the connected numbers query.
type numbersResponse struct{ WhatsAppNumbers []graphNumber }

// numberedConversation is one conversation of a graph response beside the number it came in on.
type numberedConversation struct {
	ExternalID string
	Number     *string
}

// conversationNumbers reads every listed conversation's number by its external id.
func conversationNumbers(conversations []numberedConversation) map[string]*string {
	numbers := map[string]*string{}
	for _, conversation := range conversations {
		numbers[conversation.ExternalID] = conversation.Number
	}
	return numbers
}

func TestGraphNumbersListTheTenantsOwnNumbersOldestFirst(t *testing.T) {
	t.Parallel()

	p, pool := newNumberedPlugin(t)
	acme := seedTenant(t, pool)
	connectLabelledNumber(t, p, acme, sdk.ChannelIdentity{Identifier: "5550002", Label: "Sales line"})
	connectLabelledNumber(t, p, acme, sdk.ChannelIdentity{Identifier: "5550001", Label: ""})
	connectLabelledNumber(t, p, seedGlobex(t, pool), sdk.ChannelIdentity{Identifier: "5550003", Label: "Support line"})
	client := newTenantGraphQLClient(t, p, pool, acme)

	var response numbersResponse
	client.MustPost(`{ whatsAppNumbers { identifier label } }`, &response)

	want := []graphNumber{{Identifier: "5550002", Label: "Sales line"}, {Identifier: "5550001"}}
	if !slices.Equal(response.WhatsAppNumbers, want) {
		t.Errorf("numbers = %+v, want the tenant's own %+v", response.WhatsAppNumbers, want)
	}
}

func TestGraphNumbersAnswerTheEnvironmentNumberWhenNoneIsStored(t *testing.T) {
	t.Parallel()

	p, _, pool := newSendingHarness(t, nil)
	client := newGraphQLClient(t, p, pool)

	var response numbersResponse
	client.MustPost(`{ whatsAppNumbers { identifier label } }`, &response)

	if want := []graphNumber{{Identifier: "555000222"}}; !slices.Equal(response.WhatsAppNumbers, want) {
		t.Errorf("numbers = %+v, want the environment number %+v", response.WhatsAppNumbers, want)
	}
}

func TestGraphNumberNamesNoSecret(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("graph", "schema.graphqls"))
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	schema, err := parser.ParseSchema(&ast.Source{Name: "schema.graphqls", Input: string(raw)})
	if err != nil {
		t.Fatalf("parsing the schema: %v", err)
	}

	number := schema.Definitions.ForName("WhatsAppNumber")

	if number == nil {
		t.Fatal("WhatsAppNumber is not in the schema")
	}
	fields := make([]string, 0, len(number.Fields))
	for _, field := range number.Fields {
		fields = append(fields, field.Name)
	}
	if want := []string{"identifier", "label"}; !slices.Equal(fields, want) {
		t.Errorf("WhatsAppNumber fields = %v, want only %v", fields, want)
	}
}

func TestGraphNumbersSurfaceAStoreFailure(t *testing.T) {
	t.Parallel()

	p, pool := newNumberedPlugin(t)
	client := newGraphQLClient(t, p, pool)
	if _, err := pool.Exec(t.Context(), `DROP TABLE plugin_whatsapp.credentials CASCADE`); err != nil {
		t.Fatalf("dropping the credentials table: %v", err)
	}

	_, extensions := firstGraphError(t, client, `{ whatsAppNumbers { identifier } }`)

	if extensions["code"] != "INTERNAL" {
		t.Errorf("code = %v, want INTERNAL", extensions["code"])
	}
}

// wantConversationNumbers fails the test unless each conversation reads the number named for it.
func wantConversationNumbers(t *testing.T, numbers map[string]*string, want map[string]string) {
	t.Helper()
	for externalID, number := range want {
		if got := numbers[externalID]; got == nil || *got != number {
			t.Errorf("conversation %s number = %v, want %q", externalID, got, number)
		}
	}
}

func TestGraphConversationNumberReadsTheLabelOrElseTheIdentifier(t *testing.T) {
	t.Parallel()

	p, pool := newNumberedPlugin(t)
	acme := seedTenant(t, pool)
	labelled, bare, removed := "5550001", "5550002", "5550003"
	connectLabelledNumber(t, p, acme, sdk.ChannelIdentity{Identifier: labelled, Label: "Sales line"})
	connectLabelledNumber(t, p, acme, sdk.ChannelIdentity{Identifier: bare, Label: ""})
	connectLabelledNumber(t, p, acme, sdk.ChannelIdentity{Identifier: removed, Label: "Old line"})
	seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: &labelled, customer: "184467235"})
	seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: &bare, customer: "184467236"})
	seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: &removed, customer: "184467237"})
	single := seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: nil, customer: "184467238"})
	if err := p.RemoveChannelCredentials(sdk.WithTenant(t.Context(), acme), removed); err != nil {
		t.Fatalf("RemoveChannelCredentials() error = %v, want nil", err)
	}
	client := newTenantGraphQLClient(t, p, pool, acme)

	var listed struct{ WhatsAppConversations []numberedConversation }
	client.MustPost(`{ whatsAppConversations { externalId number } }`, &listed)
	var one struct{ WhatsAppConversation numberedConversation }
	client.MustPost(`query($id: UUID!) { whatsAppConversation(id: $id) { externalId number } }`, &one,
		gqlclient.Var("id", single.String()))

	numbers := conversationNumbers(listed.WhatsAppConversations)
	wantConversationNumbers(t, numbers, map[string]string{
		"184467235": "Sales line",
		"184467236": bare,
		"184467237": removed,
	})
	if got, found := numbers["184467238"]; !found || got != nil {
		t.Errorf("conversation with no number listed as %v, want null", got)
	}
	if one.WhatsAppConversation.ExternalID != "184467238" || one.WhatsAppConversation.Number != nil {
		t.Errorf("conversation = %+v, want the one read to carry no number", one.WhatsAppConversation)
	}
}

func TestGraphConversationNumberReadsTheLabelOfTheContactEdge(t *testing.T) {
	t.Parallel()

	p, pool := newNumberedPlugin(t)
	acme := seedTenant(t, pool)
	number := "5550001"
	connectLabelledNumber(t, p, acme, sdk.ChannelIdentity{Identifier: number, Label: "Sales line"})
	seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: &number, customer: "184467235"})
	client := newTenantGraphQLClient(t, p, pool, acme)

	var response struct {
		Contacts struct {
			Edges []struct {
				Node struct{ WhatsAppConversations []numberedConversation }
			}
		}
	}
	client.MustPost(`{ contacts(first: 50) { edges { node { whatsAppConversations { externalId number } } } } }`,
		&response)

	if len(response.Contacts.Edges) != 1 {
		t.Fatalf("contacts = %d, want 1", len(response.Contacts.Edges))
	}
	numbers := conversationNumbers(response.Contacts.Edges[0].Node.WhatsAppConversations)
	if got := numbers["184467235"]; got == nil || *got != "Sales line" {
		t.Errorf("number = %v, want the label", got)
	}
}

// conversationNumberReads returns the number the tenant's only conversation answers on every graph path.
func conversationNumberReads(
	t *testing.T, client *gqlclient.Client, conversationID uuid.UUID,
) map[string]*string {
	t.Helper()
	var listed struct{ WhatsAppConversations []numberedConversation }
	client.MustPost(`{ whatsAppConversations { externalId number } }`, &listed)
	var one struct{ WhatsAppConversation numberedConversation }
	client.MustPost(`query($id: UUID!) { whatsAppConversation(id: $id) { externalId number } }`, &one,
		gqlclient.Var("id", conversationID.String()))
	var edge struct {
		Contacts struct {
			Edges []struct {
				Node struct{ WhatsAppConversations []numberedConversation }
			}
		}
	}
	client.MustPost(`{ contacts(first: 50) { edges { node { whatsAppConversations { externalId number } } } } }`, &edge)
	if len(listed.WhatsAppConversations) != 1 || len(edge.Contacts.Edges) != 1 {
		t.Fatalf("read %+v and %+v, want the one conversation on each path", listed, edge)
	}
	return map[string]*string{
		"the list":         listed.WhatsAppConversations[0].Number,
		"the conversation": one.WhatsAppConversation.Number,
		"the contact":      edge.Contacts.Edges[0].Node.WhatsAppConversations[0].Number,
	}
}

func TestGraphConversationOnNoNumberAnswersTheEnvironmentNumber(t *testing.T) {
	t.Parallel()

	p, _, pool := newSendingHarness(t, nil)
	seeded := conversationOnNumber{tenantID: sdk.DefaultTenantID, number: nil, customer: "184467235"}
	conversationID := seedNumberedConversation(t, pool, seeded)
	client := newGraphQLClient(t, p, pool)

	for path, got := range conversationNumberReads(t, client, conversationID) {
		if got == nil || *got != "555000222" {
			t.Errorf("%s answers %v, want the environment number", path, got)
		}
	}
}

func TestGraphConversationOnNoNumberAnswersNullElsewhere(t *testing.T) {
	t.Parallel()

	tests := map[string]func(t *testing.T) (*gqlclient.Client, uuid.UUID){
		"another tenant beside an environment number": func(t *testing.T) (*gqlclient.Client, uuid.UUID) {
			t.Helper()
			p, _, pool := newSendingHarness(t, nil)
			acme := seedTenant(t, pool)
			seeded := conversationOnNumber{tenantID: acme, number: nil, customer: "184467235"}
			return newTenantGraphQLClient(t, p, pool, acme), seedNumberedConversation(t, pool, seeded)
		},
		"the default tenant with no environment number": func(t *testing.T) (*gqlclient.Client, uuid.UUID) {
			t.Helper()
			p, pool := newNumberedPlugin(t)
			seeded := conversationOnNumber{tenantID: sdk.DefaultTenantID, number: nil, customer: "184467235"}
			return newGraphQLClient(t, p, pool), seedNumberedConversation(t, pool, seeded)
		},
	}
	for testName, arrange := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			client, conversationID := arrange(t)

			for path, got := range conversationNumberReads(t, client, conversationID) {
				if got != nil {
					t.Errorf("%s answers %q, want null", path, *got)
				}
			}
		})
	}
}

func TestGraphNumbersListTheEnvironmentNumberBesideAStoredOne(t *testing.T) {
	t.Parallel()

	p, _, pool := newSendingHarness(t, nil)
	p.UseEncrypter(whatsapp.BoundEncrypter())
	connectLabelledNumber(t, p, sdk.DefaultTenantID, sdk.ChannelIdentity{Identifier: "5550001", Label: "Sales line"})
	client := newGraphQLClient(t, p, pool)

	var response numbersResponse
	client.MustPost(`{ whatsAppNumbers { identifier label } }`, &response)

	want := []graphNumber{{Identifier: "555000222"}, {Identifier: "5550001", Label: "Sales line"}}
	if !slices.Equal(response.WhatsAppNumbers, want) {
		t.Errorf("numbers = %+v, want %+v", response.WhatsAppNumbers, want)
	}
}

func TestGraphConversationsFilterByNumber(t *testing.T) {
	t.Parallel()

	p, pool := newNumberedPlugin(t)
	acme := seedTenant(t, pool)
	sales, support := "5550001", "5550002"
	connectLabelledNumber(t, p, acme, sdk.ChannelIdentity{Identifier: sales, Label: "Sales line"})
	connectLabelledNumber(t, p, acme, sdk.ChannelIdentity{Identifier: support, Label: "Support line"})
	seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: &sales, customer: "184467235"})
	seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: &support, customer: "184467236"})
	seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: nil, customer: "184467237"})
	client := newTenantGraphQLClient(t, p, pool, acme)
	document := `query($number: String) { whatsAppConversations(number: $number) { externalId } }`

	tests := map[string]struct {
		number any
		want   []string
	}{
		"a held number":         {number: sales, want: []string{"184467235"}},
		"no number":             {number: nil, want: []string{"184467235", "184467236", "184467237"}},
		"a number nobody holds": {number: "5550009", want: []string{}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var response struct{ WhatsAppConversations []numberedConversation }
			client.MustPost(document, &response, gqlclient.Var("number", test.number))

			got := slices.Sorted(maps.Keys(conversationNumbers(response.WhatsAppConversations)))
			if !slices.Equal(got, test.want) {
				t.Errorf("conversations = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGraphConversationsFilterOnTheEnvironmentNumberReadsThoseStoringNone(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]string{
		"a number with its token":    nil,
		"a number without its token": {"ALPHONE_WHATSAPP_ACCESS_TOKEN": ""},
	}
	for testName, overrides := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p, _, pool := newSendingHarness(t, overrides)
			p.UseEncrypter(whatsapp.BoundEncrypter())
			environment, other := "555000222", "5550001"
			connectLabelledNumber(t, p, sdk.DefaultTenantID, sdk.ChannelIdentity{Identifier: other, Label: "Sales line"})
			seedNumberedConversation(t, pool, conversationOnNumber{
				tenantID: sdk.DefaultTenantID, number: nil, customer: "184467235",
			})
			seedNumberedConversation(t, pool, conversationOnNumber{
				tenantID: sdk.DefaultTenantID, number: &environment, customer: "184467236",
			})
			seedNumberedConversation(t, pool, conversationOnNumber{
				tenantID: sdk.DefaultTenantID, number: &other, customer: "184467237",
			})
			client := newGraphQLClient(t, p, pool)
			document := `query($number: String) { whatsAppConversations(number: $number) { externalId } }`

			filtered := map[string][]string{
				environment: {"184467235", "184467236"},
				other:       {"184467237"},
				"":          {},
			}
			for number, want := range filtered {
				var response struct{ WhatsAppConversations []numberedConversation }
				client.MustPost(document, &response, gqlclient.Var("number", number))

				got := slices.Sorted(maps.Keys(conversationNumbers(response.WhatsAppConversations)))
				if !slices.Equal(got, want) {
					t.Errorf("conversations on %q = %v, want %v", number, got, want)
				}
			}
		})
	}
}

func TestGraphConversationsFilterLeavesTheEnvironmentNumberToTheDefaultTenant(t *testing.T) {
	t.Parallel()

	p, _, pool := newSendingHarness(t, nil)
	acme := seedTenant(t, pool)
	seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: nil, customer: "184467235"})
	client := newTenantGraphQLClient(t, p, pool, acme)

	var response struct{ WhatsAppConversations []numberedConversation }
	client.MustPost(`query($number: String) { whatsAppConversations(number: $number) { externalId } }`,
		&response, gqlclient.Var("number", "555000222"))

	if len(response.WhatsAppConversations) != 0 {
		t.Errorf("conversations = %+v, want none outside the default tenant", response.WhatsAppConversations)
	}
}

func TestGraphConversationsFilterOnNoNumberReadsNothingWithoutAnEnvironmentNumber(t *testing.T) {
	t.Parallel()

	p, pool := newNumberedPlugin(t)
	seedNumberedConversation(t, pool, conversationOnNumber{
		tenantID: sdk.DefaultTenantID, number: nil, customer: "184467235",
	})
	client := newGraphQLClient(t, p, pool)

	var response struct{ WhatsAppConversations []numberedConversation }
	client.MustPost(`query($number: String) { whatsAppConversations(number: $number) { externalId } }`,
		&response, gqlclient.Var("number", ""))

	if len(response.WhatsAppConversations) != 0 {
		t.Errorf("conversations = %+v, want an empty filter to match no conversation", response.WhatsAppConversations)
	}
}

func TestGraphConversationsFilterStaysInsideTheTenant(t *testing.T) {
	t.Parallel()

	p, pool := newNumberedPlugin(t)
	acme := seedTenant(t, pool)
	globex := seedGlobex(t, pool)
	theirs := "5550001"
	connectLabelledNumber(t, p, acme, sdk.ChannelIdentity{Identifier: theirs, Label: "Sales line"})
	seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: &theirs, customer: "184467235"})
	client := newTenantGraphQLClient(t, p, pool, globex)

	var response struct{ WhatsAppConversations []numberedConversation }
	client.MustPost(`query($number: String) { whatsAppConversations(number: $number) { externalId number } }`,
		&response, gqlclient.Var("number", theirs))

	if len(response.WhatsAppConversations) != 0 {
		t.Errorf("conversations = %+v, want none from another tenant", response.WhatsAppConversations)
	}
}

func TestGraphConversationNumberReadsNoLabelOfAnotherTenant(t *testing.T) {
	t.Parallel()

	p, pool := newNumberedPlugin(t)
	acme := seedTenant(t, pool)
	globex := seedGlobex(t, pool)
	number := "5550001"
	connectLabelledNumber(t, p, globex, sdk.ChannelIdentity{Identifier: number, Label: "Support line"})
	seedNumberedConversation(t, pool, conversationOnNumber{tenantID: acme, number: &number, customer: "184467235"})
	client := newTenantGraphQLClient(t, p, pool, acme)

	var response struct{ WhatsAppConversations []numberedConversation }
	client.MustPost(`{ whatsAppConversations { externalId number } }`, &response)

	got := conversationNumbers(response.WhatsAppConversations)["184467235"]
	if got == nil || *got != number {
		t.Errorf("number = %v, want the identifier and never the label another tenant set", got)
	}
}
