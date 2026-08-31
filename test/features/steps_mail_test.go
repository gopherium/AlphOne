// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/quotedprintable"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// invitationAnswer is the answer an invitation carries.
type invitationAnswer struct {
	Data struct {
		Invite struct {
			Delivered bool `json:"delivered"`
		} `json:"invite"`
	} `json:"data"`
}

// confirmationListing is the answer a user listing carries.
type confirmationListing struct {
	Data struct {
		Users []struct {
			Email     string `json:"email"`
			Confirmed bool   `json:"confirmed"`
		} `json:"users"`
	} `json:"data"`
}

// graphErrors are the refusals one graph answer carries.
type graphErrors struct {
	Errors []struct {
		Extensions struct {
			Reason string `json:"reason"`
		} `json:"extensions"`
	} `json:"errors"`
}

// mailBodyOf decodes the quoted-printable body one delivered message carries.
func mailBodyOf(payload string) (string, error) {
	_, encoded, found := strings.Cut(payload, "\r\n\r\n")
	if !found {
		return "", fmt.Errorf("the mail carries no body: %q", payload)
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(encoded)))
	if err != nil {
		return "", fmt.Errorf("decoding the mail body: %w", err)
	}
	return string(decoded), nil
}

// waitForMail answers the payload of the one mail the relay received.
func (w *world) waitForMail() (string, error) {
	messages, err := w.relay.WaitForMessages(1, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("waiting for the mail: %w", err)
	}
	return messages[len(messages)-1].MsgRequest(), nil
}

// tokenFromMail answers the token the latest mailed link carries.
func (w *world) tokenFromMail(path string) (string, error) {
	payload, err := w.waitForMail()
	if err != nil {
		return "", err
	}
	body, err := mailBodyOf(payload)
	if err != nil {
		return "", err
	}
	_, after, found := strings.Cut(body, scenarioPublicURL+path+"?token=")
	if !found {
		return "", fmt.Errorf("the mail carries no %s link: %q", path, body)
	}
	token, _, _ := strings.Cut(after, "\n")
	return strings.TrimSpace(token), nil
}

// inviteThrough asks the admin's session to invite one address.
func (w *world) inviteThrough(ctx context.Context, email, name string) error {
	return w.postGraphAsSession(ctx, fmt.Sprintf(
		`{"query":"mutation { invite(email: \"%s\", name: \"%s\") { delivered } }"}`, email, name))
}

// activateThrough spends the latest activation link under the given password.
func (w *world) activateThrough(ctx context.Context, password string) error {
	token, err := w.tokenFromMail("/activate")
	if err != nil {
		return err
	}
	return w.postGraphKeeping(ctx, fmt.Sprintf(
		`{"query":"mutation { acceptInvite(token: \"%s\", password: \"%s\") { me { email } } }"}`,
		token, password), &w.invitedValue)
}

// postGraphKeeping posts an anonymous graph request, keeping any session cookie it answers.
func (w *world) postGraphKeeping(ctx context.Context, body string, keep *string) error {
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, w.server.URL+"/api/graphql", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("building the graph request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("posting the graph request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("reading the graph answer: %w", err)
	}
	for _, cookie := range response.Cookies() {
		if cookie.Name == "__Host-alphone_session" && keep != nil {
			*keep = cookie.Name + "=" + cookie.Value
		}
	}
	w.answered = answered
	w.status = response.StatusCode
	return nil
}

// registerMailSteps binds the mail steps and the world lifecycle.
func registerMailSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})

	sc.Given(`^a running AlphOne holding an admin and a mail relay$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.standsAsAdmin(ctx, w.ownerID)
	})

	sc.Step(`^the admin invite[sd] "([^"]*)" as "([^"]*)"$`,
		func(ctx context.Context, email, name string) error {
			return worldFrom(ctx).inviteThrough(ctx, email, name)
		})

	sc.Step(`^the invited person activate[sd] with the password "([^"]*)"$`,
		func(ctx context.Context, password string) error {
			return worldFrom(ctx).activateThrough(ctx, password)
		})

	sc.When(`^the invited person resets to the password "([^"]*)"$`,
		func(ctx context.Context, password string) error {
			w := worldFrom(ctx)
			if err := w.postGraphKeeping(ctx, `{"query":"mutation { requestPasswordReset(`+
				`email: \"grace@example.com\") }"}`, nil); err != nil {
				return err
			}
			token, err := w.tokenFromMail("/reset-password")
			if err != nil {
				return err
			}
			return w.postGraphKeeping(ctx, fmt.Sprintf(
				`{"query":"mutation { resetPassword(token: \"%s\", password: \"%s\") }"}`,
				token, password), nil)
		})

	sc.When(`^a reset is asked for "([^"]*)"$`, func(ctx context.Context, email string) error {
		return worldFrom(ctx).postGraphKeeping(ctx, fmt.Sprintf(
			`{"query":"mutation { requestPasswordReset(email: \"%s\") }"}`, email), nil)
	})

	registerMailOutcomes(sc)
}

