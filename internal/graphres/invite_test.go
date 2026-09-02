// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/ratelimit"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/sdk"
)

// sentMail records one delivery a fake mailer accepted.
type sentMail struct {
	to   string
	name string
	link string
}

// recordingMailer keeps the mail it is asked to send, failing when told to.
type recordingMailer struct {
	invites []sentMail
	resets  []sentMail
	err     error
}

// SendInvite records the invitation mail or answers the configured failure.
func (m *recordingMailer) SendInvite(_ context.Context, to, name, link string) error {
	if m.err != nil {
		return m.err
	}
	m.invites = append(m.invites, sentMail{to: to, name: name, link: link})
	return nil
}

// SendReset records the reset mail or answers the configured failure.
func (m *recordingMailer) SendReset(_ context.Context, to, name, link string) error {
	if m.err != nil {
		return m.err
	}
	m.resets = append(m.resets, sentMail{to: to, name: name, link: link})
	return nil
}

// placingInvites invites through the store and records the tenant each address was handed to.
type placingInvites struct {
	invites *authkit.Invites
	mu      sync.Mutex
	placed  map[string]uuid.UUID
}

// InviteInto invites the address through the store and records tenantID against it.
func (p *placingInvites) InviteInto(
	ctx context.Context, tenantID uuid.UUID, email, name, role string,
) (gouncer.Token, error) {
	token, err := p.invites.Invite(ctx, email, name, role)
	if err != nil {
		return token, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.placed[email] = tenantID
	return token, nil
}

// placedIn answers the tenant the address was handed to, the zero id when none was.
func (p *placingInvites) placedIn(email string) uuid.UUID {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.placed[email]
}

// newInviteResolver returns a resolver serving the invite flows over store and mailer.
func newInviteResolver(store *testkit.Store, mailer graphres.Mailer) *graphres.Resolver {
	invites := authkit.NewInvites(authkit.InvitesConfig{Store: store, ResetTokensLive: 3})
	held := &graphres.Resolver{
		Version:      "9.9.9",
		Auth:         authkit.New(authkit.Config{Store: store, CookieName: "alphone_session"}),
		Admin:        authkit.NewAdmin(authkit.AdminConfig{Store: store}),
		Invites:      invites,
		Onboarding:   &placingInvites{invites: invites, placed: map[string]uuid.UUID{}},
		Accounts:     store,
		PublicURL:    "https://crm.example.com",
		LoginLimiter: ratelimit.NewLimiter(ratelimit.Config{Limit: 2, Window: time.Minute}),
		TokenLimiter: ratelimit.NewLimiter(ratelimit.Config{Limit: 2, Window: time.Minute}),
		ResetLimiter: ratelimit.NewLimiter(ratelimit.Config{Limit: 2, Window: time.Minute}),
	}
	if mailer != nil {
		held.Mailer = mailer
	}
	return held
}

// invitePayload holds the answer shape of the invite and resend mutations.
type invitePayload struct {
	Delivered      bool    `json:"delivered"`
	ActivationLink *string `json:"activationLink"`
}

// inviteThrough runs the invite mutation for maria@example.com as the acting admin.
func inviteThrough(t *testing.T, resolver *graphres.Resolver) invitePayload {
	t.Helper()
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))
	var response struct {
		Invite invitePayload `json:"invite"`
	}
	client.MustPost(
		`mutation { invite(email: "maria@example.com", name: "Maria Perez") { delivered activationLink } }`,
		&response,
	)
	return response.Invite
}

func TestInviteMailsTheActivationLink(t *testing.T) {
	t.Parallel()

	mailer := &recordingMailer{}
	held := inviteThrough(t, newInviteResolver(testkit.NewStore(), mailer))

	if !held.Delivered || held.ActivationLink != nil {
		t.Errorf("invite = %+v, want delivered with no link shown", held)
	}
	if len(mailer.invites) != 1 {
		t.Fatalf("sent %d invitation mails, want 1", len(mailer.invites))
	}
	sent := mailer.invites[0]
	if sent.to != "maria@example.com" || sent.name != "Maria Perez" {
		t.Errorf("mailed %+v, want Maria Perez's address and name", sent)
	}
	if !strings.HasPrefix(sent.link, "https://crm.example.com/activate?token=") {
		t.Errorf("link = %q, want the activation path under the public url", sent.link)
	}
}

