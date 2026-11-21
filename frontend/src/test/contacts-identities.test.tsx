// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { configureAppErrorText } from '../i18n/errors'
import {
	anaID,
	brunoID,
	detailFor,
	expectFieldIn,
	formRowChildren,
	identityID1,
	identityID2,
	serveAnaGarcia,
} from './contactScreen'
import type { IdentityRow } from './contactScreen'
import { busyClasses, renderAt, renderLimitedAt, signalsOf } from './render'

/** identityID3 is the id of the third identity a removal test serves. */
const identityID3 = '0198c000-0000-7000-8000-000000000013'

/**
 * Presses the trash of one identity and waits for the question it opens.
 * @param identifier - The identifier the trash is named after.
 * @returns The question.
 */
async function askToRemove(identifier: string) {
	await userEvent.click(screen.getByRole('button', { name: `Remove ${identifier}` }))
	return screen.findByRole('alertdialog', { name: `Remove ${identifier}?` })
}

/** The answer of a value another contact already holds. */
const heldByBruno = {
	data: null,
	errors: [{
		message: 'contact: identity already exists',
		extensions: { code: 'CONFLICT', ownerContactId: brunoID, ownerName: 'Bruno' },
	}],
}

/**
 * Opens the contact and presses Add identity with an email in Value.
 */
async function addEmailIdentity() {
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })
	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))
}

beforeEach(serveAnaGarcia)

test('adds an email identity to the contact', async () => {
	const identities = [
		{ id: identityID1, channel: 'whatsapp', identifier: '184467235', display_name: 'Ana G' },
	]
	let posted: Record<string, unknown> | null = null
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: { contact: detailFor(String(variables.id), 'Ana García', identities) },
			}),
		),
		graphql.mutation('AddContactIdentity', ({ variables }) => {
			posted = variables.identity as Record<string, unknown>
			const created = {
				id: identityID2,
				channel: 'email',
				identifier: 'maria@example.com',
				display_name: 'Work',
			}
			identities.push(created)
			return HttpResponse.json({
				data: {
					addContactIdentity: {
						__typename: 'ContactIdentity',
						id: created.id,
						channel: created.channel,
						identifier: created.identifier,
						displayName: created.display_name,
					},
				},
			})
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), ' Maria@Example.COM ')
	await userEvent.type(screen.getByLabelText('Label'), 'Work')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByText('Email: maria@example.com (Work)')).toBeInTheDocument()
	expect(posted).toEqual({
		channel: 'email',
		identifier: ' Maria@Example.COM ',
		displayName: 'Work',
	})
	expect(screen.getByLabelText('Value')).toHaveValue('')
	expect(screen.getByText('Identity added.')).toBeInTheDocument()
})

test('adds a phone identity through the channel select', async () => {
	const identities: IdentityRow[] = []
	let posted: Record<string, unknown> | null = null
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: { contact: detailFor(String(variables.id), 'Ana García', identities) },
			}),
		),
		graphql.mutation('AddContactIdentity', ({ variables }) => {
			posted = variables.identity as Record<string, unknown>
			const created = { id: identityID2, channel: 'phone', identifier: '+184467235', display_name: '' }
			identities.push(created)
			return HttpResponse.json({
				data: {
					addContactIdentity: {
						__typename: 'ContactIdentity',
						id: created.id,
						channel: created.channel,
						identifier: created.identifier,
						displayName: created.display_name,
					},
				},
			})
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(screen.getByLabelText('Channel'))
	await userEvent.click(await screen.findByRole('option', { name: 'Phone' }))
	await userEvent.type(screen.getByLabelText('Value'), '+184 467 235')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	expect(await screen.findByText('Phone: +184467235')).toBeInTheDocument()
	expect(posted).toMatchObject({ channel: 'phone', identifier: '+184 467 235' })
})

test('marks the channel the reader chose as the selected option', async () => {
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({ data: { contact: detailFor(String(variables.id), 'Ana García', []) } }),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(screen.getByLabelText('Channel'))

	expect(await screen.findByRole('option', { name: 'Email' })).toHaveAttribute(
		'aria-selected',
		'true',
	)
})

