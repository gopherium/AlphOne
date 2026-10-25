---
title: Roles
description: Declare the roles your plugin needs, name the deployment's top role, and ask what a role can do.
---

Every account holds one role. A role carries capabilities, and a
capability is one thing the role can do, such as `manage_users`.
AlphOne ships two roles. Admin carries `manage_users` and
`manage_webhooks`, and member carries none. Your plugin can add roles
of its own, and give the roles AlphOne ships more capabilities.

This page uses one plugin, `archive`, that deletes archived rows, and
later a second one, `workspaces`, that runs the whole install.

## Declare your roles

A plugin declares roles by implementing `sdk.RoleProvider`, whose one
method returns them:

```go
var _ sdk.RoleProvider = (*Plugin)(nil)

// Roles returns the archivist role and the capability it gives admin.
func (p *Plugin) Roles() []sdk.RoleDeclaration {
	return []sdk.RoleDeclaration{
		{Name: "archivist", Capabilities: []string{"archive.purge"}},
		{Name: "admin", Capabilities: []string{"archive.purge"}},
	}
}
```

The `var _` line turns a missing or misspelled `Roles` method into a
build error. Without it AlphOne finds no roles and says nothing.

A new capability starts with the plugin id and a dot, so the archive
plugin names its own `archive.purge`. A role may also carry a capability
AlphOne ships, such as `manage_users`, or one that a plugin loaded
before yours declares. Each role and capability name starts with a
lower-case letter and is at most 64 characters long, all of them
lower-case letters, digits, dots, underscores or hyphens.

## Give admin a capability

Admin does not get a new capability by itself. To give it one, list
admin among your roles with that capability, the way `Roles` does above.
Every admin account can then purge.

A capability you give admin also changes who can manage admins. A role
can change another account's role only when it holds `manage_users` and
every capability that account's role holds. Once admin holds
`archive.purge`, a role without it can no longer change an admin's role
or make someone an admin.

## Name the top role

One plugin can name the deployment's top role, the role meant to manage
every account, admins included. The `workspaces` plugin declares an
`owner` role and names it by implementing `sdk.TopRoleProvider`:

```go
var _ sdk.TopRoleProvider = (*Plugin)(nil)

// TopRole names owner as the deployment's top role.
func (p *Plugin) TopRole() string {
	return "owner"
}
```

The top role then receives every capability the other plugins declare,
`archive.purge` included, whatever order the plugins load in. It must be
a role your plugin declares, and neither admin nor member.

AlphOne gives the top role only what the other plugins declare. It does
not give it the capabilities AlphOne ships or the ones your own plugin
declares, so list those on the role yourself:

```go
// Roles returns the owner role with the shipped capabilities and its own.
func (p *Plugin) Roles() []sdk.RoleDeclaration {
	return []sdk.RoleDeclaration{{Name: "owner", Capabilities: []string{
		"manage_users", "manage_webhooks", "workspaces.manage_workspaces",
	}}}
}
```

The top role must carry every capability admin carries, so it can
change any admin's role. AlphOne stops with an error when it does not,
for example when your plugin gives admin a capability and leaves it off
the top role.

When no plugin names a top role, nothing changes. No role receives a
capability its plugin did not give it.

## When a declaration is refused

AlphOne reads every plugin's roles when it loads the plugins, and stops
with an error when it cannot declare them:

| What the plugins declare | Error |
| --- | --- |
| A new capability without the plugin's prefix, such as `purge` | `plugin archive: goncierge: new capability outside the source's prefix "archive.": "purge"` |
| Two plugins name a top role | `plugins workspaces and teams: only one plugin may name the top role` |
| `TopRole` answers empty text | `plugin workspaces: the top role is empty` |
| `TopRole` answers a role its plugin does not declare | `plugin workspaces: the plugin does not declare its top role "owner"` |
| `TopRole` answers a role AlphOne ships, such as `admin` | `plugin workspaces: AlphOne ships the top role "admin"` |
| The top role leaves out a capability admin carries | `plugin workspaces: the top role lacks a capability admin carries: manage_webhooks` |

The error starts with `declare plugin roles:`. It stops `serve`,
`migrate`, `seed -yes`, `account:create-admin` and the account commands
that take `-as`, and `alphone check` names it.

## Ask what a role can do

Ask whether a role can do something by naming the capability, never by
checking the role's name. A check such as `role == "admin"` misses every
other role that holds the capability, the top role included. Each place
that guards an action takes a capability:

- A graph field names it in its `@scope`, such as
  `@scope(area: "archive", write: true, capability: "archive.purge")`.
  `go test ./graph/` fails on a field that uses `admin: true` instead.
- A command names it in `Capability`, see
  [An acting account](/extending/commands/#an-acting-account).
- A screen asks `can`, and a route names it in its `staticData`, see
  [Who is signed in](/extending/screens/#who-is-signed-in).
- A client reads the caller's `capabilities` from `me`, see
  [Roles](/reference/graphql-api/#roles) in the API reference.

## Check the declared roles

A plugin that changes accounts itself, such as one that invites people
or switches accounts back on, checks them against the roles the install
declares. Implement `sdk.RoleCheckerConsumer`, and AlphOne calls
`UseRoleChecker` once every plugin's roles are declared, after
`Register` and before `Start` or any of the plugin's commands runs:

```go
var _ sdk.RoleCheckerConsumer = (*Plugin)(nil)

// UseRoleChecker keeps the host's role checker for the plugin's account writes.
func (p *Plugin) UseRoleChecker(checker sdk.RoleChecker) {
	p.roles = checker
}
```

`Known(role)` reports whether AlphOne or a plugin declares the role, and
`Outranks(caller, target)` whether the caller's role carries every
capability the target's role carries. The checker only reads the roles,
so a plugin still declares its own through `sdk.RoleProvider`.

Hand the checker to authkit as `AdminConfig.Roles` or
`InvitesConfig.Roles`, and the plugin's account writes refuse a role
nobody declares, the same way the core's writes do.
`SetAccountRoleFrom` and `SetAccountDisabledFrom` also refuse an account
whose role changed after the check, and enabling an account whose role
nobody declares. Neither checks who may hand a role out, so ask
`Outranks` first when the write acts for an account.
[User administration](https://docs.gopherium.org/authentication/user-administration/#checking-roles-against-a-registry)
lists every case authkit refuses. With a nil `Roles` authkit checks none
of this, so refuse an account write that comes before `UseRoleChecker`.
