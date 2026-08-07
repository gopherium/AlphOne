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
	Checkbox,
	Collapsible,
	Drawer,
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
	menu,
	people,
} from '@wordpress/icons'
export { ThemeProvider } from '@wordpress/theme'
export { ErrorNotice, LoadMore, Page as PageScreen, PageTitle } from '@gopherium/godmin'
export { useCanvas, useFrameLocation } from '@gopherium/godmin/router'
export { ValidationError, validationMessage } from './errors'
export { createGraphClient, graphError } from './graph'
export type { GraphClient } from './graph'
export { SidebarNavigationScreen } from './SidebarNavigationScreen'
export { useEventStream, useGraphStream } from './stream'
