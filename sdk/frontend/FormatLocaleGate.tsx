// SPDX-License-Identifier: AGPL-3.0-or-later

import { FormatLocaleGate as LocaleGate } from '@gopherium/gottext/react'
import type { ReactNode } from 'react'

import { useAdminSettings } from './adminSettings'

/**
 * Renders the screens once the server named the locale dates and numbers are written in.
 * @param props - The screens, and what stands in while the settings load.
 * @returns The screens, or the stand in while the settings load.
 */
export function FormatLocaleGate({ children, loading }: { children: ReactNode; loading: ReactNode }) {
	const { settings, failed } = useAdminSettings()
	return (
		<LocaleGate locale={settings?.formatLocale} failed={failed} loading={loading}>
			{children}
		</LocaleGate>
	)
}
