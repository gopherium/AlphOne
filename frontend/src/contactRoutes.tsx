// SPDX-License-Identifier: AGPL-3.0-or-later

import { getRouteApi, useNavigate } from '@tanstack/react-router'

import { ContactScreen } from './contacts/ContactScreen'
import { ContactsScreen } from './contacts/ContactsScreen'
import { NewContactScreen } from './contacts/NewContactScreen'

/**
 * Renders the contact list screen.
 * @returns The contacts route element.
 */
export function ContactsRoute() {
	return <ContactsScreen />
}

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
