// SPDX-License-Identifier: AGPL-3.0-or-later

import { Drawer, IconButton, Text, VisuallyHidden, menu } from '@alphone/frontend-sdk'
import { useRouterState } from '@tanstack/react-router'
import { useEffect, useState } from 'react'

import { RailContent } from './RailContent'

/**
 * Renders the small viewport chrome: a menu button opening the navigation in a
 * drawer, beside the brand.
 * @returns The top bar element.
 */
export function TopBar() {
	const [open, setOpen] = useState(false)
	const pathname = useRouterState({ select: (state) => state.location.pathname })
	useEffect(() => {
		setOpen(false)
	}, [pathname])
	return (
		<div className="alphone-layout__topbar">
			<Drawer.Root open={open} onOpenChange={setOpen}>
				<Drawer.Trigger
					render={<IconButton icon={menu} label="Open navigation" />}
				/>
				<Drawer.Popup className="alphone-layout__drawer">
					<VisuallyHidden render={<Drawer.Title />}>Navigation</VisuallyHidden>
					<RailContent />
				</Drawer.Popup>
			</Drawer.Root>
			<Text variant="heading-lg">AlphOne</Text>
		</div>
	)
}
