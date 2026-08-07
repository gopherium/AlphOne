// SPDX-License-Identifier: AGPL-3.0-or-later

import { Stack, Text } from '@alphone/frontend-sdk'
import { AccountPanel } from '@gopherium/react-auth/wpds'
import { Link, useRouterState } from '@tanstack/react-router'

import { MainMenu } from './menu/MainMenu'
import { useAppVersion } from './version'

/**
 * Renders the navigation chrome: the brand, the active menu or section
 * sidebar, the account panel, and the running version.
 * @returns The rail contents.
 */
export function RailContent() {
	const matches = useRouterState({ select: (state) => state.matches })
	const sidebarMatch = [...matches]
		.reverse()
		.find((match) => match.staticData.Sidebar)
	const Sidebar = sidebarMatch?.staticData.Sidebar
	const version = useAppVersion()
	return (
		<>
			<Stack direction="column" gap="lg">
				<Link to="/" className="alphone-rail__brand">
					<Text variant="heading-lg">AlphOne</Text>
				</Link>
				<nav aria-label="Navigation">
					{Sidebar ? <Sidebar /> : <MainMenu />}
				</nav>
			</Stack>
			<AccountPanel className="alphone-rail__account" />
			{version ? (
				<Text className="alphone-rail__version">v{version}</Text>
			) : null}
		</>
	)
}