func TestInviteShowsTheLinkWithoutAMailer(t *testing.T) {
	t.Parallel()

	resolver := newInviteResolver(testkit.NewStore(), nil)
	resolver.PublicURL = ""

	held := inviteThrough(t, resolver)

	if held.Delivered {
		t.Error("invite delivered = true, want false without a mailer")
	}
	if held.ActivationLink == nil || !strings.HasPrefix(*held.ActivationLink, "/activate?token=") {
		t.Errorf("activationLink = %v, want the relative activation link", held.ActivationLink)
	}
}

func TestInviteAnswersATakenAddressNeutrally(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testkit.Store) *graphres.Resolver{
		"with a mailer":    func(s *testkit.Store) *graphres.Resolver { return newInviteResolver(s, &recordingMailer{}) },
		"without a mailer": func(s *testkit.Store) *graphres.Resolver { return newInviteResolver(s, nil) },
	}
	for testName, build := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)

			held := inviteThrough(t, build(store))

			if !held.Delivered || held.ActivationLink != nil {
				t.Errorf("invite of a taken address = %+v, want the neutral delivered answer", held)
			}
		})
	}
}

func TestInviteResendsForAPendingAddress(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)

	first := inviteThrough(t, resolver)
	second := inviteThrough(t, resolver)

	if !first.Delivered || !second.Delivered {
		t.Fatalf("invites = %+v then %+v, want both delivered", first, second)
	}
	if len(mailer.invites) != 2 {
		t.Fatalf("sent %d invitation mails, want a fresh one per invite", len(mailer.invites))
	}
	if mailer.invites[0].link == mailer.invites[1].link {
		t.Error("both mails carry the same link, want the second invite to replace the token")
	}
}

func TestInviteSurfacesAMailFailure(t *testing.T) {
	t.Parallel()

	client := newGraphClient(
		t,
		newInviteResolver(testkit.NewStore(), &recordingMailer{err: fmt.Errorf("the relay refused")}),
		uuid.Must(uuid.NewV7()),
	)

	response, err := client.RawPost(
		`mutation { invite(email: "maria@example.com", name: "Maria Perez") { delivered } }`,
	)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(response.Errors) == 0 {
		t.Fatal("invite with a failing mailer answered no error, want the failure surfaced")
	}
}

func TestResendInviteAnswersEveryAddressTheSameWay(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "ada@example.com", "Ada Lovelace", testPassword)
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	inviteThrough(t, resolver)
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	for _, address := range []string{"maria@example.com", "ada@example.com", "nobody@example.com"} {
		var response struct {
			ResendInvite invitePayload `json:"resendInvite"`
		}
		client.MustPost(
			fmt.Sprintf(`mutation { resendInvite(email: %q) { delivered activationLink } }`, address),
			&response,
		)
		if !response.ResendInvite.Delivered || response.ResendInvite.ActivationLink != nil {
			t.Errorf("resendInvite(%s) = %+v, want the neutral delivered answer", address, response.ResendInvite)
		}
	}

	if len(mailer.invites) != 2 {
		t.Fatalf("sent %d invitation mails, want the original and one resend", len(mailer.invites))
	}
	resent := mailer.invites[1]
	if resent.to != "maria@example.com" || resent.name != "Maria Perez" {
		t.Errorf("resent %+v, want the pending account's address and stored name", resent)
	}
}

