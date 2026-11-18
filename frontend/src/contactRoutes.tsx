// SPDX-License-Identifier: AGPL-3.0-or-later

import { useNavigate } from '@tanstack/react-router'

import { NewContactScreen } from './contacts/NewContactScreen'

/**
 * Renders the new-contact form, opening the created contact's detail on
 * success.
 * @returns The new contact route element.
 */
export function NewContactRoute() {
	const navigate = useNavigate()
	return (
		<NewContactScreen
			onCreated={(created) =>
				void navigate({ to: '/contacts/$contactId', params: { contactId: created.id } })
			}
		/>
	)
}
