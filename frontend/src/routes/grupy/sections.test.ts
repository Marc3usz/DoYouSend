import { describe, expect, it } from 'vitest';
import type { Group } from '$lib/api/groups';
import { groupSections } from './sections';

const g = (over: Partial<Group>): Group => ({
	id: over.name ?? 'x',
	name: 'x',
	description: '',
	kind: 'system',
	className: null,
	audience: null,
	memberCount: 0,
	createdAt: null,
	...over
});

describe('groupSections', () => {
	it('splits fixed, class and custom groups, one row per class', () => {
		const s = groupSections([
			g({ name: 'Wszyscy rodzice' }),
			g({ name: 'Uczniowie klasy 1B', className: '1B', audience: 'students' }),
			g({ name: 'Rodzice uczniów klasy 1B', className: '1B', audience: 'parents' }),
			g({ name: 'Uczniowie klasy 3A', className: '3A', audience: 'students' }),
			g({ name: 'Rada rodziców', kind: 'custom' })
		]);
		expect(s.builtIn.map((x) => x.name)).toEqual(['Wszyscy rodzice']);
		expect(s.classes.map((r) => [r.className, r.students?.name, r.parents?.name])).toEqual([
			['1B', 'Uczniowie klasy 1B', 'Rodzice uczniów klasy 1B'],
			['3A', 'Uczniowie klasy 3A', undefined]
		]);
		expect(s.custom.map((x) => x.name)).toEqual(['Rada rodziców']);
	});
});
