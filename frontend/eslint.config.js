import js from '@eslint/js';
import svelte from 'eslint-plugin-svelte';
import ts from 'typescript-eslint';

export default [
	js.configs.recommended,
	...ts.configs.recommended,
	...svelte.configs['flat/recommended'],
	{
		languageOptions: { parserOptions: { extraFileExtensions: ['.svelte'] } }
	},
	{
		files: ['**/*.svelte'],
		languageOptions: { parserOptions: { parser: ts.parser } },
		// svelte-check (TypeScript) already rejects undefined names, and no-undef
		// does not know browser globals such as File; typescript-eslint turns it
		// off for .ts files for the same reason.
		rules: { 'no-undef': 'off' }
	},
	{ ignores: ['build/', '.svelte-kit/', 'node_modules/'] }
];
