// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/locale"
)

// localeKey names the setting the reader's language choice is stored under.
const localeKey = "locale.default"

// SettingStore reads and writes one per-user setting.
type SettingStore interface {
	UserSetting(ctx context.Context, userID uuid.UUID, key string) (string, error)
	SetUserSetting(ctx context.Context, userID uuid.UUID, key, value string) error
}

// Locale answers the locale the caller is served in.
func (q QueryResolvers) Locale(ctx context.Context) (string, error) {
	stored := ""
	if id := authkit.IdentityFromContext(ctx).ID; id != uuid.Nil && q.root.Settings != nil {
		held, err := q.root.Settings.UserSetting(ctx, id, localeKey)
		if err != nil {
			return "", err
		}
		stored = held
	}
	header := ""
	if carrier, err := httpFrom(ctx); err == nil {
		header = carrier.request.Header.Get("Accept-Language")
	}
	return locale.Resolve(stored, header), nil
}

// SupportedLocales answers every locale AlphOne serves, the default first.
func (q QueryResolvers) SupportedLocales(context.Context) ([]string, error) {
	return locale.Supported(), nil
}

// SetLocale stores the caller's language choice and answers it back.
func (m MutationResolvers) SetLocale(ctx context.Context, chosen string) (string, error) {
	if err := locale.Validate(chosen); err != nil {
		return "", err
	}
	id := authkit.IdentityFromContext(ctx).ID
	if err := m.root.Settings.SetUserSetting(ctx, id, localeKey, chosen); err != nil {
		return "", err
	}
	return chosen, nil
}
