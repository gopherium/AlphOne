// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, Notice, Stack, Text } from '@wordpress/ui'
import type { ComponentProps, ReactNode } from 'react'

interface PageScreenProps {
	title: string
	subtitle?: string
	actions?: ReactNode
	className?: string
	children: ReactNode
}

type TextVariant = ComponentProps<typeof Text>['variant']

/**
 * Renders a screen's title as the page heading.
 * @param children - The title text.
 * @param variant - The type scale to render the title at.
 * @returns The page title element.
 */
export function PageTitle({
	children,
	variant = 'heading-xl',
}: {
	children: ReactNode
	variant?: TextVariant
}) {
	return (
		<Text variant={variant} render={<h1 />}>
			{children}
		</Text>
	)
}

/**
 * Renders a canvas screen: the page title as its heading top left, an
 * optional subtitle under it, optional actions top right, and the content.
 * @returns The page screen element.
 */
export function PageScreen({ title, subtitle, actions, className, children }: PageScreenProps) {
	const classes = className === undefined ? 'alphone-page' : `alphone-page ${className}`
	return (
		<Stack direction="column" gap="lg" className={classes}>
			<Stack direction="row" gap="md" align="center" justify="space-between">
				<Stack direction="column" gap="xs">
					<PageTitle>{title}</PageTitle>
					{subtitle !== undefined && (
						<Text variant="body-sm" className="alphone-page__subtitle">
							{subtitle}
						</Text>
					)}
				</Stack>
				{actions !== undefined && (
					<Stack direction="row" gap="sm" align="center">
						{actions}
					</Stack>
				)}
			</Stack>
			{children}
		</Stack>
	)
}

/**
 * Renders a failure message that a screen reader announces.
 * @returns The error notice element.
 */
export function ErrorNotice({ children }: { children: ReactNode }) {
	return (
		<Notice.Root intent="error" role="alert">
			<Notice.Description>{children}</Notice.Description>
		</Notice.Root>
	)
}

interface LoadMoreQuery {
	hasNextPage: boolean
	isFetchingNextPage: boolean
	fetchNextPage: () => Promise<unknown>
}

/**
 * Renders the cursor pagination button for an infinite query, or nothing
 * when every page is loaded.
 * @returns The load more button, or null.
 */
export function LoadMore({ query, children }: { query: LoadMoreQuery; children: ReactNode }) {
	if (!query.hasNextPage) {
		return null
	}
	return (
		<Button
			variant="minimal"
			tone="neutral"
			size="compact"
			onClick={() => void query.fetchNextPage()}
			disabled={query.isFetchingNextPage}
		>
			{children}
		</Button>
	)
}
