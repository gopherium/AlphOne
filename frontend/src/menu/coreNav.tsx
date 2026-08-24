// SPDX-License-Identifier: AGPL-3.0-or-later

import type { NavItem } from '@alphone/frontend-sdk'
import { usersNavItem } from '@gopherium/react-auth/wpds'

import { contactsNavItem } from '../contacts/nav'
import { languageNavItem } from '../i18n/nav'
import { tasksNavItem } from '../tasks/nav'

export const coreNav: NavItem[] = [tasksNavItem, contactsNavItem, usersNavItem, languageNavItem]
