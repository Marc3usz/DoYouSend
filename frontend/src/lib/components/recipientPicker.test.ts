import { describe, expect, it } from 'vitest';
import type { RecipientLookup } from '$lib/api/recipients';
import { emptyFilters, pageSelection, pickerPageCount, pickerQuery } from './recipientPicker';

const person = (id: string): RecipientLookup => ({
	id,
	firstName: 'Anna',
	lastName: 'Testowa',
	type: 'parent',
	classes: [],
	issues: []
});

describe('pickerQuery', () => {
	it.each([
		{
			name: 'no filters, first page',
			f: emptyFilters,
			page: 1,
			want: { q: undefined, type: undefined, class: undefined, limit: 20, offset: 0 }
		},
		{
			name: 'trims the search and normalizes the class',
			f: { q: '  Kowal ', type: 'student' as const, klasa: '3 a' },
			page: 3,
			want: { q: 'Kowal', type: 'student', class: '3A', limit: 20, offset: 40 }
		},
		{
			name: 'a page below 1 is the first page',
			f: emptyFilters,
			page: 0,
			want: { q: undefined, type: undefined, class: undefined, limit: 20, offset: 0 }
		}
	])('$name', ({ f, page, want }) => {
		expect(pickerQuery(f, page, 20)).toEqual(want);
	});
});

describe('pickerPageCount', () => {
	it.each([
		[0, 1],
		[20, 1],
		[21, 2]
	])('total %i -> %i pages', (total, want) => {
		expect(pickerPageCount(total, 20)).toBe(want);
	});
});

describe('pageSelection', () => {
	const items = [person('a'), person('b'), person('c')];
	it.each([
		{ name: 'nothing selected', selected: [], locked: [], want: 'none' },
		{ name: 'part selected', selected: ['a'], locked: [], want: 'some' },
		{ name: 'all selected', selected: ['a', 'b', 'c'], locked: [], want: 'all' },
		{ name: 'locked rows do not count', selected: ['a', 'b'], locked: ['c'], want: 'all' },
		{ name: 'only locked rows', selected: [], locked: ['a', 'b', 'c'], want: 'none' }
	])('$name', ({ selected, locked, want }) => {
		expect(pageSelection(items, new Set(selected), new Set(locked))).toBe(want);
	});
});
