// SPDX-License-Identifier: AGPL-3.0-or-later

import type { AnyRoute } from '@tanstack/react-router'
import type { ComponentProps, ComponentType, ReactElement } from 'react'

declare module '@tanstack/react-router' {
	interface StaticDataRouteOption {
		Sidebar?: ComponentType
	}
}

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

export {
	Badge,
	Button,
	Card,
	Checkbox,
	Collapsible,
	EmptyState,
	Icon,
	IconButton,
	InputControl,
	Link,
	Notice,
	SelectControl,
	Stack,
	Text,
	VisuallyHidden,
} from '@wordpress/ui'
export {
	calendar,
	chevronLeft,
	chevronRight,
	inbox,
} from '@wordpress/icons'
export { ThemeProvider } from '@wordpress/theme'
export { sessionQueryKey } from '@gopherium/react-auth'
export { SidebarNavigationScreen } from './SidebarNavigationScreen'
export { useEventStream } from './stream'
