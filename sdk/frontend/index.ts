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

export interface ContactPanel {
	id: string
	Panel: ComponentType<{ contactId: string }>
}

export interface FrontendPlugin {
	id: string
	routes: (parent: AnyRoute) => AnyRoute[]
	nav: NavItem[]
	contactPanels?: ContactPanel[]
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
	Skeleton,
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
export {
	ErrorNotice,
	LoadingRows,
	LoadingScreen,
	LoadMore,
	Page as PageScreen,
	PageTitle,
} from '@gopherium/godmin'
export { useCanvas, useFrameLocation } from '@gopherium/godmin/router'
export { ValidationError, validationMessage } from './errors'
export { MANAGE_USERS, can, useSession } from './session'
export type { Capability, Role, Session } from './session'
export { createGraphClient, graphError, graphExtensions } from './graph'
export type { GraphClient } from './graph'
export { GraphProvider, useGraph } from './GraphProvider'
export { useConnection } from './useConnection'
export type { ConnectionPage, ConnectionResult } from './useConnection'
export {
	useQuery as useGraphQuery,
	useMutation as useGraphMutation,
	useSubscription as useGraphSubscription,
} from 'urql'
export type { CombinedError as GraphFailure } from 'urql'
export { SidebarNavigationScreen } from './SidebarNavigationScreen'
export { useGraphEvents, useGraphStream } from './stream'
export { __, _n, _nx, _x, sprintf } from '@wordpress/i18n'
export { displayLocale, formatDate } from '@gopherium/gottext'
