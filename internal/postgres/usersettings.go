// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/postgres/db"
)

// UserSettingStore reads and writes the per-user settings.
type UserSettingStore struct {
	queries *db.Queries
}

// NewUserSettingStore returns a UserSettingStore backed by pool.
func NewUserSettingStore(pool *pgxpool.Pool) *UserSettingStore {
	return &UserSettingStore{queries: db.New(pool)}
}

// UserSetting returns the stored value under key, empty when the user set none.
func (s *UserSettingStore) UserSetting(ctx context.Context, userID uuid.UUID, key string) (string, error) {
	values, err := s.queries.UserSetting(ctx, db.UserSettingParams{UserID: userID, Key: key})
	if err != nil {
		return "", fmt.Errorf("read setting %s: %w", key, err)
	}
	if len(values) == 0 {
		return "", nil
	}
	return values[0], nil
}

// SetUserSetting stores value under key for the user, replacing any earlier value.
func (s *UserSettingStore) SetUserSetting(ctx context.Context, userID uuid.UUID, key, value string) error {
	err := s.queries.SetUserSetting(ctx, db.SetUserSettingParams{UserID: userID, Key: key, Value: value})
	if err != nil {
		return fmt.Errorf("store setting %s: %w", key, err)
	}
	return nil
}
