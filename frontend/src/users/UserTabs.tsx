// SPDX-License-Identifier: AGPL-3.0-or-later

import { PageTab, PageTabs, __ } from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'

/** UserSection names the tab of the users page a screen shows. */
type UserSection = 'users' | 'tokens'

/**
 * Renders the Users and API tokens tabs of the users page, marking the one on screen.
 * @param props - The tab the screen shows.
 * @returns The tab navigation.
 */
export function UserTabs({ current }: { current: UserSection }) {
	return (
		<PageTabs label={__('User sections', 'alphone')}>
			<PageTab render={<Link to="/users" activeOptions={{ exact: true }} />} current={current === 'users'}>
				{__('Users', 'alphone')}
			</PageTab>
			<PageTab render={<Link to="/users/tokens" activeOptions={{ exact: true }} />} current={current === 'tokens'}>
				{__('API tokens', 'alphone')}
			</PageTab>
		</PageTabs>
	)
}
