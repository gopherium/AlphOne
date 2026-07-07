// SPDX-License-Identifier: AGPL-3.0-or-later

import { Link, Outlet } from '@tanstack/react-router'

import { Card, Stack, Text } from './ui'

export function Layout() {
	return (
		<Stack direction="column" gap="lg">
			<Link to="/">
				<Text variant="heading-lg" render={<h1 />}>
					AlphOne
				</Text>
			</Link>
			<Card.Root>
				<Card.Content>
					<Outlet />
				</Card.Content>
			</Card.Root>
		</Stack>
	)
}
