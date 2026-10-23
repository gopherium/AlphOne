// SPDX-License-Identifier: AGPL-3.0-or-later

import { existsSync, globSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'

import ts from 'typescript'
import { expect, onTestFinished, test } from 'vitest'

import { domains, repositoryRoot } from '../../scripts/config.ts'

const ROOT = repositoryRoot()

/** SPOKEN lists the attributes whose value a reader reads or hears. */
const SPOKEN = new Set(['aria-label', 'aria-description', 'alt', 'label', 'placeholder', 'title'])

/** WORDED matches a run carrying a word rather than punctuation or a glyph. */
const WORDED = /[A-Za-z]{2,}/

/** brandNames reads the brand names every plugin under root lists in its testdata, one per line. */
function brandNames(root: string): string[] {
	return globSync('plugins/*/testdata/brandnames.txt', { cwd: root })
		.sort()
		.flatMap((path) => readFileSync(join(root, path), 'utf8').split('\n'))
		.map((line) => line.trim())
		.filter((name) => name !== '')
}

/** NAMED lists the names that read the same in every language. */
const NAMED = new Set(['AlphOne', ...brandNames(ROOT)])

/** UNREAD lists the sources that furnish tests rather than the interface. */
const UNREAD = ['/test/', '/gql/', 'sdk/frontend/testing.tsx']

/** Bare names one string a reader meets that no catalogue can answer for. */
interface Bare {
	file: string
	line: number
	text: string
}

/**
 * Returns every source file the given globs reach, tests and generated code aside.
 * @param globs - The globs a domain extracts from.
 * @returns The absolute paths, in walk order.
 */
function sourceFiles(globs: string[]): string[] {
	const found: string[] = []
	for (const glob of globs) {
		walk(join(ROOT, glob.slice(0, glob.indexOf('*')).replace(/\/$/, '')), found)
	}
	return found.filter((file) => !UNREAD.some((held) => file.includes(held)))
}

/**
 * Collects every TypeScript source sitting under one directory.
 * @param directory - The directory to read.
 * @param found - The paths collected so far.
 */
function walk(directory: string, found: string[]): void {
	if (!existsSync(directory)) {
		return
	}
	for (const entry of readdirSync(directory, { withFileTypes: true })) {
		const path = join(directory, entry.name)
		if (entry.isDirectory()) {
			walk(path, found)
		} else if (path.endsWith('.ts') || path.endsWith('.tsx')) {
			found.push(path)
		}
	}
}

/**
 * Returns the name one JSX attribute carries.
 * @param node - The attribute to read.
 * @returns The name as written.
 */
function attributeName(node: ts.JsxAttribute): string {
	return ts.isIdentifier(node.name) ? node.name.text : node.name.getText()
}

/**
 * Returns every string a reader meets in one file that reaches no gettext call.
 * @param file - The absolute path to read.
 * @returns The bare strings, each with the line it sits on.
 */
function bareStringsIn(file: string): Bare[] {
	const source = ts.createSourceFile(file, readFileSync(file, 'utf8'), ts.ScriptTarget.Latest, true)
	const found: Bare[] = []
	const held = (node: ts.Node, text: string): void => {
		if (NAMED.has(text.trim())) {
			return
		}
		found.push({
			file: file.slice(ROOT.length + 1),
			line: source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
			text: text.trim(),
		})
	}
	const visit = (node: ts.Node): void => {
		if (ts.isJsxText(node) && WORDED.test(node.text)) {
			held(node, node.text)
		}
		if (ts.isJsxAttribute(node) && SPOKEN.has(attributeName(node))) {
			const value = node.initializer
			if (value !== undefined && ts.isStringLiteral(value) && WORDED.test(value.text)) {
				held(node, value.text)
			}
		}
		if (ts.isJsxExpression(node) && node.expression !== undefined) {
			const held0 = node.expression
			if (ts.isStringLiteral(held0) && WORDED.test(held0.text)) {
				held(node, held0.text)
			}
		}
		ts.forEachChild(node, visit)
	}
	visit(source)
	return found
}

/** writeTree writes a temporary repository holding the given files, keyed by path. */
function writeTree(files: Record<string, string>): string {
	const root = mkdtempSync(join(tmpdir(), 'alphone-brands-'))
	onTestFinished(() => rmSync(root, { recursive: true, force: true }))
	for (const [path, content] of Object.entries(files)) {
		mkdirSync(dirname(join(root, path)), { recursive: true })
		writeFileSync(join(root, path), content)
	}
	return root
}

test('reads the brand names each plugin lists in its testdata', () => {
	const root = writeTree({
		'plugins/demo/testdata/brandnames.txt': '\nAcme\n\nAcme Cloud\n',
		'plugins/quiet/plugin.json': '{"id": "quiet"}',
		'enterprise/extra/testdata/brandnames.txt': 'Extra\n',
	})

	expect(brandNames(root)).toEqual(['Acme', 'Acme Cloud'])
})

test('leaves no string a reader meets outside a catalogue', () => {
	const bare: Bare[] = []
	for (const domain of domains()) {
		for (const file of sourceFiles(domain.sources)) {
			bare.push(...bareStringsIn(file))
		}
	}

	expect(bare.map((held) => `${held.file}:${held.line} ${JSON.stringify(held.text)}`)).toEqual([])
})
