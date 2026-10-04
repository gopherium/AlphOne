// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, InitialsAvatar, Stack, __, channelName, formatDate } from '@alphone/frontend-sdk'
import type { Field } from '@alphone/frontend-sdk/dataviews'

import type { ContactPageQuery } from '../gql/graphql'
import { channelNames } from './channel'

/** ContactRow is one contact as the contacts list shows it. */
export type ContactRow = ContactPageQuery['contactPage']['items'][number]

/**
 * Returns every channel a contact is reached on, each once, in the order its identities hold them.
 * @param contact - The contact.
 * @returns The channels.
 */
function channelsOf(contact: ContactRow): string[] {
	return [...new Set(contact.identities.map((identity) => identity.channel))]
}

/**
 * Builds the fields the contacts list shows, offering as channels every one the core and the plugins name.
 * @returns The fields.
 */
export function contactFields(): Field<ContactRow>[] {
	return [
		{
			id: 'name',
			label: __('Name', 'alphone'),
			type: 'text',
			enableHiding: false,
			filterBy: false,
		},
		{
			id: 'identity',
			label: __('Identity', 'alphone'),
			render: ({ item }) => item.identities[0]?.identifier ?? '',
			enableSorting: false,
			filterBy: false,
		},
		{
			id: 'avatar',
			label: __('Avatar', 'alphone'),
			render: ({ item }) => <InitialsAvatar name={item.name} />,
			enableSorting: false,
			filterBy: false,
		},
		{
			id: 'channels',
			label: __('Channels', 'alphone'),
			type: 'array',
			elements: channelNames(),
			enableSorting: false,
			filterBy: { operators: ['isAny'] },
			render: ({ item }) => (
				<Stack direction="row" gap="xs" wrap="nowrap">
					{channelsOf(item).map((channel) => (
						<Badge key={channel} intent="none" className="alphone-contacts__channel">
							{channelName(channel)}
						</Badge>
					))}
				</Stack>
			),
		},
		{
			id: 'created',
			label: __('Created', 'alphone'),
			render: ({ item }) => formatDate(item.createdAt),
			filterBy: false,
		},
	]
}
