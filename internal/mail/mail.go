// SPDX-License-Identifier: Elastic-2.0

// Package mail sends the account mail the invite and reset flows deliver.
package mail

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"

	"github.com/gopherium/framework/mailkit"
)

//go:embed templates/*.tmpl
var embedded embed.FS

// defaults holds the templates the binary ships.
var defaults = mustSub(embedded, "templates")

// mustSub returns the dir subtree of fsys and panics if it cannot be created.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

// The template files the flows render.
const (
	inviteTemplate = "invite.tmpl"
	resetTemplate  = "reset.tmpl"
)

// The paths the mailed links lead back to.
const (
	activatePath = "/activate"
	resetPath    = "/reset-password"
)

// Mailer renders and delivers the account mail of one installation.
type Mailer struct {
	sender    mailkit.Sender
	templates *mailkit.Templates
}

// New returns a Mailer delivering through sender, preferring the
// templates in overrideDir over the embedded ones.
func New(sender mailkit.Sender, overrideDir string) (*Mailer, error) {
	if sender == nil {
		return nil, errors.New("mail: nil sender")
	}
	templates, err := mailkit.NewTemplates(defaults, overrideDir)
	if err != nil {
		return nil, err
	}
	return &Mailer{sender: sender, templates: templates}, nil
}

// SendInvite mails the activation link to an invited address.
func (m *Mailer) SendInvite(ctx context.Context, to, name, link string) error {
	return m.send(ctx, inviteTemplate, to, name, link)
}

// SendReset mails the password reset link to an account's address.
func (m *Mailer) SendReset(ctx context.Context, to, name, link string) error {
	return m.send(ctx, resetTemplate, to, name, link)
}

// send renders one template for the recipient and delivers it.
func (m *Mailer) send(ctx context.Context, template, to, name, link string) error {
	held, err := m.templates.Render(template, struct {
		Name string
		Link string
	}{Name: name, Link: link})
	if err != nil {
		return err
	}
	held.To = to
	if err := m.sender.Send(ctx, held); err != nil {
		return fmt.Errorf("mail: send %s: %w", template, err)
	}
	return nil
}

// ActivationLink returns the address an invited person activates at,
// relative to the site when base is empty.
func ActivationLink(base, token string) string {
	return linkTo(base, activatePath, token)
}

// ResetLink returns the address a password reset is completed at,
// relative to the site when base is empty.
func ResetLink(base, token string) string {
	return linkTo(base, resetPath, token)
}

// linkTo builds one token-carrying link under base.
func linkTo(base, path, token string) string {
	return base + path + "?" + url.Values{"token": {token}}.Encode()
}
