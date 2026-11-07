// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// whatsAppNumbersFeature is the feature many WhatsApp numbers answer to.
const whatsAppNumbersFeature = "../../test/features/features/whatsapp-numbers.feature"

// whatsAppMigrations is the folder the WhatsApp plugin's migrations live in.
const whatsAppMigrations = "../../plugins/whatsapp/migrations"

// whatsAppVersionTable is where the WhatsApp plugin records its migrations.
const whatsAppVersionTable = "plugin_whatsapp.goose_db_version"

// lastVersionBeforeNumbers is the WhatsApp migration an install from before many numbers stands on.
const lastVersionBeforeNumbers = 6

// environmentAppSecret names the setting that holds the fallback app secret.
const environmentAppSecret = "ALPHONE_WHATSAPP_APP_SECRET"

// replacedTokenSuffix marks a token that replaced the one a number was connected with.
const replacedTokenSuffix = "-replaced"

// storeWhatsAppSchema creates the schema the WhatsApp plugin migrates inside.
const storeWhatsAppSchema = "CREATE SCHEMA IF NOT EXISTS plugin_whatsapp"

// conversationOnNumber reads the conversation a customer holds with a number in a workspace.
const conversationOnNumber = `SELECT id FROM plugin_whatsapp.conversations
	WHERE tenant_id = $1 AND external_id = $2 AND phone_number_id IS NOT DISTINCT FROM NULLIF($3, '')`

// storeOldNumber stores a number in the credentials table of an install from before many numbers.
const storeOldNumber = `INSERT INTO plugin_whatsapp.credentials (tenant_id, phone_number_id, access_token)
	VALUES ($1, $2, '\x00')`

// workspaceConversations counts the conversations of a workspace.
const workspaceConversations = "SELECT count(*) FROM plugin_whatsapp.conversations WHERE tenant_id = $1"

// workspaceContacts counts the contacts of a workspace.
const workspaceContacts = "SELECT count(*) FROM core.contacts WHERE tenant_id = $1"

// statusOfMessage reads the delivery status a message holds, empty for none.
const statusOfMessage = "SELECT COALESCE(status, '') FROM plugin_whatsapp.messages WHERE external_id = $1"

// replyIDFormat spells the message id the stand-in for the Graph API gives a reply.
const replyIDFormat = "wamid.reply.%d"

// sentReply is one reply the stand-in for the Graph API received.
type sentReply struct {
	path          string
	authorization string
	body          string
}

// replyRecorder stands in for the Graph API, keeping every reply it receives.
type replyRecorder struct {
	mu      sync.Mutex
	replies []sentReply
	server  *httptest.Server
}

// newReplyRecorder starts a stand-in for the Graph API.
func newReplyRecorder() *replyRecorder {
	recorder := &replyRecorder{}
	recorder.server = httptest.NewServer(http.HandlerFunc(recorder.answer))
	return recorder
}

// answer keeps one reply and answers its message id.
func (r *replyRecorder) answer(w http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replies = append(r.replies, sentReply{
		path: request.URL.Path, authorization: request.Header.Get("Authorization"), body: string(body),
	})
	_, _ = fmt.Fprintf(w, `{"messages":[{"id":%q}]}`, fmt.Sprintf(replyIDFormat, len(r.replies)))
}

// last answers the newest reply and how many arrived.
func (r *replyRecorder) last() (sentReply, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.replies) == 0 {
		return sentReply{}, 0
	}
	return r.replies[len(r.replies)-1], len(r.replies)
}

// numbersScenario is what one WhatsApp numbers scenario keeps between its steps.
type numbersScenario struct {
	tokensScenario
	replies   *replyRecorder
	plugin    *whatsapp.Plugin
	answered  int
	delivered int
	read      []string
	opened    conversationKey
}

// initializeWhatsAppNumbers returns the binding of the WhatsApp number steps, t holding their databases.
func initializeWhatsAppNumbers(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &numbersScenario{tokensScenario: tokensScenario{settingsScenario: settingsScenario{
			t: t, env: map[string]string{}, workspaces: map[string]uuid.UUID{defaultWorkspace: sdk.DefaultTenantID},
		}}, replies: newReplyRecorder()}
		s.env["ALPHONE_WHATSAPP_GRAPH_URL"] = s.replies.server.URL
		s.bindSetup(sc)
		s.bindDeliveries(sc)
		s.bindReplies(sc)
		s.bindAnswers(sc)
		s.bindSenders(sc)
		s.bindLongChat(sc)
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			s.replies.server.Close()
			return ctx, s.stop(ctx)
		})
	}
}

