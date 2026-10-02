// SPDX-License-Identifier: AGPL-3.0-or-later

import type { ReactNode } from 'react'

import { useAdminSettings } from './adminSettings'
import { rememberFormatLocale } from './format'

/**
 * Renders the screens once the server named the locale dates and numbers are written in.
 * @param props - The screens, and what stands in while the settings load.
 * @returns The screens, or the stand in while the settings load.
 */
export function FormatLocaleGate({ children, loading }: { children: ReactNode; loading: ReactNode }) {
	const { settings, failed } = useAdminSettings()
	if (settings !== undefined) {
		rememberFormatLocale(settings.formatLocale)
		return children
	}
	return failed ? children : loading
}
