import { describe, expect, it } from 'vitest';
import { lookupRecipients } from './recipients';

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
