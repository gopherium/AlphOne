// SPDX-License-Identifier: AGPL-3.0-or-later

import { Card, Stack, Text } from '@alphone/frontend-sdk'
import { Link, Outlet } from '@tanstack/react-router'

import { plugins } from './plugins'

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
