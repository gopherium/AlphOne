// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	RouterProvider,
	createMemoryHistory,
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect, test } from 'vitest'

import { SidebarNavigationScreen } from './SidebarNavigationScreen'

function renderScreen(ui: ReactNode) {
	const rootRoute = createRootRoute({
		component: function Host() {
			return <>{ui}</>
		},
	})
	const indexRoute = createRoute({
		getParentRoute: () => rootRoute,
		path: '/',
		component: function Blank() {
			return null
		},
	})
	const router = createRouter({
		routeTree: rootRoute.addChildren([indexRoute]),
		history: createMemoryHistory({ initialEntries: ['/'] }),
	})
	return render(<RouterProvider router={router} />)
}

test('renders the title, back link, description, actions, and content', async () => {
	renderScreen(
		<SidebarNavigationScreen
			title="WhatsApp"
			backTo="/"
			description="Your conversations"
			actions={<button type="button">New</button>}
			footer={<span>footer text</span>}
		>
			<p>screen content</p>
		</SidebarNavigationScreen>,
	)

	expect(
		await screen.findByRole('heading', { name: 'WhatsApp' }),
	).toBeInTheDocument()
	expect(screen.getByRole('link', { name: 'Back' })).toBeInTheDocument()
	expect(screen.getByText('Your conversations')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'New' })).toBeInTheDocument()
	expect(screen.getByText('screen content')).toBeInTheDocument()
	expect(screen.getByText('footer text')).toBeInTheDocument()
	expect(screen.queryByRole('contentinfo')).toBeNull()
})

test('omits the back link and optional regions on a root screen', async () => {
	const { container } = renderScreen(
		<SidebarNavigationScreen title="Home">
			<p>root content</p>
		</SidebarNavigationScreen>,
	)

	expect(
		await screen.findByRole('heading', { name: 'Home' }),
	).toBeInTheDocument()
	expect(screen.queryByRole('link', { name: 'Back' })).toBeNull()
	expect(screen.getByText('root content')).toBeInTheDocument()
	expect(container.querySelector('.alphone-nav-screen__description')).toBeNull()
	expect(container.querySelector('.alphone-nav-screen__actions')).toBeNull()
	expect(container.querySelector('.alphone-nav-screen__footer')).toBeNull()
})
