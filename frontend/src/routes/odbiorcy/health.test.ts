import { describe, expect, it } from 'vitest';
import type { Recipient } from '$lib/api/recipients';
import {
	coverage,
	filterFixItems,
	isTruncated,
	mergeFixItems,
	parseFixView,
	peopleLabel,
	unreachableCount
} from './health';

function person(
	id: string,
	lastName: string,
	firstName = 'Jan',
	issues: Recipient['issues'] = []
): Recipient {
	return {
		id,
		firstName,
		lastName,
		email: null,
		phone: null,
		type: 'parent',
		classes: [],
		issues,
		createdAt: '2026-10-01T10:00:00Z',
		updatedAt: '2026-10-01T10:00:00Z'
	};
}

function rowsOf(counts: Parameters<typeof coverage>[0]) {
	const [email, sms] = coverage(counts);
	if (!email || !sms) throw new Error('coverage must return an e-mail and an SMS row');
	return { email, sms };
}

describe('coverage', () => {
	it('reports reachable people and percent per channel', () => {
		const { email, sms } = rowsOf({
			total: 200,
			parents: 120,
			students: 80,
			noEmail: 10,
			noPhone: 50
		});
		expect(email).toMatchObject({ channel: 'email', reachable: 190, missing: 10, percent: 95 });
		expect(sms).toMatchObject({ channel: 'sms', reachable: 150, missing: 50, percent: 75 });
		expect(email.href).toBe('/odbiorcy?issue=email');
		expect(sms.href).toBe('/odbiorcy?issue=sms');
	});

	it('treats an empty base as fully covered', () => {
		const rows = coverage({ total: 0, parents: 0, students: 0, noEmail: 0, noPhone: 0 });
		expect(rows.map((r) => r.percent)).toEqual([100, 100]);
	});

	it('clamps a count above the total', () => {
		const { email } = rowsOf({ total: 3, parents: 3, students: 0, noEmail: 9, noPhone: 0 });
		expect(email).toMatchObject({ reachable: 0, missing: 3, percent: 0 });
	});

	it('does not round a nearly full channel up to 100', () => {
		const { email, sms } = rowsOf({
			total: 1000,
			parents: 500,
			students: 500,
			noEmail: 1,
			noPhone: 0
		});
		expect(email).toMatchObject({ percent: 99, missing: 1 });
		expect(sms.percent).toBe(100);
	});
});

describe('mergeFixItems', () => {
	it('lists a person with both problems once and first', () => {
		const items = mergeFixItems(
			[person('a', 'Nowak'), person('b', 'Kowalski')],
			[person('b', 'Kowalski'), person('c', 'Adamczyk')]
		);
		expect(items.map((i) => i.recipient.id)).toEqual(['b', 'c', 'a']);
		expect(items.map((i) => i.channels)).toEqual([['email', 'sms'], ['sms'], ['email']]);
	});

	it('sorts with Polish collation inside a group', () => {
		const names = ['Żak', 'Zalewski', 'Łoś', 'Lis', 'Zieliński'];
		const items = mergeFixItems(
			names.map((n, i) => person(String(i), n)),
			[]
		);
		expect(items.map((i) => i.recipient.lastName)).toEqual([
			'Lis',
			'Łoś',
			'Zalewski',
			'Zieliński',
			'Żak'
		]);
	});

	it('takes the channels from the person, so a cut-off list cannot hide a missing one', () => {
		const both: Recipient['issues'] = [
			{ channel: 'email', reason: 'missing' },
			{ channel: 'sms', reason: 'missing' }
		];
		// Found only in the "no e-mail" list, but has no phone either.
		const items = mergeFixItems([person('a', 'Nowak', 'Jan', both)], []);
		expect(items.map((i) => i.channels)).toEqual([['email', 'sms']]);
		expect(unreachableCount(items)).toBe(1);
		expect(filterFixItems(items, 'both')).toHaveLength(1);
	});

	it('returns nothing for two empty lists', () => {
		expect(mergeFixItems([], [])).toEqual([]);
	});

	it('does not repeat a channel when the same person is listed twice in one list', () => {
		const items = mergeFixItems([person('a', 'Nowak'), person('a', 'Nowak')], []);
		expect(items.map((i) => i.channels)).toEqual([['email']]);
	});
});

describe('unreachableCount', () => {
	it('counts people with both channels broken', () => {
		const items = mergeFixItems(
			[person('a', 'A'), person('b', 'B')],
			[person('b', 'B'), person('c', 'C')]
		);
		expect(unreachableCount(items)).toBe(1);
	});
});

describe('isTruncated', () => {
	it.each([
		[200, 200, false],
		[200, 201, true],
		[0, 0, false]
	])('shown %i of %i -> %s', (shown, total, want) => {
		expect(isTruncated(shown, total)).toBe(want);
	});
});

describe('peopleLabel', () => {
	it.each([
		[0, '0 osób'],
		[1, '1 osoba'],
		[2, '2 osoby'],
		[4, '4 osoby'],
		[5, '5 osób'],
		[12, '12 osób'],
		[14, '14 osób'],
		[22, '22 osoby'],
		[112, '112 osób'],
		[1000, '1000 osób']
	])('%i -> %s', (n, want) => {
		expect(peopleLabel(n)).toBe(want);
	});
});

describe('parseFixView', () => {
	it.each([
		['email', 'email'],
		['sms', 'sms'],
		['both', 'both'],
		['x', 'all'],
		['', 'all'],
		[null, 'all']
	])('%s -> %s', (raw, want) => {
		expect(parseFixView(raw)).toBe(want);
	});
});

describe('filterFixItems', () => {
	const items = mergeFixItems(
		[person('a', 'A'), person('b', 'B')],
		[person('b', 'B'), person('c', 'C')]
	);

	it.each([
		['all', ['b', 'a', 'c']],
		['email', ['b', 'a']],
		['sms', ['b', 'c']],
		['both', ['b']]
	] as const)('%s', (view, ids) => {
		expect(filterFixItems(items, view).map((i) => i.recipient.id)).toEqual(ids);
	});
});
