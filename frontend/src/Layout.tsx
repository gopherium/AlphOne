// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, Stack, Text, ThemeProvider } from '@alphone/frontend-sdk'
import { Link, Outlet, useRouterState } from '@tanstack/react-router'

import { useLogout, useSession } from './auth/session'
import { MainMenu } from './menu/MainMenu'
import { useAppVersion } from './version'

const CHROME_COLOR = { background: '#1e1e1e' }
const CANVAS_COLOR = { background: '#ffffff' }

/**
 * Renders the admin layout: a dark navigation chrome holding the branding and
 * either the main menu or the active section's sidebar screen, wrapped around a
 * light canvas showing the active route.
 * @returns The layout element framing the current route.
 */
export function Layout() {
	const matches = useRouterState({ select: (state) => state.matches })
	const sidebarMatch = [...matches]
		.reverse()
		.find((match) => match.staticData.Sidebar)
	const Sidebar = sidebarMatch?.staticData.Sidebar
	const user = useSession().data
	const signOut = useLogout()
	const version = useAppVersion().data
	return (
		<ThemeProvider color={CHROME_COLOR}>
			<div className="alphone-layout">
				<div className="alphone-layout__sidebar">
					<Stack direction="column" gap="lg">
						<Link to="/" className="alphone-layout__brand">
							<Text variant="heading-lg" render={<h1 />}>
								AlphOne
							</Text>
						</Link>
						<nav aria-label="Navigation">
							{Sidebar ? <Sidebar /> : <MainMenu />}
						</nav>
					</Stack>
					{user ? (
						<Stack
							direction="column"
							gap="sm"
							className="alphone-layout__account"
						>
							<Text>{user.name}</Text>
							<Button
								variant="outline"
								disabled={signOut.isPending}
								onClick={() => signOut.mutate()}
							>
								Log out
							</Button>
							{signOut.isError ? (
								<Text role="alert">Logout failed, please try again.</Text>
							) : null}
						</Stack>
					) : null}
					{version ? (
						<Text className="alphone-layout__version">v{version}</Text>
					) : null}
				</div>
				<ThemeProvider color={CANVAS_COLOR}>
					<main className="alphone-layout__canvas">
						<Outlet />
					</main>
				</ThemeProvider>
			</div>
		</ThemeProvider>
	)
}
