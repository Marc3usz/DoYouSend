import { describe, expect, it } from 'vitest';
import { nextTheme, parseTheme } from './theme';

describe('parseTheme', () => {
	it.each([
		['light', 'light'],
		['dark', 'dark'],
		['system', 'system'],
		['purple', 'system'],
		['', 'system'],
		[null, 'system']
	] as const)('%s -> %s', (input, want) => {
		expect(parseTheme(input)).toBe(want);
	});
});

describe('nextTheme', () => {
	it('cycles system -> light -> dark -> system', () => {
		expect(nextTheme('system')).toBe('light');
		expect(nextTheme('light')).toBe('dark');
		expect(nextTheme('dark')).toBe('system');
	});
});
