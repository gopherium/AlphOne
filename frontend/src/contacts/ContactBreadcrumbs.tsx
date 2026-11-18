// SPDX-License-Identifier: AGPL-3.0-or-later

import { PageBreadcrumb, PageBreadcrumbs, __ } from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'

/**
 * Renders the links that lead from a contact page back to the contacts.
 * @returns The breadcrumbs.
 */
export function ContactBreadcrumbs() {
	return (
		<PageBreadcrumbs label={__('Breadcrumbs', 'alphone')}>
			<PageBreadcrumb render={<Link to="/contacts" activeOptions={{ exact: true }} />}>
				{__('Contacts', 'alphone')}
			</PageBreadcrumb>
		</PageBreadcrumbs>
	)
}
