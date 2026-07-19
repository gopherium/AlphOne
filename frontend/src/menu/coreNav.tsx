// SPDX-License-Identifier: AGPL-3.0-or-later

import type { NavItem } from '@alphone/frontend-sdk'
import { usersNavItem } from '@gopherium/react-auth/wpds'

import { contactsNavItem } from '../contacts/nav'

export const coreNav: NavItem[] = [contactsNavItem, usersNavItem]