// bindSetup binds the steps that prepare an install and its numbers.
func (s *numbersScenario) bindSetup(sc *godog.ScenarioContext) {
	sc.Given(`^the database is migrated$`, s.migrateTheDatabase)
	sc.Given(`^the workspace "([^"]*)" exists$`, s.createWorkspace)
	sc.Given(`^the server holds an encryption key$`, s.holdAnEncryptionKey)
	sc.Given(`^the environment file names the WhatsApp number "([^"]*)" with its token$`, s.nameTheNumber)
	sc.Given(`^the environment file names the WhatsApp number "([^"]*)" without a token$`, s.nameTheNumberAlone)
	sc.Given(`^the environment file names the app secret "([^"]*)"$`, s.nameTheAppSecret)
	sc.Given(`^an install from before many numbers holds a conversation with "([^"]*)"$`, s.holdOldConversation)
	sc.Given(`^that install stored the WhatsApp number "([^"]*)" for the default workspace$`, s.holdOldNumber)
	sc.Given(`^AlphOne runs its plugins$`, s.runAndKeepThePlugin)
	sc.Step(`^the workspace "([^"]*)" connects the WhatsApp number "([^"]*)" as "([^"]*)"$`, s.connect)
	sc.Step(`^the default workspace connects the WhatsApp number "([^"]*)" as "([^"]*)"$`, s.defaultConnects)
	sc.When(`^the workspace "([^"]*)" tries to connect the WhatsApp number "([^"]*)"$`, s.tryToConnect)
	sc.When(`^the workspace "([^"]*)" replaces the token of the WhatsApp number "([^"]*)"$`, s.replaceToken)
	sc.When(`^the workspace "([^"]*)" removes the WhatsApp number "([^"]*)"$`, s.remove)
	sc.When(`^the default workspace removes the WhatsApp number "([^"]*)"$`, s.defaultRemoves)
}

// bindDeliveries binds the steps that send Meta's deliveries and handshakes to the webhook.
func (s *numbersScenario) bindDeliveries(sc *godog.ScenarioContext) {
	sc.Step(`^the customer "([^"]*)" writes to the WhatsApp number "([^"]*)"$`, s.write)
	sc.Step(`^the customer "([^"]*)" writes to the environment number "([^"]*)"$`, s.writeToEnvironment)
	sc.When(`^a delivery for "([^"]*)" arrives signed with the app secret of "([^"]*)"$`, s.deliverSigned)
	sc.When(`^a delivery for "([^"]*)" arrives signed with the environment secret$`, s.deliverSignedByEnvironment)
	sc.When(`^one delivery for "([^"]*)" and "([^"]*)" arrives signed with the app secret of "([^"]*)"$`, s.deliverMixed)
	sc.When(`^one delivery for "([^"]*)", the environment number "([^"]*)" and no number `+
		`arrives signed with the app secret of "([^"]*)"$`, s.deliverAcrossEnvironment)
	sc.When(`^a status for the newest reply arrives for "([^"]*)" signed with the app secret of "([^"]*)"$`,
		s.deliverStatusOnNewestReply)
	sc.Then(`^the delivery is answered with (\d+)$`, s.deliveryAnswered)
	sc.Step(`^the handshake with the verify token of "([^"]*)" passes$`, s.handshakePasses)
	sc.Then(`^the handshake with the verify token "([^"]*)" fails$`, s.handshakeFails)
}

// bindReplies binds the steps that reply on a conversation.
func (s *numbersScenario) bindReplies(sc *godog.ScenarioContext) {
	sc.When(`^the workspace "([^"]*)" replies to "([^"]*)" on "([^"]*)"$`, s.reply)
	sc.When(`^the workspace "([^"]*)" tries to reply to "([^"]*)" on "([^"]*)"$`, s.tryToReply)
	sc.When(`^the default workspace replies to "([^"]*)"$`, s.replyOnNoNumber)
	sc.When(`^the default workspace tries to reply to "([^"]*)"$`, s.tryToReplyOnNoNumber)
	sc.When(`^the default workspace replies to "([^"]*)" on "([^"]*)"$`, s.defaultReplies)
	sc.Then(`^the reply leaves from "([^"]*)" with its token$`, s.replyLeavesWithToken)
	sc.Then(`^the reply leaves from "([^"]*)" with its replaced token$`, s.replyLeavesWithReplacedToken)
	sc.Then(`^the reply leaves from "([^"]*)" with the environment token$`, s.replyLeavesWithEnvironmentToken)
	sc.Then(`^no reply left for WhatsApp$`, s.noReplyLeft)
}

