// SPDX-License-Identifier: AGPL-3.0-or-later

import { useWriteTimeout } from '@alphone/frontend-sdk'
import { AdminRoot } from '@gopherium/godmin'
import type { ReactNode } from 'react'

/**
 * Hosts the admin, giving a busy dialog's write the time limit the admin settings serve once a screen loaded them.
 * @param props - The admin to host.
 * @returns The root holding the admin, with no time limit on a write while no screen loaded the admin settings.
 */
export function AdminHost({ children }: { children: ReactNode }) {
	return <AdminRoot writeTimeout={useWriteTimeout()}>{children}</AdminRoot>
}
