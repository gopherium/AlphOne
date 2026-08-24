// SPDX-License-Identifier: AGPL-3.0-or-later

import { configureErrorText } from '@alphone/frontend-sdk'
import type { FrontendPlugin } from '@alphone/frontend-sdk'

import { errorFallback, errorTemplates } from './errorTemplates'
import { plugins } from '../plugins'

/**
 * Returns the templates every plugin declares, skipping the ones declaring none.
 * @param registered - The plugins the build wired in.
 * @returns The declared templates, merged in registration order.
 */
export function declaredTemplates(registered: FrontendPlugin[]): Record<string, string> {
	return registered.reduce<Record<string, string>>(
		(merged, plugin) => ({ ...merged, ...plugin.errorTemplates?.() }),
		{},
	)
}

/**
 * Returns the message every reason stands for, the core's own beside each plugin's.
 * @returns The messages, keyed by reason.
 */
export function appErrorTemplates(): Record<string, string> {
	return { ...errorTemplates(), ...declaredTemplates(plugins) }
}

/**
 * Points the graph seam at the templates every refused answer is rendered from.
 */
export function configureAppErrorText(): void {
	configureErrorText({ templates: appErrorTemplates, fallback: errorFallback })
}