// bindAnswers binds the steps that read what the workspaces hold.
func (s *numbersScenario) bindAnswers(sc *godog.ScenarioContext) {
	sc.Then(`^the workspace "([^"]*)" lists the WhatsApp numbers "([^"]*)"$`, s.listsNumbers)
	sc.Then(`^the default workspace lists the WhatsApp numbers "([^"]*)"$`, s.defaultListsNumbers)
	sc.Then(`^the workspace "([^"]*)" lists no WhatsApp numbers$`, s.listsNoNumbers)
	sc.Then(`^the workspace "([^"]*)" holds (\d+) conversations?$`, s.holdsConversations)
	sc.Then(`^the default workspace holds (\d+) conversations?$`, s.defaultHoldsConversations)
	sc.Then(`^the workspace "([^"]*)" holds (\d+) contacts?$`, s.holdsContacts)
	sc.Then(`^the default workspace holds (\d+) contacts?$`, s.defaultHoldsContacts)
	sc.Then(`^the workspace "([^"]*)" still reads the conversation with "([^"]*)" on "([^"]*)"$`, s.readsConversation)
	sc.Then(`^the default workspace reads (\d+) conversations? on "([^"]*)"$`, s.defaultReadsOnNumber)
	sc.Then(`^the default workspace reads the conversation with "([^"]*)" as on "([^"]*)"$`, s.defaultReadsNumberOf)
	sc.Then(`^the attempt is turned down as "([^"]*)"$`, s.attemptTurnedDown)
	sc.Then(`^the attempt names no workspace$`, s.attemptNamesNoWorkspace)
	sc.Then(`^the newest reply holds no delivery status$`, s.newestReplyHoldsNoStatus)
	sc.Then(`^the newest reply is marked delivered$`, s.newestReplyDelivered)
}

// numberToken answers the access token a number is connected with.
func numberToken(number string) string {
	return "EAAG-" + number + "-token"
}

// numberAppSecret answers the app secret a number is connected with.
func numberAppSecret(number string) string {
	return "app-secret-" + number
}

// numberVerifyToken answers the verify token a number is connected with.
func numberVerifyToken(number string) string {
	return "verify-" + number
}

// nameTheAppSecret sets the app secret the environment file names.
func (s *numbersScenario) nameTheAppSecret(secret string) {
	s.env[environmentAppSecret] = secret
}

// nameTheNumberAlone sets the number the environment file names with no token beside it.
func (s *numbersScenario) nameTheNumberAlone(number string) {
	s.env["ALPHONE_WHATSAPP_PHONE_NUMBER_ID"] = number
}

// holdOldNumber stores number for the default workspace in an install from before many numbers.
func (s *numbersScenario) holdOldNumber(ctx context.Context, number string) error {
	_, err := s.pool.Exec(ctx, storeOldNumber, sdk.DefaultTenantID, number)
	return err
}

// holdOldConversation brings the plugin to the version before many numbers and stores a default conversation there.
func (s *numbersScenario) holdOldConversation(ctx context.Context, customer string) error {
	if _, err := s.pool.Exec(ctx, storeWhatsAppSchema); err != nil {
		return err
	}
	db := stdlib.OpenDBFromPool(s.pool)
	defer func() { _ = db.Close() }()
	versions, err := database.NewStore(database.DialectPostgres, whatsAppVersionTable)
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider("", db, os.DirFS(whatsAppMigrations), goose.WithStore(versions))
	if err != nil {
		return err
	}
	if _, err := provider.UpTo(ctx, lastVersionBeforeNumbers); err != nil {
		return err
	}
	contactID := uuid.Must(uuid.NewV7())
	if _, err := s.pool.Exec(ctx, storeContact, contactID, "Maria Perez", sdk.DefaultTenantID); err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, storeConversation, uuid.Must(uuid.NewV7()), contactID, customer, sdk.DefaultTenantID)
	return err
}

