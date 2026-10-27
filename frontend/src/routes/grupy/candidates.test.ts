import { describe, expect, it } from 'vitest';
import type { Recipient } from '$lib/api/recipients';
import { CANDIDATE_LIMIT, pickCandidates, searchLimit } from './candidates';

function person(id: string): Recipient {
	return {
		id,
		firstName: 'Jan',
		lastName: `Test${id}`,
		email: null,
		phone: null,
		type: 'parent',
		classes: [],
		issues: [],
		createdAt: '2026-10-01T10:00:00Z',
		updatedAt: '2026-10-01T10:00:00Z'
	};
}

const people = (n: number) => Array.from({ length: n }, (_, i) => person(String(i)));

describe('searchLimit', () => {
	it.each([
		[0, CANDIDATE_LIMIT],
		[5, CANDIDATE_LIMIT + 5],
		[500, 200],
		[-3, CANDIDATE_LIMIT]
	])('%i members -> %i', (members, want) => {
		expect(searchLimit(members)).toBe(want);
	});
});

describe('pickCandidates', () => {
	it('drops members and reports how many were dropped', () => {
		const got = pickCandidates(people(5), 5, new Set(['1', '3']));
		expect(got.candidates.map((r) => r.id)).toEqual(['0', '2', '4']);
		expect(got.alreadyMembers).toBe(2);
		expect(got.moreCandidates).toBe(false);
	});

	it('still offers people when the first matches are all members', () => {
		const members = new Set(people(20).map((r) => r.id));
		const got = pickCandidates(people(25), 25, members);
		expect(got.candidates.map((r) => r.id)).toEqual(['20', '21', '22', '23', '24']);
		expect(got.alreadyMembers).toBe(20);
	});

	it('caps the offer and says there is more', () => {
		const got = pickCandidates(people(30), 30, new Set());
		expect(got.candidates).toHaveLength(CANDIDATE_LIMIT);
		expect(got.moreCandidates).toBe(true);
	});

	it('says there is more when the API cut the result off', () => {
		const got = pickCandidates(people(3), 50, new Set());
		expect(got.moreCandidates).toBe(true);
	});

	it('does not claim everyone is a member when the result was cut off', () => {
		const members = new Set(people(200).map((r) => r.id));
		const got = pickCandidates(people(200), 1000, members);
		expect(got.candidates).toEqual([]);
		expect(got.moreCandidates).toBe(true);
	});

	it('reports nothing for an empty result', () => {
		expect(pickCandidates([], 0, new Set())).toEqual({
			candidates: [],
			moreCandidates: false,
			alreadyMembers: 0
		});
	});
});