// graphAnswer decodes the body a graph mutation answers over HTTP.
type graphAnswer struct {
	Data   json.RawMessage `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

// postAnonymouslyOverHTTP runs one operation with no identity, answering the body and cookies.
func postAnonymouslyOverHTTP(
	t *testing.T, resolver *graphres.Resolver, query string,
) (*graphAnswer, []*http.Cookie) {
	t.Helper()
	srv := newGraphHandler(t, resolver)
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	ctx := graphres.WithHTTP(request.Context(), recorder, request)
	ctx = graphres.WithClientIP(ctx, "192.0.2.10")
	ctx = sdk.WithRequestScope(ctx, sdk.NewRequestScope())
	srv.ServeHTTP(recorder, request.WithContext(ctx))
	answer := &graphAnswer{}
	if err := json.Unmarshal(recorder.Body.Bytes(), answer); err != nil {
		t.Fatalf("decode answer %q: %v", recorder.Body.String(), err)
	}
	return answer, recorder.Result().Cookies()
}

// activationTokenOf pulls the token out of a recorded activation link.
func activationTokenOf(t *testing.T, link string) string {
	t.Helper()
	_, token, found := strings.Cut(link, "token=")
	if !found {
		t.Fatalf("link %q carries no token", link)
	}
	return token
}

// acceptThrough runs acceptInvite anonymously, answering the raw response and cookies.
func acceptThrough(t *testing.T, resolver *graphres.Resolver, token string) (*graphAnswer, []*http.Cookie) {
	t.Helper()
	return postAnonymouslyOverHTTP(t, resolver, fmt.Sprintf(
		`mutation { acceptInvite(token: %q, password: "correct horse battery") { me { email role } } }`, token,
	))
}

func TestAcceptInviteActivatesAndSignsIn(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	inviteThrough(t, resolver)
	token := activationTokenOf(t, mailer.invites[0].link)

	response, cookies := acceptThrough(t, resolver, token)

	if len(response.Errors) != 0 {
		t.Fatalf("acceptInvite errors = %v, want none", response.Errors)
	}
	if !strings.Contains(string(response.Data), "maria@example.com") {
		t.Errorf("payload = %s, want the activated account's identity", response.Data)
	}
	if len(cookies) == 0 {
		t.Fatal("no session cookie set, want the activation to sign in")
	}
	identity, err := resolver.Auth.SessionIdentity(t.Context(), cookies[0].Value)
	if err != nil || identity.Email != "maria@example.com" {
		t.Errorf("SessionIdentity() = %+v, %v, want Maria Perez's live session", identity, err)
	}
}

func TestAcceptInviteRefusesADeadToken(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	inviteThrough(t, resolver)
	token := activationTokenOf(t, mailer.invites[0].link)
	if first, _ := acceptThrough(t, resolver, token); len(first.Errors) != 0 {
		t.Fatalf("first accept errors = %v, want none", first.Errors)
	}

	spent, _ := acceptThrough(t, resolver, token)

	if got := firstErrorReason(t, spent.Errors); got != "token_invalid" {
		t.Errorf("spent token reason = %q, want token_invalid", got)
	}
}

func TestAcceptInviteThrottlesGuessedTokens(t *testing.T) {
	t.Parallel()

	resolver := newInviteResolver(testkit.NewStore(), &recordingMailer{})

	var last *graphAnswer
	for range 3 {
		last, _ = acceptThrough(t, resolver, "guessed-token")
	}

	if got := firstErrorReason(t, last.Errors); got != "rate_limited" {
		t.Errorf("third guess reason = %q, want rate_limited", got)
	}
}

// requestResetThrough runs requestPasswordReset anonymously for the address.
func requestResetThrough(t *testing.T, resolver *graphres.Resolver, address string) *graphAnswer {
	t.Helper()
	response, _ := postAnonymouslyOverHTTP(t, resolver, fmt.Sprintf(
		`mutation { requestPasswordReset(email: %q) }`, address,
	))
	return response
}

func TestRequestPasswordResetAnswersEveryAddressTheSameWay(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	inviteThrough(t, resolver)

	for _, address := range []string{"maria@example.com", "nobody@example.com"} {
		response := requestResetThrough(t, resolver, address)
		if len(response.Errors) != 0 {
			t.Fatalf("requestPasswordReset(%s) errors = %v, want the neutral answer", address, response.Errors)
		}
	}

	if len(mailer.resets) != 1 {
		t.Fatalf("sent %d reset mails, want only the confirmed account's", len(mailer.resets))
	}
	sent := mailer.resets[0]
	if sent.to != "maria@example.com" || sent.name != "Maria Perez" {
		t.Errorf("reset mailed %+v, want Maria Perez's address and name", sent)
	}
	if !strings.HasPrefix(sent.link, "https://crm.example.com/reset-password?token=") {
		t.Errorf("link = %q, want the reset path under the public url", sent.link)
	}
}

func TestRequestPasswordResetStacksAFreshLink(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)

	first := requestResetThrough(t, resolver, "maria@example.com")
	second := requestResetThrough(t, resolver, "maria@example.com")

	if len(first.Errors) != 0 || len(second.Errors) != 0 {
		t.Fatalf("reset requests errored %v then %v, want both neutral", first.Errors, second.Errors)
	}
	if len(mailer.resets) != 2 {
		t.Fatalf("sent %d reset mails, want a fresh link per request", len(mailer.resets))
	}
	if mailer.resets[0].link == mailer.resets[1].link {
		t.Fatal("both mails carry the same link, want independent links")
	}

	standing := activationTokenOf(t, mailer.resets[0].link)
	if _, err := resolver.Invites.RedeemReset(t.Context(), standing, "brand new password"); err != nil {
		t.Errorf("RedeemReset(the earlier link) error = %v, want a later request to leave it standing", err)
	}
}

func TestSpendingOneResetLinkEndsTheFamily(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	resolver.ResetLimiter = ratelimit.NewLimiter(ratelimit.Config{Limit: 3, Window: time.Minute})

	for range 3 {
		requestResetThrough(t, resolver, "maria@example.com")
	}
	if len(mailer.resets) != 3 {
		t.Fatalf("sent %d reset mails, want three standing links", len(mailer.resets))
	}

	spent := activationTokenOf(t, mailer.resets[1].link)
	if _, err := resolver.Invites.RedeemReset(t.Context(), spent, "brand new password"); err != nil {
		t.Fatalf("RedeemReset(middle link) error = %v, want nil", err)
	}

	for _, index := range []int{0, 2} {
		sibling := activationTokenOf(t, mailer.resets[index].link)
		if _, err := resolver.Invites.RedeemReset(t.Context(), sibling, "another password"); err == nil {
			t.Errorf("RedeemReset(link %d) error = nil, want the family ended with the spent one", index)
		}
	}
}

func TestRequestPasswordResetHoldsInsideTheCooldown(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	resolver.ResetCooldown = ratelimit.NewLimiter(ratelimit.Config{Limit: 1, Window: time.Minute})

	first := requestResetThrough(t, resolver, "maria@example.com")
	second := requestResetThrough(t, resolver, "maria@example.com")

	if len(first.Errors) != 0 || len(second.Errors) != 0 {
		t.Fatalf("reset requests errored %v then %v, want both neutral", first.Errors, second.Errors)
	}
	if len(mailer.resets) != 1 {
		t.Errorf("sent %d reset mails, want the cooldown to hold the second", len(mailer.resets))
	}
}

func TestTheResetCooldownIsKeyedPerAddress(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	store.AddUser(t, "ada@example.com", "Ada Lovelace", testPassword)
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	resolver.ResetCooldown = ratelimit.NewLimiter(ratelimit.Config{Limit: 1, Window: time.Minute})

	requestResetThrough(t, resolver, "maria@example.com")
	requestResetThrough(t, resolver, "ada@example.com")

	if len(mailer.resets) != 2 {
		t.Errorf("sent %d reset mails, want one address not to spend another's cooldown", len(mailer.resets))
	}
}

func TestRequestPasswordResetIsThrottledPerClient(t *testing.T) {
	t.Parallel()

	resolver := newInviteResolver(testkit.NewStore(), &recordingMailer{})

	var last *graphAnswer
	for range 3 {
		last = requestResetThrough(t, resolver, "maria@example.com")
	}

	if got := firstErrorReason(t, last.Errors); got != "rate_limited" {
		t.Errorf("third request reason = %q, want rate_limited", got)
	}
}

func TestResetPasswordReplacesThePasswordAndKillsSessions(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	cookie, err := resolver.Auth.StartSession(t.Context(), maria.ID)
	if err != nil {
		t.Fatalf("StartSession() error = %v, want nil", err)
	}
	requestResetThrough(t, resolver, "maria@example.com")
	token := activationTokenOf(t, mailer.resets[0].link)

	response, _ := postAnonymouslyOverHTTP(t, resolver, fmt.Sprintf(
		`mutation { resetPassword(token: %q, password: "brand new password") }`, token,
	))

	if len(response.Errors) != 0 {
		t.Fatalf("resetPassword errors = %v, want none", response.Errors)
	}
	if _, err := resolver.Auth.Authenticate(t.Context(), "maria@example.com", "brand new password"); err != nil {
		t.Errorf("Authenticate(new password) error = %v, want the replaced password accepted", err)
	}
	if _, err := resolver.Auth.SessionIdentity(t.Context(), cookie.Value); err == nil {
		t.Error("the old session survived the reset, want every session ended")
	}
}

func TestResetPasswordRefusesADeadToken(t *testing.T) {
	t.Parallel()

	response, _ := postAnonymouslyOverHTTP(t, newInviteResolver(testkit.NewStore(), &recordingMailer{}),
		`mutation { resetPassword(token: "guessed-token", password: "brand new password") }`,
	)

	if got := firstErrorReason(t, response.Errors); got != "token_invalid" {
		t.Errorf("dead token reason = %q, want token_invalid", got)
	}
}

func TestAnonymousInvitingStaysRefused(t *testing.T) {
	t.Parallel()

	response, _ := postAnonymouslyOverHTTP(t, newInviteResolver(testkit.NewStore(), &recordingMailer{}),
		`mutation { invite(email: "maria@example.com", name: "Maria Perez") { delivered } }`,
	)

	if got := firstErrorReason(t, response.Errors); got != "authentication_required" {
		t.Errorf("anonymous invite reason = %q, want authentication_required", got)
	}
}

func TestUsersReportTheirConfirmation(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	ada := store.AddUser(t, "ada@example.com", "Ada Lovelace", testPassword)
	resolver := newInviteResolver(store, &recordingMailer{})
	inviteThrough(t, resolver)
	client := newGraphClient(t, resolver, ada.ID)

	var response struct {
		Users []struct {
			Email     string `json:"email"`
			Confirmed bool   `json:"confirmed"`
		} `json:"users"`
	}
	client.MustPost(`query { users { email confirmed } }`, &response)

	confirmed := map[string]bool{}
	for _, user := range response.Users {
		confirmed[user.Email] = user.Confirmed
	}
	held, present := confirmed["maria@example.com"]
	if !present || held {
		t.Errorf("the invited account answers confirmed %v present %v, want listed unconfirmed", held, present)
	}
	if !confirmed["ada@example.com"] {
		t.Error("the password-created account answers unconfirmed, want confirmed")
	}
}

func TestRequestPasswordResetAnswersAFailedDeliveryNeutrally(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	resolver := newInviteResolver(store, &recordingMailer{err: errors.New("the relay refused the message")})

	held := requestResetThrough(t, resolver, "maria@example.com")
	unknown := requestResetThrough(t, resolver, "nobody@example.com")

	if len(held.Errors) != 0 {
		t.Errorf("a failed delivery answered %s, want the same neutral answer an unknown address gets", held.Errors)
	}
	if len(unknown.Errors) != 0 {
		t.Fatalf("an unknown address errored: %s", unknown.Errors)
	}
}

func TestRequestPasswordResetAnswersAStoreFailureNeutrally(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	store.LookupErr = errors.New("the database is unreachable")
	resolver := newInviteResolver(store, &recordingMailer{})

	held := requestResetThrough(t, resolver, "maria@example.com")

	if len(held.Errors) != 0 {
		t.Errorf("a store failure answered %s, want the neutral answer", held.Errors)
	}
}

func TestRequestPasswordResetSpendsItsOwnBudget(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	resolver.ResetLimiter = ratelimit.NewLimiter(ratelimit.Config{Limit: 1, Window: time.Minute})

	first := requestResetThrough(t, resolver, "maria@example.com")
	second := requestResetThrough(t, resolver, "maria@example.com")

	if len(first.Errors) != 0 {
		t.Fatalf("first request errored: %s", first.Errors)
	}
	if got := firstErrorReason(t, second.Errors); got != "rate_limited" {
		t.Errorf("second request reason = %q, want rate_limited by the reset budget", got)
	}
	if len(mailer.resets) != 1 {
		t.Errorf("sent %d reset mails, want the refused request to mail nothing", len(mailer.resets))
	}

	token := activationTokenOf(t, mailer.resets[0].link)
	response, _ := postAnonymouslyOverHTTP(t, resolver, fmt.Sprintf(
		`mutation { resetPassword(token: %q, password: "brand new password") }`, token,
	))
	if len(response.Errors) != 0 {
		t.Errorf("resetPassword under a spent reset budget errored: %s", response.Errors)
	}
}

// inviteMutation invites Maria Perez, asking only for the neutral flag.
const inviteMutation = `mutation { invite(email: "maria@example.com", name: "Maria Perez") { delivered } }`

// resendMutation resends Maria Perez's invitation, asking only for the neutral flag.
const resendMutation = `mutation { resendInvite(email: "maria@example.com") { delivered } }`

func TestInviteSurfacesTheStoreFailuresBehindIt(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		arrange func(*testing.T, *testkit.Store, *graphres.Resolver)
		query   string
	}{
		"creating the account fails": {
			arrange: func(_ *testing.T, store *testkit.Store, _ *graphres.Resolver) {
				store.CreateUserErr = errors.New("the account store is unreachable")
			},
			query: inviteMutation,
		},
		"reading a taken address fails": {
			arrange: func(t *testing.T, store *testkit.Store, _ *graphres.Resolver) {
				store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
				store.LookupErr = errors.New("the account store is unreachable")
			},
			query: inviteMutation,
		},
		"reading the tenants fails": {
			arrange: func(t *testing.T, store *testkit.Store, resolver *graphres.Resolver) {
				store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
				resolver.Tenants = failingTenantStore{}
			},
			query: resendMutation,
		},
		"replacing the pending token fails": {
			arrange: func(t *testing.T, store *testkit.Store, resolver *graphres.Resolver) {
				inviteThrough(t, resolver)
				store.TokenErr = errors.New("the token store is unreachable")
			},
			query: resendMutation,
		},
	}
	for testName, held := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			resolver := newInviteResolver(store, &recordingMailer{})
			held.arrange(t, store, resolver)
			client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

			response, err := client.RawPost(held.query)
			if err != nil {
				t.Fatalf("RawPost() error = %v, want nil", err)
			}

			if len(response.Errors) == 0 {
				t.Error("errors = none, want the store failure surfaced")
			}
		})
	}
}

func TestAcceptInviteSurfacesTheFailuresBehindIt(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testkit.Store, *graphres.Resolver){
		"the tenant is deactivated": func(_ *testkit.Store, resolver *graphres.Resolver) {
			resolver.Tenants = deactivatedTenantStore{}
		},
		"starting the session fails": func(store *testkit.Store, _ *graphres.Resolver) {
			store.CreateSessionErr = errors.New("the session store is unreachable")
		},
		"reading the new session fails": func(store *testkit.Store, _ *graphres.Resolver) {
			store.SessionErr = errors.New("the session store is unreachable")
		},
	}
	for testName, breakIt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			mailer := &recordingMailer{}
			resolver := newInviteResolver(store, mailer)
			inviteThrough(t, resolver)
			token := activationTokenOf(t, mailer.invites[0].link)
			breakIt(store, resolver)

			response, _ := acceptThrough(t, resolver, token)

			if len(response.Errors) == 0 {
				t.Error("errors = none, want the failure surfaced")
			}
		})
	}
}

func TestAcceptInviteFailsWithoutTheHTTPTransport(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	inviteThrough(t, resolver)
	token := activationTokenOf(t, mailer.invites[0].link)
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	response, err := client.RawPost(fmt.Sprintf(
		`mutation { acceptInvite(token: %q, password: "correct horse battery") { me { email } } }`, token,
	))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorCode(t, response.Errors); got != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL with no response to set the cookie on", got)
	}
}

func TestRequestPasswordResetSurfacesLimiterFailures(t *testing.T) {
	t.Parallel()

	tests := map[string]graphres.AttemptLimiter{
		"the budget check fails":      stubLimiter{checkErr: errors.New("the limiter is unreachable")},
		"recording the request fails": stubLimiter{recordErr: errors.New("the limiter is unreachable")},
	}
	for testName, limiter := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
			mailer := &recordingMailer{}
			resolver := newInviteResolver(store, mailer)
			resolver.ResetLimiter = limiter

			held := requestResetThrough(t, resolver, "maria@example.com")

			if len(held.Errors) == 0 {
				t.Error("errors = none, want the limiter failure surfaced")
			}
			if len(mailer.resets) != 0 {
				t.Errorf("sent %d reset mails, want none behind a refused request", len(mailer.resets))
			}
		})
	}
}

// resetAnswer is the answer shape of the requestPasswordReset mutation.
type resetAnswer struct {
	RequestPasswordReset bool `json:"requestPasswordReset"`
}

func TestRequestPasswordResetAnswersNeutrallyWithoutAMailer(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	resolver := newInviteResolver(store, nil)

	held := requestResetThrough(t, resolver, "maria@example.com")

	if len(held.Errors) != 0 {
		t.Fatalf("a request without a mailer answered %s, want the neutral answer", held.Errors)
	}
	var answered resetAnswer
	if err := json.Unmarshal(held.Data, &answered); err != nil {
		t.Fatalf("decode answer %s: %v", held.Data, err)
	}
	if !answered.RequestPasswordReset {
		t.Error("requestPasswordReset = false, want the same true answer a mailed request gets")
	}
	if len(store.Tokens) != 0 {
		t.Errorf("the store holds %d tokens, want no reset token minted with nothing to mail it with", len(store.Tokens))
	}
}

func TestRequestPasswordResetLogsTheDeliveryItHides(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	recorded := &bytes.Buffer{}
	resolver := newInviteResolver(store, &recordingMailer{err: errors.New("the relay refused the message")})
	resolver.Logger = slog.New(slog.NewTextHandler(recorded, nil))

	held := requestResetThrough(t, resolver, "maria@example.com")

	if len(held.Errors) != 0 {
		t.Errorf("a failed delivery answered %s, want the neutral answer", held.Errors)
	}
	if !strings.Contains(recorded.String(), "sending the reset link") {
		t.Errorf("the log holds %q, want the hidden delivery failure recorded", recorded.String())
	}
}

func TestResetPasswordSurfacesTheTokenFailuresBehindIt(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testkit.Store, *graphres.Resolver){
		"the token budget check fails": func(_ *testkit.Store, resolver *graphres.Resolver) {
			resolver.TokenLimiter = stubLimiter{checkErr: errors.New("the limiter is unreachable")}
		},
		"recording the refused token fails": func(_ *testkit.Store, resolver *graphres.Resolver) {
			resolver.TokenLimiter = stubLimiter{recordErr: errors.New("the limiter is unreachable")}
		},
		"spending the token fails": func(store *testkit.Store, _ *graphres.Resolver) {
			store.ResetErr = errors.New("the token store is unreachable")
		},
	}
	for testName, breakIt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			resolver := newInviteResolver(store, &recordingMailer{})
			breakIt(store, resolver)

			response, _ := postAnonymouslyOverHTTP(t, resolver,
				`mutation { resetPassword(token: "guessed-token", password: "brand new password") }`,
			)

			if len(response.Errors) == 0 {
				t.Error("errors = none, want the failure surfaced")
			}
		})
	}
}

func TestRequestPasswordResetSurvivesACooldownFailure(t *testing.T) {
	t.Parallel()

	tests := map[string]stubLimiter{
		"the cooldown check fails": {checkErr: errors.New("cooldown down")},
		"recording the mail fails": {recordErr: errors.New("cooldown down")},
	}
	for testName, limiter := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
			mailer := &recordingMailer{}
			resolver := newInviteResolver(store, mailer)
			resolver.ResetCooldown = limiter

			held := requestResetThrough(t, resolver, "maria@example.com")

			if len(held.Errors) != 0 {
				t.Errorf("a cooldown failure answered %s, want the neutral answer", held.Errors)
			}
			if len(mailer.resets) != 0 {
				t.Errorf("sent %d reset mails, want none when the cooldown cannot be spent", len(mailer.resets))
			}
		})
	}
}

func TestRequestPasswordResetHoldsWhenTheStackIsFull(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	resolver.ResetLimiter = ratelimit.NewLimiter(ratelimit.Config{Limit: 5, Window: time.Minute})

	for range 3 {
		requestResetThrough(t, resolver, "maria@example.com")
	}
	if len(mailer.resets) != 3 {
		t.Fatalf("sent %d reset mails, want the stack filled", len(mailer.resets))
	}

	beyond := requestResetThrough(t, resolver, "maria@example.com")

	if len(beyond.Errors) != 0 {
		t.Errorf("a request beyond the stack answered %s, want the neutral answer", beyond.Errors)
	}
	if len(mailer.resets) != 3 {
		t.Errorf("sent %d reset mails, want no new link beyond the stack", len(mailer.resets))
	}
}

// gatedLimiter counts spends and holds every check until it is released.
type gatedLimiter struct {
	mu      sync.Mutex
	spent   int
	limit   int
	release chan struct{}
}

// Check reports whether the budget was free when it was consulted.
func (g *gatedLimiter) Check(string) (bool, time.Duration, error) {
	g.mu.Lock()
	spent := g.spent
	g.mu.Unlock()
	<-g.release
	return spent < g.limit, 0, nil
}

// RecordFailure counts one spend.
func (g *gatedLimiter) RecordFailure(string) error {
	g.mu.Lock()
	g.spent++
	g.mu.Unlock()
	return nil
}

func TestTheResetCooldownIsSpentAtomically(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	mailer := &countingMailer{}
	resolver := newInviteResolver(store, mailer)
	resolver.ResetLimiter = ratelimit.NewLimiter(ratelimit.Config{Limit: 100, Window: time.Minute})
	gate := &gatedLimiter{limit: 1, release: make(chan struct{})}
	resolver.ResetCooldown = gate

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			requestResetThrough(t, resolver, "maria@example.com")
		}()
	}
	time.Sleep(100 * time.Millisecond)
	close(gate.release)
	wg.Wait()

	if got := mailer.sent.Load(); got != 1 {
		t.Errorf("two simultaneous requests mailed %d links, want the cooldown to admit exactly one", got)
	}
}

// countingMailer counts the reset mail it accepts.
type countingMailer struct {
	recordingMailer
	sent atomic.Int64
}

// SendReset counts one delivered reset link.
func (c *countingMailer) SendReset(context.Context, string, string, string) error {
	c.sent.Add(1)
	return nil
}
