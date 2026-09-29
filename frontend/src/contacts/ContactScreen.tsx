// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	IconButton,
	InputControl,
	LoadingScreen,
	LogItem,
	LogList,
	PageScreen,
	SectionTitle,
	SelectControl,
	Text,
	ValidationError,
	__,
	graphError,
	sprintf,
	graphExtensions,
	trash,
	useConnection,
	useGraph,
	useGraphMutation,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useId, useState } from 'react'

import { plugins } from '../plugins'
import { ContactTasks } from '../tasks/ContactTasks'
import { ContactPanels } from './ContactPanels'
import { channelItemOf, channelItems } from './channel'
import { formatCreated } from './format'
import {
	addContactIdentityMutation,
	contactDetailQuery,
	deleteContactIdentityMutation,
	renameContactMutation,
} from './operations'

const contactPanels = plugins.flatMap((plugin) => plugin.contactPanels ?? [])
const pluginChannels = plugins.flatMap((plugin) => plugin.channels ?? [])
const contactTasksPageSize = 50
const contactDetailOperation = 'ContactDetail'

/** ContactDetail is the contact the screen renders, identities included. */
export interface ContactDetail {
	id: string
	name: string
	createdAt: string
	identities: {
		id: string
		channel: string
		identifier: string
		displayName: string
	}[]
}

/**
 * Renders one contact's detail: its name, identities and tasks beside its creation date and panels.
 * @returns The contact screen.
 */
export function ContactScreen({ contactId }: { contactId: string }) {
	const detail = useConnection({
		query: contactDetailQuery,
		variables: { id: contactId, first: contactTasksPageSize },
		select: (data) => data.contact?.tasks,
	})
	const identitiesHeading = useId()
	const contact = detail.data?.contact

	if (detail.isPending) {
		return (
			<PageScreen title={__('Contact', 'alphone')}>
				<LoadingScreen label={__('Loading contact…', 'alphone')} />
			</PageScreen>
		)
	}
	if (detail.isError || !contact) {
		return <ErrorNotice>{__('The contact could not be loaded.', 'alphone')}</ErrorNotice>
	}
	return (
		<PageScreen
			title={contact.name}
			aside={
				<>
					<Text className="alphone-contacts__created">
						{sprintf(__('Created %(date)s', 'alphone'), { date: formatCreated(new Date(contact.createdAt)) })}
					</Text>
					<ContactPanels contactId={contact.id} panels={contactPanels} />
				</>
			}
		>
			<RenameForm key={contact.name} contact={contact} />
			<SectionTitle id={identitiesHeading}>{__('Identities', 'alphone')}</SectionTitle>
			<IdentityList contact={contact} labelledBy={identitiesHeading} />
			<AddIdentityForm contact={contact} />
			<ContactTasks contactId={contact.id} tasks={detail} />
		</PageScreen>
	)
}

/**
 * Returns the error an identity write should show, naming the owner of a claim.
 * @param error - The failure the mutation answered with.
 * @returns The mapped error, or undefined when the write succeeded.
 */
function identityError(error: Parameters<typeof graphError>[0]): Error | undefined {
	const owner = graphExtensions(error).ownerName
	if (typeof owner === 'string') {
		return new ValidationError(sprintf(__('Already on contact %(name)s.', 'alphone'), { name: owner }))
	}
	return graphError(error)
}

/**
 * Returns the callback refreshing the contact after a write.
 * @returns The refresh callback.
 */
function useContactRefresh() {
	const graph = useGraph()
	return () => {
		graph.refetch([contactDetailOperation])
	}
}

/**
 * Returns the name the channel select or a plugin gives a channel, or the channel itself when none does.
 * @param channel - The channel an identity belongs to.
 * @returns The channel name to show.
 */
function channelName(channel: string): string {
	return [...channelItems(), ...pluginChannels].find((item) => item.value === channel)?.label ?? channel
}

/**
 * Returns the line one identity shows: its channel name, its identifier and any label.
 * @param identity - The identity to describe.
 * @returns The identity line.
 */
function identityText(identity: ContactDetail['identities'][number]): string {
	const text = `${channelName(identity.channel)}: ${identity.identifier}`
	return identity.displayName === '' ? text : `${text} (${identity.displayName})`
}

/**
 * Renders the contact's identities as a log list, each with its trash at the row end, or a placeholder when none exist.
 * @param props - The contact and the id of the heading that names the list.
 * @returns The identity list.
 */
