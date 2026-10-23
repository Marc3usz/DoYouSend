import { describe, expect, it } from 'vitest';
import { load } from './+page.server';
import type { PageServerLoad } from './$types';

type LoadEvent = Parameters<PageServerLoad>[0];

const groups = [{ id: 'g1', name: 'Rada rodziców', memberCount: 5, kind: 'custom' }];

const batch = {
	id: '00000000-0000-4000-8000-000000000001',
	subject: 'Zebranie rodziców',
	body: 'Wiadomość testowa dla {{imie}}',
	groups: [{ id: 'g1', name: 'Rada rodziców' }]
};

function fakeEvent(urlStr: string, answers: Record<string, Response>): LoadEvent {
	const url = new URL(urlStr);
	const fetch: typeof globalThis.fetch = async (input) => {
		const path = String(input);
		const resp = answers[path];
		if (!resp) throw new Error(`unhandled fetch ${path}`);
		return resp;
	};
	return { url, fetch } as unknown as LoadEvent;
}

describe('wiadomosci load', () => {
	it('loads groups with no initial draft when kopia is omitted', async () => {
		const ev = fakeEvent('http://localhost/wiadomosci', {
			'/api/groups': new Response(JSON.stringify(groups), { status: 200 })
		});
		const got = await load(ev);
		expect(got).toEqual({
			groups,
			groupsFailed: false,
			initialDraft: null
		});
	});

	it('prefills draft from batch when kopia is provided and batch exists', async () => {
		const ev = fakeEvent('http://localhost/wiadomosci?kopia=00000000-0000-4000-8000-000000000001', {
			'/api/groups': new Response(JSON.stringify(groups), { status: 200 }),
			'/api/batches/00000000-0000-4000-8000-000000000001': new Response(JSON.stringify(batch), {
				status: 200
			})
		});
		const got = await load(ev);
		expect(got).toEqual({
			groups,
			groupsFailed: false,
			initialDraft: {
				subject: 'Zebranie rodziców',
				body: 'Wiadomość testowa dla {{imie}}',
				groupIds: ['g1']
			}
		});
	});

	it('gracefully handles missing kopia batch and leaves draft null', async () => {
		const ev = fakeEvent('http://localhost/wiadomosci?kopia=non-existent', {
			'/api/groups': new Response(JSON.stringify(groups), { status: 200 }),
			'/api/batches/non-existent': new Response(null, { status: 404 })
		});
		const got = await load(ev);
		expect(got).toEqual({
			groups,
			groupsFailed: false,
			initialDraft: null
		});
	});
});
