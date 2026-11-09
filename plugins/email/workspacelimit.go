// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// workspaceLimitKey is the plugin setting that holds a workspace's own mail limit in bytes.
const workspaceLimitKey = "workspace_max_bytes"

// readWorkspaceLimit returns the mail limit of the workspace tenant, its own when one reads, fallback otherwise.
func readWorkspaceLimit(
	ctx context.Context, store sdk.SettingStore, logger *slog.Logger, tenant uuid.UUID, fallback int64,
) (int64, error) {
	ctx = sdk.WithTenant(ctx, tenant)
	value, set, err := store.Lookup(ctx, workspaceLimitKey)
	if err != nil {
		return 0, fmt.Errorf("email: read the workspace mail limit: %w", err)
	}
	if !set || value == "" {
		return fallback, nil
	}
	limit, err := strconv.ParseInt(value, 10, 64)
	if err != nil || limit < 0 {
		logger.WarnContext(ctx, "email: the workspace mail limit does not read as bytes, keeping the default",
			"workspace", tenant, "key", workspaceLimitKey)
		return fallback, nil
	}
	return limit, nil
}
