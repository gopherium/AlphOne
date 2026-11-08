// SPDX-License-Identifier: AGPL-3.0-or-later

import '@alphone/frontend-sdk/dataviews'
import { useAdminSettings, useListView } from '@alphone/frontend-sdk'
import { readyListDates } from '@alphone/frontend-sdk/dataviews'
import { offeredSize } from '@gopherium/godmin'
import { UsersPage, usersListDefaults } from '@gopherium/react-auth/wpds/users'
import { Link } from '@tanstack/react-router'
import { useMemo } from 'react'

import { accountErrorMessages } from './errorMessages'
import { roleLabels } from './roles'
import { UserTabs } from './UserTabs'

/**
 * Renders the account administration screen, the shared users list over the view the address holds.
 * @returns The users screen.
 */
export function UsersScreen() {
	readyListDates()
	const { settings, failed } = useAdminSettings()
	const sizes = settings?.listPageSizes
	const size = settings?.listPageSize
	const labels = useMemo(() => roleLabels(), [])
	const errorMessages = useMemo(() => accountErrorMessages(), [])
	const list = useListView(usersListDefaults({ perPage: size }))
	const view = { ...list.view, perPage: offeredSize(list.view.perPage, sizes, size) }
	return (
		<UsersPage
			list={{ ...list, view }}
			pageSizes={sizes}
			sizesFailed={failed}
			roleLabels={labels}
			errorMessages={errorMessages}
			addLink={<Link to="/users/new" />}
			tabs={<UserTabs current="users" />}
			headingLevel={2}
		/>
	)
}
