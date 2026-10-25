// SPDX-License-Identifier: AGPL-3.0-or-later

import { NavigationScreen } from '@alphone/frontend-sdk'
import type { FrontendPlugin } from '@alphone/frontend-sdk'
import { createRoute } from '@tanstack/react-router'

/**
 * Renders the sub menu the section opens.
 * @returns The section's sidebar screen.
 */
function SectionSidebar() {
	return (
		<NavigationScreen title="Section">
			<p>section sidebar</p>
		</NavigationScreen>
	)
}

/**
 * Renders the page the section opens.
 * @returns The section's canvas.
 */
function SectionCanvas() {
	return <p>section canvas</p>
}

/** A plugin whose one row opens a sub menu. */
export const sectionPlugin: FrontendPlugin = {
	id: 'section',
	nav: [{ label: 'Section', to: '/section' }],
	routes: (parent) => [
		createRoute({
			getParentRoute: () => parent,
			path: '/section',
			staticData: { Sidebar: SectionSidebar },
			component: SectionCanvas,
		}),
	],
}
