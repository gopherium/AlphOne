// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	close as wordpressClose,
	commentAuthorAvatar as wordpressCommentAuthorAvatar,
	commentReplyLink as wordpressCommentReplyLink,
	envelope as wordpressEnvelope,
	file as wordpressFile,
	formatBold as wordpressFormatBold,
	formatItalic as wordpressFormatItalic,
	formatListBullets as wordpressFormatListBullets,
	formatListNumbered as wordpressFormatListNumbered,
	inlineImage as wordpressInlineImage,
	link as wordpressLink,
	lock as wordpressLock,
	moveTo as wordpressMoveTo,
	quote as wordpressQuote,
} from '@wordpress/icons'
import { expect, test } from 'vitest'

import {
	close,
	commentAuthorAvatar,
	commentReplyLink,
	envelope,
	file,
	formatBold,
	formatItalic,
	formatListBullets,
	formatListNumbered,
	inlineImage,
	link,
	lock,
	moveTo,
	quote,
} from '../index'

test('hands plugins the move and close icons a bulk action draws', () => {
	expect(moveTo).toBe(wordpressMoveTo)
	expect(close).toBe(wordpressClose)
})

test('hands plugins the single person icon a link to one contact draws', () => {
	expect(commentAuthorAvatar).toBe(wordpressCommentAuthorAvatar)
})

test('hands plugins the mail, reply, encrypted and attachment icons a mail thread draws', () => {
	expect(envelope).toBe(wordpressEnvelope)
	expect(commentReplyLink).toBe(wordpressCommentReplyLink)
	expect(lock).toBe(wordpressLock)
	expect(file).toBe(wordpressFile)
})

test('hands plugins the bold, italic, link, list, quote and inline image icons a composer toolbar draws', () => {
	expect(formatBold).toBe(wordpressFormatBold)
	expect(formatItalic).toBe(wordpressFormatItalic)
	expect(link).toBe(wordpressLink)
	expect(formatListBullets).toBe(wordpressFormatListBullets)
	expect(formatListNumbered).toBe(wordpressFormatListNumbered)
	expect(quote).toBe(wordpressQuote)
	expect(inlineImage).toBe(wordpressInlineImage)
})
