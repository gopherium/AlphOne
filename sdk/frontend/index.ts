// SPDX-License-Identifier: AGPL-3.0-or-later

import type { MenuItem } from '@gopherium/godmin/router'
import type { Catalog } from '@gopherium/gottext'
import type { AnyRoute } from '@tanstack/react-router'
import type { ComponentType } from 'react'

import type { GraphKeys } from './graph'
import type { Capability } from './session'

export interface ContactPanel {
	id: string
	Panel: ComponentType<{ contactId: string }> & { preload?: () => Promise<void> | undefined }
}

export interface PluginLocale {
	domain: string
	load: (locale: string) => Promise<Catalog | undefined>
}

export interface ChannelName {
	value: string
	label: string
}

export interface TokenArea {
	value: string
	label: string
}

/** AccountLink is an address outside the admin that a plugin shows under the account panel. */
export interface AccountLink {
	/** label is the text the link shows. */
	label: string
	/** href is the address the link opens. */
	href: string
	/** capability is the capability the reader must hold to see the link. */
	capability?: Capability
	/** openInNewTab opens the address in a new tab. */
	openInNewTab?: boolean
}

export interface FrontendPlugin {
	id: string
	routes: (parent: AnyRoute) => AnyRoute[]
	nav: MenuItem[]
	accountLinks?: AccountLink[]
	contactPanels?: ContactPanel[]
	channels?: ChannelName[]
	tokenAreas?: TokenArea[]
	graphKeys?: GraphKeys
	locale?: PluginLocale
	errorTemplates?: () => Record<string, string>
}

export {
	Badge,
	Button,
	ButtonLink,
	Card,
	Checkbox,
	Collapsible,
	CollapsibleCard,
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
	TextareaControl,
	ValidatedInputControl,
	VisuallyHidden,
} from '@wordpress/ui'
export {
	calendar,
	check,
	chevronLeft,
	chevronRight,
	close,
	comment,
	commentAuthorAvatar,
	commentReplyLink,
	envelope,
	file,
	formatBold,
	formatItalic,
	formatListBullets,
	formatListNumbered,
	inbox,
	inlineImage,
	institution,
	key,
	language,
	link,
	list,
	lock,
	menu,
	moveTo,
	notAllowed,
	pencil,
	people,
	quote,
	trash,
	upload,
} from '@wordpress/icons'
export { ThemeProvider } from '@wordpress/theme'
export {
	ConfirmBody,
	ErrorNotice,
	FileButton,
	FormBody,
	InitialsAvatar,
	keyFromLabel,
	ListEmpty,
	ListNoResults,
	LoadingScreen,
	LoadMore,
	LogItem,
	LogList,
	LogTime,
	markLatest,
	MenuCount,
	NavigationScreen,
	Page as PageScreen,
	PageBreadcrumb,
	PageBreadcrumbs,
	PageTab,
	PageTabs,
	PageTitle,
	pageWindow,
	paginationOf,
	RenameBody,
	RepeatRows,
	RowControls,
	runEach,
	RunConfirm,
	SectionTitle,
	Toaster,
	useBulkRun,
	useListRun,
	useServerPaging,
	useToaster,
	useUnsavedText,
	viewNarrows,
} from '@gopherium/godmin'
export type {
	BulkFailure,
	BulkOutcome,
	BulkRun,
	BulkWords,
	ConfirmBodyProps,
	FormBodyProps,
	FormKind,
	ListRun,
	ListRunState,
	PagedView,
	PageWindow,
	PaginationInfo,
	RenameBodyProps,
	RowLabels,
	RunConfirmProps,
	RunSettled,
	ServedPage,
	ServedSize,
	ServerPaging,
	ToastAction,
	ToasterHandle,
	UnsavedText,
} from '@gopherium/godmin'
export { DrilldownItem, NavigationItem, NavigationList } from '@gopherium/godmin/router'
export type { MenuItem } from '@gopherium/godmin/router'
export { channelName, configureChannelNames } from './channels'
export { ValidationError, validationMessage } from './errors'
export { useLastAnswer } from './lastAnswer'
export { useRefusedField } from './refusedField'
export { MANAGE_USERS, useSession } from './session'
export { can } from '@gopherium/react-auth'
export type { Capability, Role, Session } from './session'
export {
	configureErrorText,
	createGraphClient,
	graphError,
	graphExtensions,
	graphFetch,
	reasonText,
	writeFailure,
} from './graph'
export type { ErrorCopy, StoredReason } from './graph'
export type { GraphClient, GraphKeys } from './graph'
export { GraphProvider, useGraph } from './GraphProvider'
export { useConnection } from './useConnection'
export type { ConnectionPage, ConnectionResult } from './useConnection'
export { listSearch, openOnTap, useListView } from './listView'
export type { ListDefaults, ListSearch, ListView } from './listView'
export { useAdminSettings } from './adminSettings'
export { FormatLocaleGate } from './FormatLocaleGate'
export type { AdminSettings, AdminSettingsRead } from './adminSettings'
export {
	stringifyDocument as printGraphDocument,
	useQuery as useGraphQuery,
	useMutation as useGraphMutation,
	useSubscription as useGraphSubscription,
} from 'urql'
export type { CombinedError as GraphFailure } from 'urql'
export { useGraphEvents, useGraphStream } from './stream'
export { __, _n, _nx, _x, sprintf } from '@wordpress/i18n'
export {
	displayLocale,
	formatDate,
	formatList,
	formatMoney,
	formatNumber,
	formatTime,
	formatWeekday,
	globCatalogs,
	rememberFormatLocale,
	rememberLocale,
} from '@gopherium/gottext'
export type { Catalog } from '@gopherium/gottext'
