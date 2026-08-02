// SPDX-License-Identifier: AGPL-3.0-or-later

import { Stack, Text, ThemeProvider, useEventStream } from '@alphone/frontend-sdk'
import { AccountPanel } from '@gopherium/react-auth/wpds'
import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import type { StaticDataRouteOption } from '@tanstack/react-router'

import { MainMenu } from './menu/MainMenu'
import { useAppVersion } from './version'

const CHROME_COLOR = { background: '#1e1e1e' }
const CANVAS_COLOR = { background: '#ffffff' }
const CANVAS_CLASS = 'alphone-layout__canvas'

/**
 * Returns the canvas class for the given route matches.
 * @param matches - The router matches for the active route.
 * @returns The canvas class, carrying the bleed modifier when a match asks for it.
 */
function canvasClass(matches: { staticData: StaticDataRouteOption }[]): string {
	const bleed = matches.some((match) => match.staticData.canvas === 'bleed')
	return bleed ? `${CANVAS_CLASS} ${CANVAS_CLASS}--bleed` : CANVAS_CLASS
}

/**
 * Renders the admin layout: a dark navigation chrome holding the branding and
 * either the main menu or the active section's sidebar screen, wrapped around a
 * light canvas showing the active route.
 * @returns The layout element framing the current route.
 */
export function Layout() {
	useEventStream('/api/events', { invalidateKeys: [['tasks'], ['contacts']] })
	const matches = useRouterState({ select: (state) => state.matches })
	const sidebarMatch = [...matches]
		.reverse()
		.find((match) => match.staticData.Sidebar)
	const Sidebar = sidebarMatch?.staticData.Sidebar
	const version = useAppVersion().data
	return (
		<ThemeProvider color={CHROME_COLOR}>
			<div className="alphone-layout">
				<div className="alphone-layout__sidebar">
					<Stack direction="column" gap="lg">
						<Link to="/" className="alphone-layout__brand">
							<Text variant="heading-lg">AlphOne</Text>
						</Link>
						<nav aria-label="Navigation">
							{Sidebar ? <Sidebar /> : <MainMenu />}
						</nav>
					</Stack>
					<AccountPanel className="alphone-layout__account" />
					{version ? (
						<Text className="alphone-layout__version">v{version}</Text>
					) : null}
				</div>
				<ThemeProvider color={CANVAS_COLOR}>
					<main className={canvasClass(matches)}>
						<Outlet />
					</main>
				</ThemeProvider>
			</div>
		</ThemeProvider>
	)
}