function IdentityList({ contact, labelledBy }: { contact: ContactDetail; labelledBy: string }) {
	const settled = useContactRefresh()
	const [remove, runRemove] = useGraphMutation(deleteContactIdentityMutation)
	const removeIdentity = async (identityId: string) => {
		const result = await runRemove({ contactId: contact.id, identityId })
		if (result.data) {
			settled()
		}
	}

	if (contact.identities.length === 0) {
		return <Text role="status">{__('No identities yet.', 'alphone')}</Text>
	}
	return (
		<>
			<LogList aria-labelledby={labelledBy}>
				{contact.identities.map((identity) => (
					<LogItem
						key={identity.id}
						aria-label={identityText(identity)}
						label={<Text>{identityText(identity)}</Text>}
						actions={
							<IconButton
								icon={trash}
								variant="minimal"
								tone="neutral"
								size="compact"
								label={sprintf(__('Remove %(identifier)s', 'alphone'), { identifier: identity.identifier })}
								loading={remove.fetching}
								onClick={() => void removeIdentity(identity.id)}
							/>
						}
					/>
				))}
			</LogList>
			{remove.error ? (
				<ErrorNotice>{__('The identity could not be removed.', 'alphone')}</ErrorNotice>
			) : null}
		</>
	)
}

/**
 * Renders the form that attaches a new identity to the contact.
 * @returns The add identity form.
 */
function AddIdentityForm({ contact }: { contact: ContactDetail }) {
	const settled = useContactRefresh()
	const [channel, setChannel] = useState('email')
	const [identifier, setIdentifier] = useState('')
	const [label, setLabel] = useState('')
	const [add, runAdd] = useGraphMutation(addContactIdentityMutation)
	const channels = channelItems()
	const submitIdentity = async () => {
		const result = await runAdd({
			contactId: contact.id,
			identity: { channel, identifier, displayName: label },
		})
		if (result.data) {
			setIdentifier('')
			setLabel('')
			settled()
		}
	}

	return (
		<form
			className="godmin-form godmin-form--inline"
			onSubmit={(event) => {
				event.preventDefault()
				void submitIdentity()
			}}
		>
			<div className="godmin-form__row">
				<SelectControl
					label={__('Channel', 'alphone')}
					items={channels}
					value={channels.find((option) => option.value === channel)}
					onValueChange={(item) => setChannel(channelItemOf(item).value)}
				/>
				<InputControl
					label={__('Value', 'alphone')}
					value={identifier}
					onChange={(event) => setIdentifier(event.target.value)}
				/>
				<InputControl
					label={__('Label', 'alphone')}
					value={label}
					onChange={(event) => setLabel(event.target.value)}
				/>
				<Button
					type="submit"
					disabled={identifier.trim() === '' || add.fetching}
					loading={add.fetching}
				>
					{__('Add identity', 'alphone')}
				</Button>
			</div>
			{add.error ? (
				<ErrorNotice>
					{validationMessage(identityError(add.error), __('The identity could not be added.', 'alphone'))}
				</ErrorNotice>
			) : null}
		</form>
	)
}

/**
 * Renders the rename form for a loaded contact.
 * @returns The rename form.
 */
function RenameForm({ contact }: { contact: ContactDetail }) {
	const settled = useContactRefresh()
	const [name, setName] = useState(contact.name)
	const [rename, runRename] = useGraphMutation(renameContactMutation)
	const submitRename = async () => {
		const result = await runRename({ id: contact.id, name })
		if (result.data) {
			settled()
		}
	}

	return (
		<form
			className="godmin-form godmin-form--inline"
			onSubmit={(event) => {
				event.preventDefault()
				void submitRename()
			}}
		>
			<div className="godmin-form__row">
				<InputControl
					label={__('Name', 'alphone')}
					value={name}
					onChange={(event) => setName(event.target.value)}
				/>
				<Button
					type="submit"
					disabled={name.trim() === '' || rename.fetching}
					loading={rename.fetching}
				>
					{__('Save', 'alphone')}
				</Button>
			</div>
			{rename.error ? (
				<ErrorNotice>
					{validationMessage(graphError(rename.error), __('The contact could not be renamed.', 'alphone'))}
				</ErrorNotice>
			) : null}
		</form>
	)
}
