// SPDX-License-Identifier: AGPL-3.0-or-later

import { Link } from '@tanstack/react-router'
import { Icon, Stack, Text } from '@wordpress/ui'
import type { ReactNode } from 'react'

const chevronLeft = (
	<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" fill="currentColor">
		<path d="M14.6 7l-1.2-1L8 12l5.4 6 1.2-1-4.6-5z" />
	</svg>
)

interface SidebarNavigationScreenProps {
	title: string
	backTo?: string
	description?: ReactNode
	actions?: ReactNode
	footer?: ReactNode
	children: ReactNode
}

/**
 * Renders a drill-down sidebar screen: an optional back link to the parent
 * layer, the title, optional description and actions, the content, and an
 * optional footer.
 * @returns The sidebar screen element.
 */
export function SidebarNavigationScreen({
	title,
	backTo,
	description,
	actions,
	footer,
	children,
}: SidebarNavigationScreenProps) {
	return (
		<Stack direction="column" gap="md" className="alphone-nav-screen">
			<Stack direction="row" align="flex-start" gap="sm">
				{backTo !== undefined && (
					<Link
						to={backTo}
						aria-label="Back"
						className="alphone-nav-screen__back"
					>
						<Icon icon={chevronLeft} size={24} aria-hidden />
					</Link>
				)}
				<Text
					variant="heading-md"
					render={<h2 />}
					className="alphone-nav-screen__title"
				>
					{title}
				</Text>
				{!!actions && (
					<div className="alphone-nav-screen__actions">{actions}</div>
				)}
			</Stack>
			<Stack direction="column" gap="sm">
				{!!description && (
					<div className="alphone-nav-screen__description">{description}</div>
				)}
				{children}
			</Stack>
			{!!footer && <div className="alphone-nav-screen__footer">{footer}</div>}
		</Stack>
	)
}
