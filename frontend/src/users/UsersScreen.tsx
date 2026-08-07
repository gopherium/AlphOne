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
import type { User } from '@gopherium/react-auth/admin'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

/**
 * Renders one account row with its status and disable control, which the
 * signed-in account does not get.
 * @returns The table row element.
 */
function UserRow({ user, isSelf }: { user: User; isSelf: boolean }) {
	const queryClient = useQueryClient()
	const toggle = useMutation({
		mutationFn: () => setUserDisabled(user.id, !user.disabled),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: usersQueryKey }),
	})

	return (
		<tr>
			<td>{user.name}</td>
			<td>{user.email}</td>
			<td>
				<Badge intent={user.disabled ? 'draft' : 'stable'}>
					{user.disabled ? 'Disabled' : 'Active'}
				</Badge>
			</td>
			<td className="godmin-table__actions">
				{isSelf ? null : (
					<Stack direction="column" gap="xs">
						<Button
							variant="outline"
							aria-label={`${user.disabled ? 'Enable' : 'Disable'} ${user.name}`}
							disabled={toggle.isPending}
							onClick={() => toggle.mutate()}
						>
							{user.disabled ? 'Enable' : 'Disable'}
						</Button>
						{toggle.isError ? <Text role="alert">Update failed.</Text> : null}
					</Stack>
				)}
			</td>
		</tr>
	)
}

/**
 * Renders the account list with its status and disable controls.
 * @returns The users screen.
 */
export function UsersScreen() {
	const currentUserId = useSession().data?.id
	const users = useQuery({
		queryKey: usersQueryKey,
		queryFn: ({ signal }) => fetchUsers(signal),
	})

	return (
		<PageScreen
			title="Users"
			actions={
				<Button variant="solid" render={<Link to="/users/new" />}>
					New user
				</Button>
			}
		>
			<UserRows users={users} currentUserId={currentUserId} />
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
}: {
	users: ReturnType<typeof useQuery<User[], Error>>
	currentUserId: string | undefined
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
		<div className="godmin-table-scroll" role="region" aria-label="Users" tabIndex={0}>
			<table className="godmin-table">
				<thead>
					<tr>
						<th scope="col">Name</th>
						<th scope="col">Email</th>
						<th scope="col">Status</th>
						<th scope="col" className="godmin-table__actions">
							<VisuallyHidden>Actions</VisuallyHidden>
						</th>
					</tr>
				</thead>
				<tbody>
					{users.data.map((user) => (
						<UserRow key={user.id} user={user} isSelf={user.id === currentUserId} />
					))}
				</tbody>
			</table>
		</div>
	)
}