test('names the owner when the identity belongs to someone else', async () => {
	server.use(graphql.mutation('AddContactIdentity', () => HttpResponse.json(heldByBruno)))

	await addEmailIdentity()

	const value = screen.getByLabelText('Value')
	await screen.findByText('Already on contact Bruno.')
	expect(value).toHaveAccessibleDescription('Already on contact Bruno.')
	expect(value).toHaveFocus()
	expect(document.querySelector('.godmin-toast')).toBeNull()
})

test('takes the message about an identity away when the reader picks another channel', async () => {
	let adds = 0
	server.use(
		graphql.mutation('AddContactIdentity', () => {
			adds += 1
			return HttpResponse.json(heldByBruno)
		}),
	)
	await addEmailIdentity()
	await screen.findByText('Already on contact Bruno.')

	await userEvent.click(screen.getByRole('combobox', { name: 'Channel' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Phone' }))

	await waitFor(() => expect(screen.queryByText('Already on contact Bruno.')).not.toBeInTheDocument())
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))
	await waitFor(() => expect(adds).toBe(2))
})

test('puts the answer about a value in a toast when the reader picked another channel before it came', async () => {
	let answer = () => {}
	const answered = new Promise<void>((resolve) => {
		answer = resolve
	})
	server.use(
		graphql.mutation('AddContactIdentity', async () => {
			await answered
			return HttpResponse.json(heldByBruno)
		}),
	)
	await addEmailIdentity()

	await userEvent.click(screen.getByRole('combobox', { name: 'Channel' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Phone' }))
	answer()

	const toast = await screen.findByText('Could not add the identity. Already on contact Bruno.', {
		selector: '.godmin-toast',
	})
	expect(toast).toBeInTheDocument()
	expect(screen.getByLabelText('Value')).toBeValid()
})

test.each([
	['identity_channel_required', 'contact: empty channel'],
	['channel_not_writable', 'contact: channel not writable'],
])('says in a toast that the identity could not be added for the channel reason %s', async (reason, words) => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: words, extensions: { code: 'VALIDATION', reason } }],
			}),
		),
	)

	await addEmailIdentity()

	const toast = await screen.findByText(`Could not add the identity. ${words}`, { selector: '.godmin-toast' })
	expect(toast).toBeInTheDocument()
	expect(screen.getByLabelText('Value')).toBeValid()
})

test('shows the backend message when the identity is invalid', async () => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'contact: empty identifier', extensions: { code: 'VALIDATION' } }],
			}),
		),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), 'abc')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	await screen.findByText('contact: empty identifier')
	expect(screen.getByLabelText('Value')).toHaveAccessibleDescription('contact: empty identifier')

	await userEvent.type(screen.getByLabelText('Value'), 'd')

	await waitFor(() => expect(screen.queryByText('contact: empty identifier')).not.toBeInTheDocument())
})

test('falls back to the backend message when the conflict names no owner', async () => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({
				data: null,
				errors: [{
					message: 'contact: identity already exists',
					extensions: { code: 'CONFLICT', ownerContactId: brunoID },
				}],
			}),
		),
	)

	await addEmailIdentity()

	await screen.findByText('contact: identity already exists')
	expect(screen.getByLabelText('Value')).toHaveAccessibleDescription('contact: identity already exists')
})

