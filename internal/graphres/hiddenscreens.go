// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"log/slog"

	"github.com/gopherium/framework/pluginkit"

	"github.com/gopherium/alphone/sdk"
)

// ScreensHook names the hook through which the core asks the plugins whether their screens show.
const ScreensHook = "admin.screens"

// hookFailedLine is the message the server logs for each failure of a plugin inside a hook.
const hookFailedLine = "plugin hook failed"

// ScreenGated is a registered plugin that answers whether its screens show.
type ScreenGated interface {
	ID() string
	sdk.ScreenGate
}

// HiddenPlugins answers the ids of the plugins that hide their screens in the caller's workspace.
func (q QueryResolvers) HiddenPlugins(ctx context.Context) ([]string, error) {
	if q.root.HiddenScreens == nil {
		return []string{}, nil
	}
	return q.root.HiddenScreens(ctx), nil
}

// HideScreens lists the id of plugin after ids when its screens hide, an answer after ctx ended failing as ctx did.
func HideScreens(ctx context.Context, plugin ScreenGated, ids []string) ([]string, error) {
	shows, err := plugin.ShowsScreens(ctx)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil || shows {
		return ids, err
	}
	return append(ids, plugin.ID()), nil
}

// LogHookFailure returns the report that logs each failure of a plugin inside a hook once to logger.
func LogHookFailure(logger *slog.Logger) func(pluginkit.HookFailure) {
	return func(failure pluginkit.HookFailure) {
		logger.Error(hookFailedLine, "plugin", failure.ID, "hook", failure.Hook, "panicked", failure.Panicked,
			"context_ended", failure.ContextEnded, "error", failure.Err, "stack", failure.Stack)
	}
}
