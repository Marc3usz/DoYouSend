import { describe, expect, it } from 'vitest';
import { listHref, PAGE_SIZE, pageCount, parseListParams, toApiQuery } from './list';

describe('parseListParams', () => {
	it('reads known values', () => {
		expect(
			parseListParams(new URLSearchParams('q=+kowal+&type=student&klasa=3+a&issue=sms&page=3'))
		).toEqual({
			q: 'kowal',
			type: 'student',
			klasa: '3A',
			issue: 'sms',
			page: 3
		});
	});

	it('falls back to no filter for unknown values', () => {
		expect(parseListParams(new URLSearchParams('type=teacher&issue=fax&page=-2'))).toEqual({
			q: '',
			type: '',
			klasa: '',
			issue: '',
			page: 1
		});
	});

	it('caps the query at the API limit', () => {
		expect(parseListParams(new URLSearchParams({ q: 'a'.repeat(150) })).q).toHaveLength(100);
	});
});

describe('toApiQuery', () => {
	it('turns the page into an offset and drops empty filters', () => {
		expect(toApiQuery({ q: '', type: 'parent', klasa: '', issue: '', page: 2 })).toEqual({
			q: undefined,
			type: 'parent',
			issue: undefined,
			limit: PAGE_SIZE,
			offset: PAGE_SIZE
		});
	});
});

describe('listHref', () => {
	it('leaves out defaults', () => {
		expect(listHref({ q: '', type: '', klasa: '', issue: '', page: 1 })).toBe('/odbiorcy');
		expect(listHref({ q: 'jan k', type: '', klasa: '', issue: 'email', page: 2 })).toBe(
			'/odbiorcy?q=jan+k&issue=email&page=2'
		);
	});
});

describe('pageCount', () => {
	it('has at least one page', () => {
		expect(pageCount(0)).toBe(1);
		expect(pageCount(PAGE_SIZE)).toBe(1);
		expect(pageCount(PAGE_SIZE + 1)).toBe(2);
	});
});
