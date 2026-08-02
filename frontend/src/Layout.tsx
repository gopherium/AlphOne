// SPDX-License-Identifier: AGPL-3.0-or-later

import { ThemeProvider, useEventStream } from '@alphone/frontend-sdk'
import { Outlet, useRouterState } from '@tanstack/react-router'
import type { StaticDataRouteOption } from '@tanstack/react-router'

import { RailContent } from './RailContent'
import { TopBar } from './TopBar'
import { useMediaQuery } from './useMediaQuery'

const CHROME_COLOR = { background: '#1e1e1e' }
const CANVAS_COLOR = { background: '#ffffff' }
const CANVAS_CLASS = 'alphone-layout__canvas'
const SMALL_VIEWPORT = '(max-width: 1023px)'

/**
 * Returns the canvas class for the given route matches.
 * @param matches - The router matches for the active route.
 * @returns The canvas class, carrying the bleed modifier when a match asks for it.
 */
function canvasClass(matches: { staticData: StaticDataRouteOption }[]): string {
	const bleed = matches.some((match) => match.staticData.canvas === 'bleed')
	return bleed ? `${CANVAS_CLASS} ${CANVAS_CLASS}--bleed` : CANVAS_CLASS
}

/**
 * Renders the admin layout: a dark navigation chrome wrapped around a light
 * canvas showing the active route.
 * @returns The layout element framing the current route.
 */
export function Layout() {
	useEventStream('/api/events', { invalidateKeys: [['tasks'], ['contacts']] })
	const matches = useRouterState({ select: (state) => state.matches })
	const small = useMediaQuery(SMALL_VIEWPORT)
	return (
		<ThemeProvider color={CHROME_COLOR}>
			<div className={small ? 'alphone-layout alphone-layout--small' : 'alphone-layout'}>
				{small ? (
					<TopBar />
				) : (
					<div className="alphone-layout__sidebar">
						<RailContent />
					</div>
				)}
				<ThemeProvider color={CANVAS_COLOR}>
					<main className={canvasClass(matches)}>
						<Outlet />
					</main>
				</ThemeProvider>
			</div>
		</ThemeProvider>
	)
}
