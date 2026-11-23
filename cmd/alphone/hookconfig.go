// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"time"

	"github.com/gopherium/framework/gonsole"
)

// defaultScreensBudget is how long the plugins together may take to say whether their screens show when unset.
const defaultScreensBudget = 50 * time.Millisecond

// defaultHookTimedFirings is how many calls of a hook after one ran past its budget time each plugin when unset.
const defaultHookTimedFirings = 20

// mostHookTimedFirings is the most calls of a hook after one ran past its budget that time each plugin.
const mostHookTimedFirings = 1000

// hookSettings bound the hooks through which the core asks the plugins.
type hookSettings struct {
	screensBudget time.Duration
	timedFirings  int
}

// loadHookSettings reads how long the plugins may take to say whether their screens show and how many calls time them.
func loadHookSettings(env gonsole.Env) (hookSettings, error) {
	budget, err := env.Duration("PLUGIN_SCREENS_BUDGET", defaultScreensBudget)
	if err != nil {
		return hookSettings{}, err
	}
	firings, err := env.Count("PLUGIN_HOOK_TIMED_FIRINGS", defaultHookTimedFirings, gonsole.AtMost(mostHookTimedFirings))
	if err != nil {
		return hookSettings{}, err
	}
	return hookSettings{screensBudget: budget, timedFirings: firings}, nil
}
