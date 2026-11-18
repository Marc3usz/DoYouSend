import { describe, expect, it } from 'vitest';
import type { RecipientLookup } from '$lib/api/recipients';
import {
	collectMatching,
	emptyFilters,
	pageSelection,
	pickerPageCount,
	pickerQuery
} from './recipientPicker';

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

describe('collectMatching', () => {
	const person = (n: number): RecipientLookup => ({
		id: `id-${n}`,
		firstName: 'Imię',
		lastName: `Nazwisko${n}`,
		type: 'parent',
		classes: [],
		issues: []
	});
	const base = (n: number) => Array.from({ length: n }, (_, i) => person(i));
	const served =
		(all: RecipientLookup[], seen: number[]) => async (q: { limit?: number; offset?: number }) => {
			seen.push(q.offset ?? 0);
			const offset = q.offset ?? 0;
			return { items: all.slice(offset, offset + (q.limit ?? 0)), total: all.length };
		};

	it('reads every page of the matches', async () => {
		const seen: number[] = [];
		const got = await collectMatching(served(base(450), seen), emptyFilters);
		expect(got.people).toHaveLength(450);
		expect(got.truncated).toBe(false);
		expect(seen).toEqual([0, 200, 400]);
	});

	it('stops at the cap and says so', async () => {
		const seen: number[] = [];
		const got = await collectMatching(served(base(1300), seen), emptyFilters, 200, 1000);
		expect(got.people).toHaveLength(1000);
		expect(got.truncated).toBe(true);
		expect(seen).toEqual([0, 200, 400, 600, 800]);
	});

	it('passes the filters on', async () => {
		const queries: unknown[] = [];
		await collectMatching(
			async (q) => {
				queries.push(q);
				return { items: [], total: 0 };
			},
			{ q: ' anna ', type: 'parent', klasa: '3 a' }
		);
		expect(queries).toEqual([{ q: 'anna', type: 'parent', class: '3A', limit: 200, offset: 0 }]);
	});

	it('keeps each person once when the base shifts between pages', async () => {
		const all = base(3);
		const shifted = async (q: { offset?: number }) =>
			q.offset === 0
				? { items: all.slice(0, 2), total: 3 }
				: { items: [all[1] as RecipientLookup, all[2] as RecipientLookup], total: 3 };
		const got = await collectMatching(shifted, emptyFilters, 2);
		expect(got.people.map((p) => p.id)).toEqual(['id-0', 'id-1', 'id-2']);
	});
});