// runAndKeepThePlugin runs the plugins and keeps the compiled WhatsApp plugin.
func (s *numbersScenario) runAndKeepThePlugin(ctx context.Context) error {
	if err := s.runThePlugins(ctx); err != nil {
		return err
	}
	plugin, compiled := s.whatsApp.(*whatsapp.Plugin)
	if !compiled {
		return errors.New("the plugin that offers WhatsApp credentials is not the WhatsApp plugin")
	}
	s.plugin = plugin
	return nil
}

// connect connects number to the workspace called name under label, with every secret of its own.
func (s *numbersScenario) connect(ctx context.Context, name, number, label string) error {
	return s.whatsApp.SetChannelCredentials(s.in(ctx, name), sdk.ChannelCredentials{
		Identifier:  number,
		Label:       label,
		Secret:      numberToken(number),
		AppSecret:   numberAppSecret(number),
		VerifyToken: numberVerifyToken(number),
	})
}

// defaultConnects connects number to the default workspace under label, with every secret of its own.
func (s *numbersScenario) defaultConnects(ctx context.Context, number, label string) error {
	return s.connect(ctx, defaultWorkspace, number, label)
}

// tryToConnect tries to connect number to the workspace called name and keeps what it answered.
func (s *numbersScenario) tryToConnect(ctx context.Context, name, number string) {
	s.attempt = s.connect(ctx, name, number, "Elsewhere")
}

// replaceToken gives number a new token and nothing else.
func (s *numbersScenario) replaceToken(ctx context.Context, name, number string) error {
	return s.whatsApp.SetChannelCredentials(s.in(ctx, name), sdk.ChannelCredentials{
		Identifier: number, Secret: numberToken(number) + replacedTokenSuffix,
	})
}

// remove removes number from the workspace called name.
func (s *numbersScenario) remove(ctx context.Context, name, number string) error {
	return s.whatsApp.RemoveChannelCredentials(s.in(ctx, name), number)
}

// defaultRemoves removes number from the default workspace.
func (s *numbersScenario) defaultRemoves(ctx context.Context, number string) error {
	return s.remove(ctx, defaultWorkspace, number)
}

// nextMessageID answers a message id no delivery of the scenario used.
func (s *numbersScenario) nextMessageID() string {
	s.delivered++
	return fmt.Sprintf("wamid.delivered.%d", s.delivered)
}

// change answers one change of a delivery, a text message from customer arriving on number.
func (s *numbersScenario) change(number, customer string) string {
	return fmt.Sprintf(`{"field":"messages","value":{"metadata":{"phone_number_id":%q},`+
		`"contacts":[{"wa_id":%q,"profile":{"name":"Maria Perez"}}],`+
		`"messages":[{"from":%q,"id":%q,"timestamp":"1751791000","type":"text","text":{"body":"hello"}}]}}`,
		number, customer, customer, s.nextMessageID())
}

// post sends the changes to the webhook as one delivery signed with secret and keeps the answer.
func (s *numbersScenario) post(secret string, changes ...string) {
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"0","changes":[` +
		strings.Join(changes, ",") + `]}]}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	recorder := httptest.NewRecorder()
	s.plugin.Routes().ServeHTTP(recorder, request)
	s.answered = recorder.Code
}

// wantStored fails unless the last delivery was answered with success.
func (s *numbersScenario) wantStored() error {
	if s.answered != http.StatusOK {
		return fmt.Errorf("the delivery was answered with %d, want %d", s.answered, http.StatusOK)
	}
	return nil
}

// write delivers a message from customer to number, signed with the number's own app secret.
func (s *numbersScenario) write(customer, number string) error {
	s.post(numberAppSecret(number), s.change(number, customer))
	return s.wantStored()
}

// writeToEnvironment delivers a message from customer to the environment number, signed with the setting.
func (s *numbersScenario) writeToEnvironment(customer, number string) error {
	s.post(s.env[environmentAppSecret], s.change(number, customer))
	return s.wantStored()
}

// deliverSigned delivers a message for number signed with the app secret of signer.
func (s *numbersScenario) deliverSigned(number, signer string) {
	s.post(numberAppSecret(signer), s.change(number, "184467235"))
}

