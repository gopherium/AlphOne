// SPDX-License-Identifier: AGPL-3.0-or-later

import type { AnyRoute } from '@tanstack/react-router'
import type { ComponentProps, ReactElement } from 'react'

export interface NavItem {
	label: string
	to: string
	icon: ReactElement<ComponentProps<'svg'>>
}

export interface FrontendPlugin {
	id: string
	routes: (parent: AnyRoute) => AnyRoute[]
	nav: NavItem[]
}

export { Badge, Button, Card, Icon, Stack, Text } from '@wordpress/ui'
export { ThemeProvider } from '@wordpress/theme'
