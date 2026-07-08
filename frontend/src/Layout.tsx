// SPDX-License-Identifier: AGPL-3.0-or-later

import { Card, Stack, Text } from '@alphone/frontend-sdk'
import { Link, Outlet } from '@tanstack/react-router'

import { plugins } from './plugins'

/**
 * Renders the application shell with a header, plugin navigation links, and an outlet for routed content.
 * @returns The layout element containing the branding, navigation, and a card wrapping the active route.
 */
export function Layout() {
	return (
		<Stack direction="column" gap="lg">
			<Stack direction="row" gap="xl" align="baseline" render={<header />}>
				<Link to="/">
					<Text variant="heading-lg" render={<h1 />}>
						AlphOne
					</Text>
				</Link>
				<Stack direction="row" gap="md" render={<nav />}>
					{plugins.flatMap((plugin) =>
						plugin.nav.map((item) => (
							<Link key={item.to} to={item.to}>
								{item.label}
							</Link>
						)),
					)}
				</Stack>
			</Stack>
			<Card.Root>
				<Card.Content>
					<Outlet />
				</Card.Content>
			</Card.Root>
		</Stack>
	)
}
