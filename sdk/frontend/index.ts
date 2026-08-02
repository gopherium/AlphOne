// SPDX-License-Identifier: AGPL-3.0-or-later

import type { AnyRoute } from '@tanstack/react-router'
import type { ComponentProps, ComponentType, ReactElement } from 'react'

declare module '@tanstack/react-router' {
	interface StaticDataRouteOption {
		Sidebar?: ComponentType
		canvas?: 'padded' | 'bleed'
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
	chevronLeft,
	chevronRight,
	comment,
	inbox,
	people,
} from '@wordpress/icons'
export { ThemeProvider } from '@wordpress/theme'
export { ValidationError, validationMessage } from './errors'
export { ErrorNotice, LoadMore, PageScreen } from './PageScreen'
export { SidebarNavigationScreen } from './SidebarNavigationScreen'
export { useEventStream } from './stream'
