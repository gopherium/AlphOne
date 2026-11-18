// SPDX-License-Identifier: AGPL-3.0-or-later

import { useAdminSettings } from '@alphone/frontend-sdk'
import { AdminRoot } from '@gopherium/godmin'
import type { ReactNode } from 'react'

/**
 * Hosts the admin under the design system root, a busy dialog waiting on its write as long as the admin settings say.
 * @param props - The admin to host.
 * @returns The root holding the admin, with no time limit on a write until the admin settings arrived.
 */
export function AdminHost({ children }: { children: ReactNode }) {
	const { settings } = useAdminSettings()
	return <AdminRoot writeTimeout={settings?.dialogWriteTimeoutMilliseconds ?? Infinity}>{children}</AdminRoot>
}
