// SPDX-License-Identifier: AGPL-3.0-or-later

import { Stack, Text, ThemeProvider } from '@alphone/frontend-sdk'
import { Link, Outlet } from '@tanstack/react-router'

import { plugins } from './plugins'

const CHROME_COLOR = { background: '#1e1e1e' }
const CANVAS_COLOR = { background: '#ffffff' }

/**
 * Renders the admin layout: a dark navigation chrome holding the branding and
 * plugin menu, wrapped around a light canvas showing the active route.
 * @returns The layout element framing the current route.
 */
export function Layout() {
	return (
		<ThemeProvider color={CHROME_COLOR}>
			<div className="alphone-layout">
				<div className="alphone-layout__sidebar">
					<Stack direction="column" gap="lg">
						<Link to="/">
							<Text variant="heading-lg" render={<h1 />}>
								AlphOne
							</Text>
						</Link>
						<Stack direction="column" gap="sm" render={<nav />}>
							{plugins.flatMap((plugin) =>
								plugin.nav.map((item) => (
									<Link key={item.to} to={item.to}>
										{item.label}
									</Link>
								)),
							)}
						</Stack>
					</Stack>
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
