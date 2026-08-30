// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"fmt"
	"log/slog"
	"net/url"

	"github.com/gopherium/framework/mailkit/smtp"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/mail"
)

// buildMailer returns the mailer the account flows deliver through, nil when no relay is named.
func buildMailer(settings mailSettings, logger *slog.Logger) (graphres.Mailer, error) {
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
	held, err := mail.New(sender, settings.templateDir)
	if err != nil {
		return nil, err
	}
	logger.Info(
		"mail relay configured",
		"host", settings.host,
		"port", settings.port,
		"templates", templateSource(settings.templateDir),
	)
	return held, nil
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
