// SPDX-License-Identifier: AGPL-3.0-or-later

import { FileButton as godminFileButton } from '@gopherium/godmin'
import { upload as wordpressUpload } from '@wordpress/icons'
import { expect, test } from 'vitest'

import { FileButton, upload } from '../index'
import { buttonClasses } from '../testing'

test('hands plugins the file button and the upload icon an upload control draws', () => {
	expect(FileButton).toBe(godminFileButton)
	expect(upload).toBe(wordpressUpload)
})

test('samples a button in the tone a dialog body draws its Cancel in', () => {
	expect(buttonClasses('minimal', undefined, 'neutral')).not.toEqual(buttonClasses('minimal'))
})
