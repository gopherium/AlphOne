import jsdoc from 'eslint-plugin-jsdoc'
import tsdoc from 'eslint-plugin-tsdoc'
import tseslint from 'typescript-eslint'

export default [
	{
		ignores: ['**/coverage/**', '**/dist/**', '**/node_modules/**'],
	},
	{
		files: [
			'frontend/src/**/*.{ts,tsx}',
			'sdk/frontend/**/*.{ts,tsx}',
			'plugins/*/frontend/**/*.{ts,tsx}',
		],
		ignores: ['frontend/src/plugins/index.ts'],
		languageOptions: {
			parser: tseslint.parser,
			parserOptions: { ecmaFeatures: { jsx: true } },
		},
		rules: {
			'max-len': ['error', { code: 120, tabWidth: 1, ignoreUrls: true }],
		},
	},
	{
		files: [
			'frontend/src/**/*.{ts,tsx}',
			'sdk/frontend/**/*.{ts,tsx}',
			'plugins/*/frontend/**/*.{ts,tsx}',
		],
		ignores: [
			'**/*.test.{ts,tsx}',
			'frontend/src/test/**',
			'frontend/src/main.tsx',
			'frontend/src/plugins/index.ts',
		],
		languageOptions: {
			parser: tseslint.parser,
			parserOptions: { ecmaFeatures: { jsx: true } },
		},
		settings: { jsdoc: { mode: 'typescript' } },
		plugins: { jsdoc, tsdoc },
		rules: {
			'tsdoc/syntax': 'error',
			'jsdoc/require-jsdoc': [
				'error',
				{
					require: { FunctionDeclaration: true, MethodDefinition: true },
					exemptEmptyConstructors: true,
				},
			],
			'jsdoc/require-param': [
				'error',
				{ checkDestructured: false, checkDestructuredRoots: false },
			],
			'jsdoc/require-param-description': 'error',
			'jsdoc/require-returns': 'error',
			'jsdoc/require-returns-description': 'error',
		},
	},
]