// deliverSignedByEnvironment delivers a message for number signed with the app secret in the environment file.
func (s *numbersScenario) deliverSignedByEnvironment(number string) {
	s.post(s.env[environmentAppSecret], s.change(number, "184467235"))
}

// deliverMixed delivers one message for each of two numbers in one body signed with the app secret of signer.
func (s *numbersScenario) deliverMixed(first, second, signer string) {
	s.post(numberAppSecret(signer), s.change(first, "184467235"), s.change(second, "184467236"))
}

// deliverAcrossEnvironment delivers one body for number, the environment number and no number, signed for signer.
func (s *numbersScenario) deliverAcrossEnvironment(number, environment, signer string) {
	s.post(numberAppSecret(signer),
		s.change(number, "184467235"), s.change(environment, "184467236"), s.change("", "184467237"))
}

// newestReplyID answers the message id the stand-in for the Graph API gave the newest reply.
func (s *numbersScenario) newestReplyID() string {
	_, sent := s.replies.last()
	return fmt.Sprintf(replyIDFormat, sent)
}

// deliverStatusOnNewestReply delivers a status naming the newest reply on number, signed with the secret of signer.
func (s *numbersScenario) deliverStatusOnNewestReply(number, signer string) {
	s.post(numberAppSecret(signer), fmt.Sprintf(`{"field":"messages","value":{"metadata":{"phone_number_id":%q},`+
		`"statuses":[{"id":%q,"status":"delivered","timestamp":"1751791000","recipient_id":"184467235"}]}}`,
		number, s.newestReplyID()))
}

// newestReplyHolds fails unless the newest reply holds want, an empty want meaning no delivery status.
func (s *numbersScenario) newestReplyHolds(ctx context.Context, want string) error {
	var status string
	if err := s.pool.QueryRow(ctx, statusOfMessage, s.newestReplyID()).Scan(&status); err != nil {
		return err
	}
	if status != want {
		return fmt.Errorf("the newest reply is %q, want %q", status, want)
	}
	return nil
}

// newestReplyHoldsNoStatus fails when the newest reply holds a delivery status.
func (s *numbersScenario) newestReplyHoldsNoStatus(ctx context.Context) error {
	return s.newestReplyHolds(ctx, "")
}

// newestReplyDelivered fails unless the newest reply is marked delivered.
func (s *numbersScenario) newestReplyDelivered(ctx context.Context) error {
	return s.newestReplyHolds(ctx, "delivered")
}

// deliveryAnswered fails unless the last delivery was answered with status.
func (s *numbersScenario) deliveryAnswered(status int) error {
	if s.answered != status {
		return fmt.Errorf("the delivery was answered with %d, want %d", s.answered, status)
	}
	return nil
}

// handshake answers the status of Meta's verification handshake with token.
func (s *numbersScenario) handshake(token string) int {
	query := url.Values{"hub.mode": {"subscribe"}, "hub.verify_token": {token}, "hub.challenge": {"42"}}
	request := httptest.NewRequest(http.MethodGet, "/webhook?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()
	s.plugin.Routes().ServeHTTP(recorder, request)
	return recorder.Code
}

// handshakePasses fails unless the handshake passes with the verify token of number.
func (s *numbersScenario) handshakePasses(number string) error {
	if got := s.handshake(numberVerifyToken(number)); got != http.StatusOK {
		return fmt.Errorf("the handshake answered %d, want %d", got, http.StatusOK)
	}
	return nil
}

// handshakeFails fails unless the handshake is turned down with token.
func (s *numbersScenario) handshakeFails(token string) error {
	if got := s.handshake(token); got != http.StatusForbidden {
		return fmt.Errorf("the handshake answered %d, want %d", got, http.StatusForbidden)
	}
	return nil
}

// conversationKey names the conversation a customer holds on a number in a workspace.
type conversationKey struct {
	workspace string
	customer  string
	number    string
}

// conversationID reads the conversation the key names.
func (s *numbersScenario) conversationID(ctx context.Context, key conversationKey) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, conversationOnNumber, s.workspaces[key.workspace], key.customer, key.number).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("reading the conversation with %s on %q: %w", key.customer, key.number, err)
	}
	return id, nil
}

