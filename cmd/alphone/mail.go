// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/gopherium/framework/mailkit"
	"github.com/gopherium/framework/mailkit/smtp"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/mail"
	"github.com/gopherium/alphone/sdk"
)

// buildMail returns the relay sender and the mailer the account flows deliver through.
func buildMail(settings mailSettings, logger *slog.Logger) (mailkit.Sender, graphres.Mailer, error) {
	sender, err := buildMailSender(settings, logger)
	if err != nil {
		return nil, nil, err
	}
	mailer, err := buildMailer(sender, settings.templateDir)
	if err != nil {
		return nil, nil, err
	}
	return sender, mailer, nil
}

// buildMailSender returns the relay sender, nil when no relay is named.
func buildMailSender(settings mailSettings, logger *slog.Logger) (mailkit.Sender, error) {
	if settings.host == "" {
		logger.Info("no mail relay configured, invitations answer their activation link")
		return nil, nil
	}
	sender, err := smtp.New(smtp.Config{
		Host:     settings.host,
		Port:     settings.port,
		Username: settings.username,
		Password: settings.password,
		From:     settings.from,
		TLS:      smtp.TLS(settings.tls),
		HELO:     heloFor(settings.publicURL),
	})
	if err != nil {
		return nil, fmt.Errorf("build mail sender: %w", err)
	}
	logger.Info(
		"mail relay configured",
		"host", settings.host,
		"port", settings.port,
		"templates", templateSource(settings.templateDir),
	)
	return sender, nil
}

// buildMailer returns the mailer the account flows deliver through, nil when no relay is named.
func buildMailer(sender mailkit.Sender, templateDir string) (graphres.Mailer, error) {
	if sender == nil {
		return nil, nil
	}
	return mail.New(sender, templateDir)
}

// mailSenderBridge adapts the host's mailkit sender to the plugin contract.
type mailSenderBridge struct {
	sender mailkit.Sender
}

// Send delivers a plugin's mail through the host's relay.
func (b mailSenderBridge) Send(ctx context.Context, m sdk.Mail) error {
	return b.sender.Send(ctx, mailkit.Message{To: m.To, Subject: m.Subject, Body: m.Body})
}

// wireMailSenderFrom hands a bridge over sender to every consumer, nothing without a relay.
func wireMailSenderFrom(registered []sdk.Plugin, sender mailkit.Sender) {
	if sender == nil {
		return
	}
	wireMailSender(registered, mailSenderBridge{sender: sender})
}

// wireMailSender hands the mail sender to every registered consumer.
func wireMailSender(registered []sdk.Plugin, sender sdk.MailSender) {
	for _, plugin := range registered {
		if consumer, ok := plugin.(sdk.MailSenderConsumer); ok {
			consumer.UseMailSender(sender)
		}
	}
}

// heloFor returns the domain the sender greets a relay with.
func heloFor(publicURL string) string {
	held, err := url.Parse(publicURL)
	if err != nil {
		return ""
	}
	return held.Hostname()
}

// templateSource names where the mail templates are read from.
func templateSource(overrideDir string) string {
	if overrideDir == "" {
		return "built in"
	}
	return overrideDir
}
