// SPDX-License-Identifier: AGPL-3.0-or-later

import { getRouteApi, useNavigate } from '@tanstack/react-router'

import { ContactScreen } from './contacts/ContactScreen'
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

const contactRouteApi = getRouteApi('/contacts/$contactId')

/**
 * Renders the contact detail screen for the route's contact id.
 * @returns The contact route element.
 */
export function ContactRoute() {
	const { contactId } = contactRouteApi.useParams()
	return <ContactScreen contactId={contactId} />
}
