// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

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

// newInviteResolver returns a resolver serving the invite flows over store and mailer.
func newInviteResolver(store *testkit.Store, mailer graphres.Mailer) *graphres.Resolver {
	held := &graphres.Resolver{
		Version:      "9.9.9",
		Auth:         authkit.New(authkit.Config{Store: store, CookieName: "alphone_session"}),
		Admin:        authkit.NewAdmin(authkit.AdminConfig{Store: store}),
		Invites:      authkit.NewInvites(authkit.InvitesConfig{Store: store}),
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

func TestRequestPasswordResetReplacesAStandingToken(t *testing.T) {
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
		t.Error("both mails carry the same link, want the second request to replace the token")
	}

	stale := activationTokenOf(t, mailer.resets[0].link)
	if _, err := resolver.Invites.RedeemReset(t.Context(), stale, "brand new password"); err == nil {
		t.Error("RedeemReset(stale) error = nil, want the replaced link dead")
	}
	fresh := activationTokenOf(t, mailer.resets[1].link)
	if _, err := resolver.Invites.RedeemReset(t.Context(), fresh, "brand new password"); err != nil {
		t.Errorf("RedeemReset(fresh) error = %v, want the latest link usable", err)
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

// failingOnceMailer refuses its first reset, then records what follows.
type failingOnceMailer struct {
	recordingMailer
	refused bool
}

// SendReset refuses the first reset it is asked to deliver.
func (m *failingOnceMailer) SendReset(ctx context.Context, to, name, link string) error {
	if !m.refused {
		m.refused = true
		return errors.New("the relay refused the message")
	}
	return m.recordingMailer.SendReset(ctx, to, name, link)
}

func TestRequestPasswordResetRetriesAfterAFailedDelivery(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	mailer := &failingOnceMailer{}
	resolver := newInviteResolver(store, mailer)

	refused := requestResetThrough(t, resolver, "maria@example.com")
	if len(refused.Errors) == 0 {
		t.Fatal("the failed delivery answered no error, want the failure surfaced")
	}

	retried := requestResetThrough(t, resolver, "maria@example.com")

	if len(retried.Errors) != 0 {
		t.Fatalf("the retry errored: %s", retried.Errors)
	}
	if len(mailer.resets) != 1 {
		t.Fatalf("the retry mailed %d links, want one usable link", len(mailer.resets))
	}
	token := activationTokenOf(t, mailer.resets[0].link)
	if _, err := resolver.Invites.RedeemReset(t.Context(), token, "brand new password"); err != nil {
		t.Errorf("RedeemReset(retried) error = %v, want the mailed link usable", err)
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
