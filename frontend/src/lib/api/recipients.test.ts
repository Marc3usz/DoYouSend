import { describe, expect, it } from 'vitest';
import { lookupNames, lookupRecipients } from './recipients';

describe('lookupRecipients', () => {
	it('calls GET /api/recipients/lookup with the set params only', async () => {
		const page = {
			items: [
				{
					id: '00000000-0000-4000-8000-000000000001',
					firstName: 'Anna',
					lastName: 'Testowa',
					type: 'parent',
					classes: ['3A'],
					issues: []
				}
			],
			total: 1
		};
		const fakeFetch: typeof fetch = async (input) => {
			expect(String(input)).toBe('/api/recipients/lookup?q=anna&class=3A&limit=20');
			return new Response(JSON.stringify(page), { status: 200 });
		};
		const got = await lookupRecipients(
			{ q: 'anna', type: undefined, class: '3A', limit: 20 },
			fakeFetch
		);
		expect(got).toEqual(page);
	});

	it('calls GET /api/recipients/lookup without params', async () => {
		const fakeFetch: typeof fetch = async (input) => {
			expect(String(input)).toBe('/api/recipients/lookup');
			return new Response(JSON.stringify({ items: [], total: 0 }), { status: 200 });
		};
		expect((await lookupRecipients({}, fakeFetch)).total).toBe(0);
	});
});

describe('lookupNames', () => {
	const person = (n: number) => ({
		id: `id-${n}`,
		firstName: `Imię${n}`,
		lastName: `Nazwisko${n}`,
		type: 'parent',
		classes: [],
		issues: []
	});
	// 450 people served 200 per page, like the backend's maximum.
	const all = Array.from({ length: 450 }, (_, i) => person(i));
	const pages =
		(seen: string[]): typeof fetch =>
		async (input) => {
			const url = new URL(String(input), 'http://x');
			seen.push(url.search);
			const offset = Number(url.searchParams.get('offset'));
			const limit = Number(url.searchParams.get('limit'));
			const items = all.slice(offset, offset + limit);
			return new Response(JSON.stringify({ items, total: all.length }), { status: 200 });
		};

	it('pages until every id is found', async () => {
		const seen: string[] = [];
		const got = await lookupNames(['id-5', 'id-420'], pages(seen));
		expect(got).toEqual(
			new Map([
				['id-5', 'Nazwisko5 Imię5'],
				['id-420', 'Nazwisko420 Imię420']
			])
		);
		expect(seen).toEqual(['?limit=200&offset=0', '?limit=200&offset=200', '?limit=200&offset=400']);
	});

	it('stops after the first page when it has everything', async () => {
		const seen: string[] = [];
		await lookupNames(['id-1'], pages(seen));
		expect(seen).toEqual(['?limit=200&offset=0']);
	});

	it('stops at the end of the list when an id no longer exists', async () => {
		const seen: string[] = [];
		const got = await lookupNames(['id-1', 'deleted'], pages(seen));
		expect(got.get('id-1')).toBe('Nazwisko1 Imię1');
		expect(got.has('deleted')).toBe(false);
		expect(seen).toHaveLength(3);
	});
});
