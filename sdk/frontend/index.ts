// SPDX-License-Identifier: AGPL-3.0-or-later

import type { AnyRoute } from '@tanstack/react-router'

export interface NavItem {
	label: string
	to: string
}

export interface FrontendPlugin {
	id: string
	routes: (parent: AnyRoute) => AnyRoute[]
	nav: NavItem[]
}

export { Badge, Button, Card, Stack, Text } from '@wordpress/ui'
export { ThemeProvider } from '@wordpress/theme'
