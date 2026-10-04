// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	close as wordpressClose,
	commentAuthorAvatar as wordpressCommentAuthorAvatar,
	moveTo as wordpressMoveTo,
} from '@wordpress/icons'
import { expect, test } from 'vitest'

import { close, commentAuthorAvatar, moveTo } from '../index'

test('hands plugins the move and close icons a bulk action draws', () => {
	expect(moveTo).toBe(wordpressMoveTo)
	expect(close).toBe(wordpressClose)
})

test('hands plugins the single person icon a link to one contact draws', () => {
	expect(commentAuthorAvatar).toBe(wordpressCommentAuthorAvatar)
})
