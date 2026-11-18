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
	const all = new Map(Array.from({ length: 450 }, (_, i) => [`id-${i}`, person(i)]));
	const byIds =
		(seen: string[][]): typeof fetch =>
		async (input) => {
			const url = new URL(String(input), 'http://x');
			const ids = (url.searchParams.get('ids') ?? '').split(',');
			seen.push(ids);
			expect(url.searchParams.get('limit')).toBe('200');
			const items = ids.flatMap((id) => {
				const p = all.get(id);
				return p ? [p] : [];
			});
			return new Response(JSON.stringify({ items, total: items.length }), { status: 200 });
		};

	it('asks only for the wanted ids, in one call', async () => {
		const seen: string[][] = [];
		const got = await lookupNames(['id-5', 'id-420', 'id-5'], byIds(seen));
		expect(got).toEqual(
			new Map([
				['id-5', 'Nazwisko5 Imię5'],
				['id-420', 'Nazwisko420 Imię420']
			])
		);
		expect(seen).toEqual([['id-5', 'id-420']]);
	});

	it('splits more than 200 ids into several calls', async () => {
		const seen: string[][] = [];
		const ids = Array.from({ length: 450 }, (_, i) => `id-${i}`);
		const got = await lookupNames(ids, byIds(seen));
		expect(got.size).toBe(450);
		expect(seen.map((s) => s.length)).toEqual([200, 200, 50]);
	});

	it('leaves out ids that no longer exist and makes no call for none', async () => {
		const seen: string[][] = [];
		const got = await lookupNames(['id-1', 'deleted'], byIds(seen));
		expect(got.get('id-1')).toBe('Nazwisko1 Imię1');
		expect(got.has('deleted')).toBe(false);
		expect((await lookupNames([], byIds(seen))).size).toBe(0);
		expect(seen).toHaveLength(1);
	});
});
