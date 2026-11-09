// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/sdk"
)

// defaultLimit is the limit of every workspace that holds none of its own in the limit tests.
const defaultLimit int64 = 5000

// limitSettings returns the host's settings store of the email plugin over a fresh database, beside its pool.
func limitSettings(t *testing.T) (sdk.SettingStore, *pgxpool.Pool) {
	t.Helper()
	pool, _ := newUnmigratedPool(t)
	return postgres.NewSettingStore(pool, nil).ForPlugin("email"), pool
}

// holdLimit stores value as the own mail limit of the workspace tenant.
func holdLimit(t *testing.T, store sdk.SettingStore, tenant uuid.UUID, value string) {
	t.Helper()
	if err := store.Save(sdk.WithTenant(t.Context(), tenant), map[string]string{workspaceLimitKey: value}); err != nil {
		t.Fatalf("storing the workspace limit %q: %v", value, err)
	}
}

// bufferLogger returns a logger that writes every line from info up to the buffer it returns beside it, as JSON.
func bufferLogger() (*slog.Logger, *bytes.Buffer) {
	written := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(written, nil)), written
}

// loggedLines returns each line a bufferLogger wrote, its time left out.
func loggedLines(t *testing.T, written *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for text := range strings.Lines(written.String()) {
		line := map[string]any{}
		if err := json.Unmarshal([]byte(text), &line); err != nil {
			t.Fatalf("reading the log line %q: %v", text, err)
		}
		delete(line, slog.TimeKey)
		lines = append(lines, line)
	}
	return lines
}

func TestReadWorkspaceLimitAnswersTheDefaultForAWorkspaceThatHoldsNone(t *testing.T) {
	t.Parallel()

	store, _ := limitSettings(t)
	logger, written := bufferLogger()

	limit, err := readWorkspaceLimit(t.Context(), store, logger, sdk.DefaultTenantID, defaultLimit)

	if err != nil || limit != defaultLimit {
		t.Errorf("readWorkspaceLimit() = %d, %v, want the default %d", limit, err, defaultLimit)
	}
	if written.Len() != 0 {
		t.Errorf("the log holds %q, want nothing", written)
	}
}

func TestReadWorkspaceLimitAnswersTheLimitAWorkspaceHolds(t *testing.T) {
	t.Parallel()

	store, _ := limitSettings(t)
	holdLimit(t, store, sdk.DefaultTenantID, "100")
	logger, written := bufferLogger()

	limit, err := readWorkspaceLimit(t.Context(), store, logger, sdk.DefaultTenantID, defaultLimit)

	if err != nil || limit != 100 {
		t.Errorf("readWorkspaceLimit() = %d, %v, want the workspace's own 100", limit, err)
	}
	if written.Len() != 0 {
		t.Errorf("the log holds %q, want nothing", written)
	}
}

func TestReadWorkspaceLimitTakesZeroAsNoLimit(t *testing.T) {
	t.Parallel()

	store, _ := limitSettings(t)
	holdLimit(t, store, sdk.DefaultTenantID, "0")

	limit, err := readWorkspaceLimit(t.Context(), store, slog.New(slog.DiscardHandler), sdk.DefaultTenantID, defaultLimit)

	if err != nil || limit != 0 {
		t.Errorf("readWorkspaceLimit() = %d, %v, want 0 for no limit", limit, err)
	}
}

func TestReadWorkspaceLimitReadsAnEmptyValueAsUnset(t *testing.T) {
	t.Parallel()

	store, _ := limitSettings(t)
	holdLimit(t, store, sdk.DefaultTenantID, "100")
	holdLimit(t, store, sdk.DefaultTenantID, "")
	logger, written := bufferLogger()

	limit, err := readWorkspaceLimit(t.Context(), store, logger, sdk.DefaultTenantID, defaultLimit)

	if err != nil || limit != defaultLimit {
		t.Errorf("readWorkspaceLimit() of a cleared limit = %d, %v, want the default %d", limit, err, defaultLimit)
	}
	if written.Len() != 0 {
		t.Errorf("the log holds %q, want nothing for a cleared limit", written)
	}
}

func TestReadWorkspaceLimitKeepsTheDefaultForAValueThatDoesNotRead(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"a lot", "-1", "1.5", "12 MB", " 100", "9223372036854775808"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			store, pool := limitSettings(t)
			tenant := storeWorkspace(t, pool)
			holdLimit(t, store, tenant, value)
			logger, written := bufferLogger()

			limit, err := readWorkspaceLimit(t.Context(), store, logger, tenant, defaultLimit)

			if err != nil || limit != defaultLimit {
				t.Errorf("readWorkspaceLimit() of %q = %d, %v, want the default %d", value, limit, err, defaultLimit)
			}
			lines := loggedLines(t, written)
			if len(lines) != 1 || lines[0][slog.LevelKey] != slog.LevelWarn.String() {
				t.Fatalf("the log holds %v, want one warning line", lines)
			}
			want := map[string]any{"workspace": tenant.String(), "key": workspaceLimitKey}
			delete(lines[0], slog.LevelKey)
			delete(lines[0], slog.MessageKey)
			if !reflect.DeepEqual(lines[0], want) {
				t.Errorf("the warning names %v, want only the workspace and the key %v", lines[0], want)
			}
		})
	}
}

func TestReadWorkspaceLimitReportsAReadThatFails(t *testing.T) {
	t.Parallel()

	store, _ := limitSettings(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	limit, err := readWorkspaceLimit(ctx, store, slog.New(slog.DiscardHandler), sdk.DefaultTenantID, defaultLimit)

	if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), "email: read the workspace mail limit: ") {
		t.Errorf("readWorkspaceLimit() = %d, %v, want the failed read named", limit, err)
	}
}

func TestReadWorkspaceLimitLeavesOutAnotherWorkspacesLimit(t *testing.T) {
	t.Parallel()

	store, pool := limitSettings(t)
	acme := storeWorkspace(t, pool)
	holdLimit(t, store, acme, "100")
	ctx := sdk.WithTenant(t.Context(), acme)

	limit, err := readWorkspaceLimit(ctx, store, slog.New(slog.DiscardHandler), sdk.DefaultTenantID, defaultLimit)

	if err != nil || limit != defaultLimit {
		t.Errorf("readWorkspaceLimit() of the default workspace = %d, %v, want the default %d, not Acme's 100",
			limit, err, defaultLimit)
	}
}