// registerMailOutcomes binds the steps reading what a mail scenario produced.
func registerMailOutcomes(sc *godog.ScenarioContext) {
	sc.Then(`^the relay holds one mail addressed to "([^"]*)"$`,
		func(ctx context.Context, email string) error {
			payload, err := worldFrom(ctx).waitForMail()
			if err != nil {
				return err
			}
			if !strings.Contains(payload, "To: <"+email+">") {
				return fmt.Errorf("the mail is not addressed to %s: %q", email, payload)
			}
			return nil
		})

	sc.Then(`^the mail carries an activation link$`, func(ctx context.Context) error {
		token, err := worldFrom(ctx).tokenFromMail("/activate")
		if err != nil {
			return err
		}
		if token == "" {
			return fmt.Errorf("the activation link carries no token")
		}
		return nil
	})

	sc.Then(`^the invited person holds a session$`, func(ctx context.Context) error {
		if worldFrom(ctx).invitedValue == "" {
			return fmt.Errorf("the activation answered no session cookie")
		}
		return nil
	})

	sc.Then(`^the account shows as confirmed$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.postGraphAsSession(ctx, `{"query":"{ users { email confirmed } }"}`); err != nil {
			return err
		}
		var listing confirmationListing
		if err := json.Unmarshal(w.answered, &listing); err != nil {
			return fmt.Errorf("decoding the listing: %w", err)
		}
		for _, held := range listing.Data.Users {
			if held.Email == "grace@example.com" && held.Confirmed {
				return nil
			}
		}
		return fmt.Errorf("the activated account is not confirmed: %s", w.answered)
	})

	sc.Then(`^the invitation answers delivered$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		var answer invitationAnswer
		if err := json.Unmarshal(w.answered, &answer); err != nil {
			return fmt.Errorf("decoding the invitation: %w", err)
		}
		if !answer.Data.Invite.Delivered {
			return fmt.Errorf("the invitation was not delivered: %s", w.answered)
		}
		return nil
	})

	sc.Then(`^the activation is refused as an invalid link$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		var refused graphErrors
		if err := json.Unmarshal(w.answered, &refused); err != nil {
			return fmt.Errorf("decoding the refusal: %w", err)
		}
		if len(refused.Errors) == 0 || refused.Errors[0].Extensions.Reason != "token_invalid" {
			return fmt.Errorf("the activation was not refused as invalid: %s", w.answered)
		}
		return nil
	})

	sc.Then(`^the earlier session is gone$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.postGraphWithCookie(ctx, w.invitedValue, `{"query":"{ me { email } }"}`); err != nil {
			return err
		}
		if !strings.Contains(string(w.answered), "authentication required") {
			return fmt.Errorf("the earlier session still answers: %s", w.answered)
		}
		return nil
	})

	sc.Then(`^the invited person signs in with "([^"]*)"$`, func(ctx context.Context, password string) error {
		w := worldFrom(ctx)
		if err := w.postGraphKeeping(ctx, fmt.Sprintf(
			`{"query":"mutation { login(email: \"grace@example.com\", password: \"%s\") { me { email } } }"}`,
			password), nil); err != nil {
			return err
		}
		if !strings.Contains(string(w.answered), "grace@example.com") {
			return fmt.Errorf("the replaced password does not sign in: %s", w.answered)
		}
		return nil
	})

	sc.Then(`^the request is answered$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if !strings.Contains(string(w.answered), `"requestPasswordReset":true`) {
			return fmt.Errorf("the request was not answered neutrally: %s", w.answered)
		}
		return nil
	})

	sc.Then(`^the relay holds no mail$`, func(ctx context.Context) error {
		if held := worldFrom(ctx).relay.Messages(); len(held) != 0 {
			return fmt.Errorf("the relay holds %d mails, want none", len(held))
		}
		return nil
	})
}
