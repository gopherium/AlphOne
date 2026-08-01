// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, Stack, Text } from '@wordpress/ui'
import type { ReactNode } from 'react'

interface PageScreenProps {
	title: string
	subtitle?: string
	actions?: ReactNode
	className?: string
	children: ReactNode
}

/**
 * Renders a canvas screen: the page title as its heading top left, an
 * optional subtitle under it, optional actions top right, and the content.
 * @returns The page screen element.
 */
export function PageScreen({ title, subtitle, actions, className, children }: PageScreenProps) {
	return (
		<Stack direction="column" gap="lg" className={className}>
			<Stack direction="row" gap="md" align="center" justify="space-between">
				<Stack direction="column" gap="xs">
					<Text variant="heading-xl" render={<h1 />}>
						{title}
					</Text>
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
