// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, can, sprintf, Text, useGraph, useGraphStream, useSession } from '@alphone/frontend-sdk'
import { Frame } from '@gopherium/godmin'
import { Navigation, useCanvas, useFrameLocation } from '@gopherium/godmin/router'
import { AccountPanel } from '@gopherium/react-auth/wpds'
import { Link, Outlet } from '@tanstack/react-router'

import { LANDING } from './landing'
import { coreNav } from './menu/coreNav'
import { plugins } from './plugins'
import { useAppVersion } from './version'

/**
 * Renders the admin layout framing the active route.
 * @returns The layout element.
 */
export function Layout() {
	const graph = useGraph()
	useGraphStream({
		graph,
		operations: ['DayTasks', 'OverdueTasks', 'Contacts', 'ContactDetail'],
	})
	const session = useSession()
	const version = useAppVersion()
	return (
		<Frame.Root location={useFrameLocation()}>
			<Frame.Sidebar
				brand={<Text variant="heading-lg">{'AlphOne'}</Text>}
				footer={<AccountPanel />}
				footerNote={version ? sprintf(__('v%(version)s', 'alphone'), { version }) : undefined}
			>
				<Navigation
					title={'AlphOne'}
					items={[...coreNav, ...plugins.flatMap((plugin) => plugin.nav)]}
					home={<Link to={LANDING} />}
					can={(capability) => can(session, capability)}
				/>
			</Frame.Sidebar>
			<Frame.Canvas canvas={useCanvas()}>
				<Outlet />
			</Frame.Canvas>
		</Frame.Root>
	)
}
