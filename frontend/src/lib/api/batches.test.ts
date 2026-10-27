import { describe, expect, it } from 'vitest';
import { getBatch, listBatches, sendBatch } from './batches';

describe('sendBatch', () => {
	it('POSTs the request with the Idempotency-Key header and the body unchanged', async () => {
		const key = '00000000-0000-4000-8000-0000000000aa';
		const body = '  Zebranie {{imie}}\n';
		const fakeFetch: typeof fetch = async (input, init) => {
			expect(String(input)).toBe('/api/batches');
			expect(init?.method).toBe('POST');
			const headers = new Headers(init?.headers);
			expect(headers.get('Idempotency-Key')).toBe(key);
			expect(headers.get('Content-Type')).toBe('application/json');
			expect(JSON.parse(String(init?.body))).toEqual({
				subject: 'Zebranie',
				body,
				selection: { groupIds: ['g1'] }
			});
			return new Response(JSON.stringify({ id: 'b1', status: 'running' }), { status: 202 });
		};
		const got = await sendBatch(
			{ subject: 'Zebranie', body, selection: { groupIds: ['g1'] } },
			key,
			fakeFetch
		);
		expect(got.id).toBe('b1');
	});
});

describe('listBatches', () => {
	it('sends only the set query params', async () => {
		const fakeFetch: typeof fetch = async (input) => {
			expect(String(input)).toBe('/api/batches?status=done&limit=20&offset=40');
			return new Response(JSON.stringify({ items: [], total: 0 }), { status: 200 });
		};
		await listBatches({ status: 'done', limit: 20, offset: 40 }, fakeFetch);
	});
});

describe('getBatch', () => {
	it('escapes the id', async () => {
		const fakeFetch: typeof fetch = async (input) => {
			expect(String(input)).toBe('/api/batches/a%2Fb');
			return new Response(JSON.stringify({ id: 'a/b' }), { status: 200 });
		};
		await getBatch('a/b', fakeFetch);
	});
});
