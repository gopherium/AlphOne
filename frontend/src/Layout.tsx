// SPDX-License-Identifier: AGPL-3.0-or-later

import { Text, useCanvas, useFrameLocation, useGraph, useGraphStream } from '@alphone/frontend-sdk'
import { Frame } from '@gopherium/godmin'
import { Outlet } from '@tanstack/react-router'

import { RailContent } from './RailContent'

const CHROME_COLOR = { background: '#1e1e1e' }
const CANVAS_COLOR = { background: '#ffffff' }

/**
 * Renders the admin layout: a dark navigation chrome wrapped around a light
 * canvas showing the active route.
 * @returns The layout element framing the current route.
 */
export function Layout() {
	const graph = useGraph()
	useGraphStream({
		graph,
		operations: ['DayTasks', 'OverdueTasks', 'Contacts', 'ContactDetail'],
	})
	const location = useFrameLocation()
	const canvas = useCanvas()
	return (
		<Frame.Root location={location} chromeColor={CHROME_COLOR} canvasColor={CANVAS_COLOR}>
			<Frame.Rail brand={<Text variant="heading-lg">AlphOne</Text>}>
				<RailContent />
			</Frame.Rail>
			<Frame.Canvas canvas={canvas}>
				<Outlet />
			</Frame.Canvas>
		</Frame.Root>
	)
}