test('says in a toast that the identity could not be added when the failure is about no field', async () => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	await addEmailIdentity()

	expect(await screen.findByText('Could not add the identity.', { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(screen.queryByText('Identity added.')).not.toBeInTheDocument()
	expect(screen.getByLabelText('Value')).toHaveValue('maria@example.com')
	await waitFor(() =>
		expect(screen.getByRole('button', { name: 'Add identity' })).not.toHaveAttribute('aria-disabled', 'true'),
	)
})

test('lays the channel, value, label and Add identity on one form row', async () => {
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	const add = screen.getByRole('button', { name: 'Add identity' })
	const cells = formRowChildren('Add identity')
	expect(cells).toHaveLength(4)
	expectFieldIn(cells[0], 'Channel')
	expectFieldIn(cells[1], 'Value')
	expectFieldIn(cells[2], 'Label')
	expect(cells[3]).toBe(add)
})

test('drops the session when the identity add is unauthorized', async () => {
	server.use(
		graphql.mutation('AddContactIdentity', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.type(screen.getByLabelText('Value'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Add identity' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('drops the session when the identity removal is unauthorized', async () => {
	server.use(
		graphql.mutation('DeleteContactIdentity', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(within(await askToRemove('184467235')).getByRole('button', { name: 'Remove' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('asks before it removes an identity, in a red question that says it cannot be undone', async () => {
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	const question = await askToRemove('184467235')

	expect(question).toHaveAccessibleDescription('This action cannot be undone.')
	expect(within(question).getByRole('button', { name: 'Remove' }).className).toMatch(/irreversible/)
	expect(within(question).getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
})

test('keeps the identity and sends nothing when the reader cancels its removal', async () => {
	let removals = 0
	server.use(
		graphql.mutation('DeleteContactIdentity', () => {
			removals += 1
			return HttpResponse.json({ data: { deleteContactIdentity: true } })
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(within(await askToRemove('184467235')).getByRole('button', { name: 'Cancel' }))

	await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
	expect(screen.getByText('WhatsApp: 184467235 (Ana G)')).toBeInTheDocument()
	expect(removals).toBe(0)
})

test('removes an identity once the reader confirms', async () => {
	let identities = [
		{ id: identityID1, channel: 'whatsapp', identifier: '184467235', display_name: 'Ana G' },
		{ id: identityID2, channel: 'email', identifier: 'maria@example.com', display_name: '' },
	]
	let deleted = ''
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: { contact: detailFor(String(variables.id), 'Ana García', identities) },
			}),
		),
		graphql.mutation('DeleteContactIdentity', ({ variables }) => {
			deleted = String(variables.identityId)
			identities = identities.filter((identity) => identity.id !== deleted)
			return HttpResponse.json({ data: { deleteContactIdentity: true } })
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	expect(screen.getByText('Email: maria@example.com')).toBeInTheDocument()

	await userEvent.click(within(await askToRemove('maria@example.com')).getByRole('button', { name: 'Remove' }))

	await waitFor(() =>
		expect(screen.queryByText('Email: maria@example.com')).not.toBeInTheDocument(),
	)
	await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
	expect(deleted).toBe(identityID2)
	expect(screen.getByText('WhatsApp: 184467235 (Ana G)')).toBeInTheDocument()
	expect(screen.getByText('Identity removed.')).toBeInTheDocument()
})

/**
 * Serves Ana García with the given identities, each removal taking its identity away.
 * @param identities - The identities the contact starts with.
 */
function serveRemovals(identities: IdentityRow[]) {
	let held = identities
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({ data: { contact: detailFor(String(variables.id), 'Ana García', held) } }),
		),
		graphql.mutation('DeleteContactIdentity', ({ variables }) => {
			held = held.filter((identity) => identity.id !== String(variables.identityId))
			return HttpResponse.json({ data: { deleteContactIdentity: true } })
		}),
	)
}

test.each([
	{ removed: 'maria@example.com', focused: 'Remove support@example.com', place: 'the next identity' },
	{ removed: 'support@example.com', focused: 'Remove maria@example.com', place: 'the one before when the last goes' },
])('puts the focus on the trash of $place once a removal works', async ({ removed, focused }) => {
	serveRemovals([
		{ id: identityID1, channel: 'whatsapp', identifier: '184467235', display_name: '' },
		{ id: identityID2, channel: 'email', identifier: 'maria@example.com', display_name: '' },
		{ id: identityID3, channel: 'email', identifier: 'support@example.com', display_name: '' },
	])
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(within(await askToRemove(removed)).getByRole('button', { name: 'Remove' }))

	await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
	await waitFor(() => expect(screen.getByRole('button', { name: focused })).toHaveFocus())
})

test('puts the focus on the first field of Add identity once the only identity goes', async () => {
	serveRemovals([{ id: identityID1, channel: 'email', identifier: 'maria@example.com', display_name: '' }])
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	await userEvent.click(within(await askToRemove('maria@example.com')).getByRole('button', { name: 'Remove' }))

	await screen.findByText('No identities yet.')
	await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
	await waitFor(() => expect(screen.getByLabelText('Channel')).toHaveFocus())
})

test('holds the question with a spinner on Remove until the removal ends', async () => {
	let answer = () => {}
	const answered = new Promise<void>((resolve) => {
		answer = resolve
	})
	server.use(
		graphql.mutation('DeleteContactIdentity', async () => {
			await answered
			return HttpResponse.json({ data: { deleteContactIdentity: true } })
		}),
	)
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })
	const question = await askToRemove('184467235')
	const remove = within(question).getByRole('button', { name: 'Remove' })

	await userEvent.click(remove)
	await waitFor(() => expect(remove).toHaveAttribute('aria-disabled', 'true'))
	expect([...remove.classList]).toEqual(expect.arrayContaining(busyClasses()))
	await userEvent.keyboard('{Escape}')
	expect(screen.getByRole('alertdialog', { name: 'Remove 184467235?' })).toBe(question)
	expect(screen.queryByText('Identity removed.')).not.toBeInTheDocument()
	answer()

	await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
	expect(screen.getByText('Identity removed.')).toBeInTheDocument()
})

test.each([
	{ answered: { message: 'internal error' }, words: 'Could not remove the identity.' },
	{
		answered: { message: 'identity not found', extensions: { code: 'NOT_FOUND', reason: 'identity_not_found' } },
		words: 'Could not remove the identity. That address is no longer on this contact.',
	},
])('says inside the question that an identity could not be removed: $words', async ({ answered, words }) => {
	configureAppErrorText()
	server.use(graphql.mutation('DeleteContactIdentity', () => HttpResponse.json({ data: null, errors: [answered] })))
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })
	const question = await askToRemove('184467235')
	const remove = within(question).getByRole('button', { name: 'Remove' })

	await userEvent.click(remove)

	expect(await within(question).findByText(words)).toBeInTheDocument()
	expect(screen.getByRole('alertdialog', { name: 'Remove 184467235?' })).toBe(question)
	expect(document.querySelector('.godmin-toast')).toBeNull()
	expect(screen.queryByText('Identity removed.')).not.toBeInTheDocument()
	await waitFor(() => expect(remove).not.toHaveAttribute('aria-disabled', 'true'))
})

/** shortLimit is the time limit in milliseconds a busy dialog waits on its write in a test that runs past it. */
const shortLimit = 50

test('stops a removal past the time limit and says inside the question that the server gave no answer', async () => {
	server.use(graphql.mutation('DeleteContactIdentity', () => new Promise(() => {})))
	const signals = signalsOf('DeleteContactIdentity')
	renderLimitedAt(`/contacts/${anaID}`, shortLimit)
	await screen.findByRole('heading', { name: 'Ana García' })
	const question = await askToRemove('184467235')
	const remove = within(question).getByRole('button', { name: 'Remove' })

	await userEvent.click(remove)

	expect(await within(question).findByText('Could not get a valid response from the server.')).toBeInTheDocument()
	expect(signals).toHaveLength(1)
	expect(signals[0]?.aborted).toBe(true)
	await waitFor(() => expect(remove).not.toHaveAttribute('aria-disabled', 'true'))
	expect(screen.getByText('WhatsApp: 184467235 (Ana G)')).toBeInTheDocument()
	expect(screen.queryByText('Could not remove the identity.')).not.toBeInTheDocument()
	expect(document.querySelector('.godmin-toast')).toBeNull()
})

test('shows each identity remove as an icon named after its identifier', async () => {
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	const remove = screen.getByRole('button', { name: 'Remove 184467235' })
	expect(remove.textContent).toBe('')
	expect(remove.querySelector('svg')).not.toBeNull()
})

test('lists each identity in the bordered log list with its trash at the row end', async () => {
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	const list = screen.getByRole('list', { name: 'Identities' })
	expect(list).toHaveClass('godmin-log-list')
	expect(within(list).getAllByRole('listitem')).toHaveLength(2)
	for (const [text, identifier] of [
		['WhatsApp: 184467235 (Ana G)', '184467235'],
		['WhatsApp: 184467236', '184467236'],
	]) {
		const item = within(list).getByRole('listitem', { name: text })
		const line = within(item).getByText(text)
		const remove = within(item).getByRole('button', { name: `Remove ${identifier}` })
		expect(line.closest('.godmin-log-list__label')).not.toBeNull()
		expect(remove.closest('.godmin-log-list__actions')).not.toBeNull()
		expect(line).not.toContainElement(remove)
		expect(line.closest('.godmin-log-list__header')).toContainElement(remove)
	}
})

test('fills the column with the add identity form', async () => {
	renderAt(`/contacts/${anaID}`)
	await screen.findByRole('heading', { name: 'Ana García' })

	expect(screen.getByRole('button', { name: 'Add identity' }).closest('form')).toHaveClass(
		'godmin-form',
		'godmin-form--inline',
	)
})