// sendReply replies on the conversation the key names.
func (s *numbersScenario) sendReply(ctx context.Context, key conversationKey) error {
	id, err := s.conversationID(ctx, key)
	if err != nil {
		return err
	}
	_, err = s.plugin.MutationResolvers().WhatsAppSendMessage(s.in(ctx, key.workspace), id, "Ready at 5pm")
	return err
}

// reply replies to customer on number from the workspace called name.
func (s *numbersScenario) reply(ctx context.Context, name, customer, number string) error {
	return s.sendReply(ctx, conversationKey{workspace: name, customer: customer, number: number})
}

// tryToReply tries to reply to customer on number and keeps what it answered.
func (s *numbersScenario) tryToReply(ctx context.Context, name, customer, number string) {
	s.attempt = s.sendReply(ctx, conversationKey{workspace: name, customer: customer, number: number})
}

// replyOnNoNumber replies to customer from the default workspace on the conversation naming no number.
func (s *numbersScenario) replyOnNoNumber(ctx context.Context, customer string) error {
	return s.sendReply(ctx, conversationKey{workspace: defaultWorkspace, customer: customer})
}

// tryToReplyOnNoNumber tries to reply to customer on the conversation naming no number and keeps what it answered.
func (s *numbersScenario) tryToReplyOnNoNumber(ctx context.Context, customer string) {
	s.attempt = s.sendReply(ctx, conversationKey{workspace: defaultWorkspace, customer: customer})
}

// defaultReplies replies to customer from the default workspace on the conversation carrying number.
func (s *numbersScenario) defaultReplies(ctx context.Context, customer, number string) error {
	return s.sendReply(ctx, conversationKey{workspace: defaultWorkspace, customer: customer, number: number})
}

// wantReply fails unless the newest reply left from number with token.
func (s *numbersScenario) wantReply(number, token string) error {
	path, authorization := "/"+number+"/messages", "Bearer "+token
	if got, _ := s.replies.last(); got.path != path || got.authorization != authorization {
		return fmt.Errorf("the reply left on %s as %s, want %s as %s", got.path, got.authorization, path, authorization)
	}
	return nil
}

// replyLeavesWithToken fails unless the newest reply left from number with the number's own token.
func (s *numbersScenario) replyLeavesWithToken(number string) error {
	return s.wantReply(number, numberToken(number))
}

// replyLeavesWithReplacedToken fails unless the newest reply left from number with its replaced token.
func (s *numbersScenario) replyLeavesWithReplacedToken(number string) error {
	return s.wantReply(number, numberToken(number)+replacedTokenSuffix)
}

// replyLeavesWithEnvironmentToken fails unless the newest reply left from number with the environment token.
func (s *numbersScenario) replyLeavesWithEnvironmentToken(number string) error {
	return s.wantReply(number, tokenOf(defaultWorkspace))
}

// noReplyLeft fails when any reply reached the Graph API.
func (s *numbersScenario) noReplyLeft() error {
	if got, sent := s.replies.last(); sent != 0 {
		return fmt.Errorf("%d replies left, the newest as %+v, want none", sent, got)
	}
	return nil
}

// identities answers the numbers the workspace called name lists, each as its id and label.
func (s *numbersScenario) identities(ctx context.Context, name string) ([]string, error) {
	listed, err := s.whatsApp.ChannelIdentities(s.in(ctx, name))
	if err != nil {
		return nil, err
	}
	numbers := make([]string, len(listed))
	for i, identity := range listed {
		numbers[i] = strings.TrimSpace(identity.Identifier + " " + identity.Label)
	}
	return numbers, nil
}

// listsNumbers fails unless the workspace called name lists exactly the given numbers.
func (s *numbersScenario) listsNumbers(ctx context.Context, name, numbers string) error {
	listed, err := s.identities(ctx, name)
	if err != nil {
		return err
	}
	if want := strings.Split(numbers, ", "); !slices.Equal(listed, want) {
		return fmt.Errorf("the workspace %s lists %q, want %q", name, listed, want)
	}
	return nil
}

// defaultListsNumbers fails unless the default workspace lists exactly the given numbers.
func (s *numbersScenario) defaultListsNumbers(ctx context.Context, numbers string) error {
	return s.listsNumbers(ctx, defaultWorkspace, numbers)
}

// listsNoNumbers fails unless the workspace called name lists no number.
func (s *numbersScenario) listsNoNumbers(ctx context.Context, name string) error {
	listed, err := s.identities(ctx, name)
	if err != nil {
		return err
	}
	if len(listed) != 0 {
		return fmt.Errorf("the workspace %s lists %q, want none", name, listed)
	}
	return nil
}

