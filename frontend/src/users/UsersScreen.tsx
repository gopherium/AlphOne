// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Badge,
	Button,
	EmptyState,
	ErrorNotice,
	LoadingRows,
	PageScreen,
	Stack,
	Text,
	VisuallyHidden,
	people,
} from '@alphone/frontend-sdk'
import { useSession } from '@gopherium/react-auth'
import { fetchUsers, setUserDisabled, usersQueryKey } from '@gopherium/react-auth/admin'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import type { Account } from '../auth/graphTransport'
import { setUserRole } from '../auth/graphTransport'
import { useRole } from '../auth/role'

/**
 * Renders one account row with its status and tier, offering the controls only
 * to an admin looking at somebody else.
 * @param user - The account the row shows.
 * @param isSelf - Whether the row is the signed-in account, which gets no controls.
 * @param manages - Whether the caller may manage users at all.
 * @returns The table row element.
 */
function UserRow({ user, isSelf, manages }: { user: Account; isSelf: boolean; manages: boolean }) {
	return (
		<tr>
			<td>{user.name}</td>
			<td>{user.email}</td>
			<td>
				<Badge intent={user.disabled ? 'draft' : 'stable'}>
					{user.disabled ? 'Disabled' : 'Active'}
				</Badge>
			</td>
			<td>{user.role === 'admin' ? 'Admin' : 'Member'}</td>
			<td className="godmin-table__actions">
				{isSelf || !manages ? null : <UserControls user={user} />}
			</td>
		</tr>
	)
}

/**
 * Renders the disable and tier controls one account offers an admin.
 * @param user - The account the controls act on.
 * @returns The control stack element.
 */
function UserControls({ user }: { user: Account }) {
	const queryClient = useQueryClient()
	const invalidate = { onSuccess: () => queryClient.invalidateQueries({ queryKey: usersQueryKey }) }
	const barred = user.disabled
	const stands = user.role === 'admin'
	const toggle = useMutation({
		mutationFn: () => setUserDisabled(user.id, !barred),
		...invalidate,
	})
	const restand = useMutation({
		mutationFn: () => setUserRole(user.id, stands ? 'member' : 'admin'),
		...invalidate,
	})
	const refused = toggle.error ?? restand.error

	return (
		<Stack direction="column" gap="xs">
			<Button
				variant="outline"
				aria-label={`${barred ? 'Enable' : 'Disable'} ${user.name}`}
				loading={toggle.isPending}
				onClick={() => toggle.mutate()}
			>
				{barred ? 'Enable' : 'Disable'}
			</Button>
			<Button
				variant="outline"
				aria-label={`${stands ? 'Demote' : 'Promote'} ${user.name}`}
				loading={restand.isPending}
				onClick={() => restand.mutate()}
			>
				{stands ? 'Demote' : 'Promote'}
			</Button>
			{refused ? <Text role="alert">{refused.message}</Text> : null}
		</Stack>
	)
}

/**
 * Renders the account list with its status and disable controls.
 * @returns The users screen.
 */
export function UsersScreen() {
	const currentUserId = useSession().data?.id
	const manages = useRole() === 'admin'
	const users = useQuery({
		queryKey: usersQueryKey,
		queryFn: ({ signal }) => fetchUsers(signal) as Promise<Account[]>,
	})

	return (
		<PageScreen
			title="Users"
			actions={
				<Stack direction="row" gap="sm">
					<Button variant="outline" render={<Link to="/users/tokens" />}>
						API tokens
					</Button>
					{manages ? (
						<Button variant="solid" render={<Link to="/users/new" />}>
							New user
						</Button>
					) : null}
				</Stack>
			}
		>
			<UserRows users={users} currentUserId={currentUserId} manages={manages} />
		</PageScreen>
	)
}

/**
 * Renders the loading, error, empty, and loaded states of the account list.
 * @returns The list body.
 */
function UserRows({
	users,
	currentUserId,
	manages,
}: {
	users: ReturnType<typeof useQuery<Account[], Error>>
	currentUserId: string | undefined
	manages: boolean
}) {
	if (users.isPending) {
		return <LoadingRows label="Loading users…" />
	}
	if (users.isError) {
		return <ErrorNotice>Users could not be loaded.</ErrorNotice>
	}
	if (users.data.length === 0) {
		return (
			<EmptyState.Root className="godmin-empty">
				<EmptyState.Icon icon={people} />
				<EmptyState.Title>No users yet.</EmptyState.Title>
				<EmptyState.Description>Add one with New user.</EmptyState.Description>
			</EmptyState.Root>
		)
	}
	return (
		<div
			className="godmin-table-scroll godmin-arrival"
			role="region"
			aria-label="Users"
			tabIndex={0}
		>
			<table className="godmin-table">
				<thead>
					<tr>
						<th scope="col">Name</th>
						<th scope="col">Email</th>
						<th scope="col">Status</th>
						<th scope="col">Role</th>
						<th scope="col" className="godmin-table__actions">
							<VisuallyHidden>Actions</VisuallyHidden>
						</th>
					</tr>
				</thead>
				<tbody>
					{users.data.map((user) => (
						<UserRow
							key={user.id}
							user={user}
							isSelf={user.id === currentUserId}
							manages={manages}
						/>
					))}
				</tbody>
			</table>
		</div>
	)
}
