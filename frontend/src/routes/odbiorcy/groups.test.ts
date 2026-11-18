import { describe, expect, it } from 'vitest';
import type { Group } from '$lib/api/groups';
import { memberGroups } from './groups';

const group = (over: Partial<Group>): Group => ({
	id: 'g',
	name: 'Grupa',
	description: '',
	kind: 'custom',
	className: null,
	audience: null,
	memberCount: 0,
	createdAt: null,
	...over
});

const groups: Group[] = [
	group({ id: 'all-parents', name: 'Wszyscy rodzice', kind: 'system' }),
	group({
		id: 'p3a',
		name: 'Rodzice uczniów klasy 3A',
		kind: 'system',
		className: '3A',
		audience: 'parents'
	}),
	group({
		id: 's3a',
		name: 'Uczniowie klasy 3A',
		kind: 'system',
		className: '3A',
		audience: 'students'
	}),
	group({ id: 'rada', name: 'Rada rodziców' }),
	group({ id: 'chor', name: 'Chór' })
];

describe('memberGroups', () => {
	it('lists custom groups and the matching class groups by name', () => {
		const got = memberGroups({ type: 'parent', classes: ['3A'], groupIds: ['rada'] }, groups);
		expect(got?.map((g) => g.id)).toEqual(['rada', 'p3a']);
	});

	it('picks the student class group for a student', () => {
		const got = memberGroups({ type: 'student', classes: ['3A'], groupIds: [] }, groups);
		expect(got?.map((g) => g.id)).toEqual(['s3a']);
	});

	it('is null without the group list', () => {
		expect(memberGroups({ type: 'parent', classes: [], groupIds: [] }, null)).toBeNull();
	});
});