// count fails unless query counts want rows for the workspace called name.
func (s *numbersScenario) count(ctx context.Context, query, name string, want int) error {
	var held int
	if err := s.pool.QueryRow(ctx, query, s.workspaces[name]).Scan(&held); err != nil {
		return err
	}
	if held != want {
		return fmt.Errorf("the workspace %s holds %d, want %d", name, held, want)
	}
	return nil
}

// holdsConversations fails unless the workspace called name holds want conversations.
func (s *numbersScenario) holdsConversations(ctx context.Context, name string, want int) error {
	return s.count(ctx, workspaceConversations, name, want)
}

// defaultHoldsConversations fails unless the default workspace holds want conversations.
func (s *numbersScenario) defaultHoldsConversations(ctx context.Context, want int) error {
	return s.count(ctx, workspaceConversations, defaultWorkspace, want)
}

// holdsContacts fails unless the workspace called name holds want contacts.
func (s *numbersScenario) holdsContacts(ctx context.Context, name string, want int) error {
	return s.count(ctx, workspaceContacts, name, want)
}

// defaultHoldsContacts fails unless the default workspace holds want contacts.
func (s *numbersScenario) defaultHoldsContacts(ctx context.Context, want int) error {
	return s.count(ctx, workspaceContacts, defaultWorkspace, want)
}

// readConversation reads through the graph the conversation the key names.
func (s *numbersScenario) readConversation(
	ctx context.Context, key conversationKey,
) (*model.WhatsAppConversation, error) {
	id, err := s.conversationID(ctx, key)
	if err != nil {
		return nil, err
	}
	return s.plugin.QueryResolvers().WhatsAppConversation(s.in(ctx, key.workspace), id)
}

// readsConversation fails unless the workspace called name still opens its conversation on number.
func (s *numbersScenario) readsConversation(ctx context.Context, name, customer, number string) error {
	read, err := s.readConversation(ctx, conversationKey{workspace: name, customer: customer, number: number})
	if err != nil {
		return err
	}
	if read == nil || read.ExternalID != customer {
		return fmt.Errorf("the workspace %s reads %+v, want the conversation with %s", name, read, customer)
	}
	return nil
}

// defaultReadsOnNumber fails unless filtering the default workspace on number reads want conversations.
func (s *numbersScenario) defaultReadsOnNumber(ctx context.Context, want int, number string) error {
	read, err := s.plugin.QueryResolvers().WhatsAppConversations(s.in(ctx, defaultWorkspace), nil, &number)
	if err != nil {
		return err
	}
	if len(read) != want {
		return fmt.Errorf("the default workspace reads %d conversations on %s, want %d", len(read), number, want)
	}
	return nil
}

// defaultReadsNumberOf fails unless the conversation storing no number reads as number to the default workspace.
func (s *numbersScenario) defaultReadsNumberOf(ctx context.Context, customer, number string) error {
	read, err := s.readConversation(ctx, conversationKey{workspace: defaultWorkspace, customer: customer})
	if err != nil {
		return err
	}
	if read.Number == nil || *read.Number != number {
		return fmt.Errorf("the default workspace reads the number %v, want %s", read.Number, number)
	}
	return nil
}

// attemptTurnedDown fails unless the last attempt answered the reason.
func (s *numbersScenario) attemptTurnedDown(reason string) error {
	var coded sdk.GraphError
	if !errors.As(s.attempt, &coded) || coded.Reason != reason {
		return fmt.Errorf("the attempt answered %v, want the reason %s", s.attempt, reason)
	}
	return nil
}

// attemptNamesNoWorkspace fails when the last attempt carries the name or the id of a workspace.
func (s *numbersScenario) attemptNamesNoWorkspace() error {
	said := fmt.Sprintf("%+v", s.attempt)
	for name, id := range s.workspaces {
		if strings.Contains(said, name) || strings.Contains(said, id.String()) {
			return fmt.Errorf("the attempt said %q, which names the workspace %s", said, name)
		}
	}
	return nil
}

func TestWhatsAppNumbers(t *testing.T) {
	runFeature(t, whatsAppNumbersFeature, initializeWhatsAppNumbers(t))
}
