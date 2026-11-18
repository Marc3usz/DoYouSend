import { describe, expect, it } from 'vitest';
import {
	ariaSort,
	listHref,
	PAGE_SIZE,
	pageCount,
	parseListParams,
	sortHref,
	sortIndicator,
	toApiQuery
} from './list';

describe('parseListParams', () => {
	it('reads known values', () => {
		expect(
			parseListParams(
				new URLSearchParams(
					'q=+kowal+&type=student&klasa=3+a&issue=sms&sort=class&order=desc&page=3'
				)
			)
		).toEqual({
			q: 'kowal',
			type: 'student',
			klasa: '3A',
			issue: 'sms',
			sort: 'class',
			order: 'desc',
			page: 3
		});
	});

	it('falls back to defaults for unknown values', () => {
		expect(
			parseListParams(
				new URLSearchParams('type=teacher&issue=fax&sort=height&order=sideways&page=-2')
			)
		).toEqual({
			q: '',
			type: '',
			klasa: '',
			issue: '',
			sort: 'name',
			order: 'asc',
			page: 1
		});
	});

	it('caps the query at the API limit', () => {
		expect(parseListParams(new URLSearchParams({ q: 'a'.repeat(150) })).q).toHaveLength(100);
	});
});

describe('toApiQuery', () => {
	it('turns the page into an offset and drops empty filters', () => {
		expect(
			toApiQuery({
				q: '',
				type: 'parent',
				klasa: '',
				issue: '',
				sort: 'name',
				order: 'asc',
				page: 2
			})
		).toEqual({
			q: undefined,
			type: 'parent',
			issue: undefined,
			sort: undefined,
			order: undefined,
			limit: PAGE_SIZE,
			offset: PAGE_SIZE
		});
	});

	it('includes non-default sort and order in api query', () => {
		expect(
			toApiQuery({
				q: '',
				type: '',
				klasa: '',
				issue: '',
				sort: 'email',
				order: 'desc',
				page: 1
			})
		).toEqual({
			q: undefined,
			type: undefined,
			issue: undefined,
			sort: 'email',
			order: 'desc',
			limit: PAGE_SIZE,
			offset: 0
		});
	});
});

describe('listHref', () => {
	it('leaves out defaults', () => {
		expect(
			listHref({ q: '', type: '', klasa: '', issue: '', sort: 'name', order: 'asc', page: 1 })
		).toBe('/odbiorcy');
		expect(
			listHref({
				q: 'jan k',
				type: '',
				klasa: '',
				issue: 'email',
				sort: 'name',
				order: 'asc',
				page: 2
			})
		).toBe('/odbiorcy?q=jan+k&issue=email&page=2');
	});

	it('keeps sort and order in href when not default', () => {
		expect(
			listHref({
				q: '',
				type: '',
				klasa: '',
				issue: '',
				sort: 'type',
				order: 'desc',
				page: 1
			})
		).toBe('/odbiorcy?sort=type&order=desc');
	});
});

describe('sortHref and sortIndicator', () => {
	it('toggles direction when clicking active column', () => {
		const current = parseListParams(new URLSearchParams('sort=name&order=asc&page=3'));
		expect(sortHref(current, 'name')).toBe('/odbiorcy?order=desc');
	});

	it('resets to asc and page 1 when clicking new column', () => {
		const current = parseListParams(new URLSearchParams('sort=name&order=desc&page=3'));
		expect(sortHref(current, 'class')).toBe('/odbiorcy?sort=class');
	});

	it('returns correct sort indicators', () => {
		const pAsc = parseListParams(new URLSearchParams('sort=name&order=asc'));
		expect(sortIndicator(pAsc, 'name')).toBe('▲');
		expect(sortIndicator(pAsc, 'email')).toBe('↕');

		const pDesc = parseListParams(new URLSearchParams('sort=type&order=desc'));
		expect(sortIndicator(pDesc, 'type')).toBe('▼');
		expect(sortIndicator(pDesc, 'name')).toBe('↕');
	});

	it('tells screen readers how a column is sorted', () => {
		const pDesc = parseListParams(new URLSearchParams('sort=type&order=desc'));
		expect(ariaSort(pDesc, 'type')).toBe('descending');
		expect(ariaSort(pDesc, 'name')).toBe('none');
		const pAsc = parseListParams(new URLSearchParams('sort=class&order=asc'));
		expect(ariaSort(pAsc, 'class')).toBe('ascending');
	});
});

describe('pageCount', () => {
	it('has at least one page', () => {
		expect(pageCount(0)).toBe(1);
		expect(pageCount(PAGE_SIZE)).toBe(1);
		expect(pageCount(PAGE_SIZE + 1)).toBe(2);
	});
});
