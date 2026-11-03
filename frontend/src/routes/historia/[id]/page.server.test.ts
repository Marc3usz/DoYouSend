import { describe, expect, it } from 'vitest';
import { load } from './+page.server';
import type { PageServerLoad } from './$types';

type LoadEvent = Parameters<PageServerLoad>[0];

const fakeBatch = {
	id: '00000000-0000-4000-8000-000000000001',
	subject: 'Powiadomienie',
	status: 'done',
	createdBy: { id: 'u1', fullName: 'Dyrektor' },
	createdAt: '2026-10-30T10:00:00Z',
	finishedAt: '2026-10-30T10:05:00Z',
	groups: [],
	recipientCount: 1,
	partialCount: 0,
	failedCount: 0,
	totalSmsParts: 1,
	costMilli: 80,
	body: 'Ważna treść dla {{imie}}',
	recipientIds: ['00000000-0000-4000-8000-000000000002'],
	excludedRecipientIds: ['00000000-0000-4000-8000-000000000099'],
	recipients: [
		{
			recipientId: '00000000-0000-4000-8000-000000000002',
			firstName: 'Jan',
			lastName: 'Kowalski',
			renderedBody: 'Ważna treść dla Jan',
			partial: false,
			unreachable: false,
			deliveries: []
		}
	]
};

const fakeLookup = {
	items: [
		{
			id: '00000000-0000-4000-8000-000000000099',
			firstName: 'Anna',
			lastName: 'Nowak',
			type: 'parent',
			classes: ['3A'],
			issues: []
		}
	],
	total: 1
};

function fakeEvent(id: string, answers: Record<string, Response>): LoadEvent {
	const params = { id };
	const fetch: typeof globalThis.fetch = async (input) => {
		const path = String(input);
		const resp = answers[path];
		if (!resp) throw new Error(`unhandled fetch ${path}`);
		return resp;
	};
	return { params, fetch } as unknown as LoadEvent;
}

describe('historia/[id] load', () => {
	it('loads batch and resolves excluded recipients names', async () => {
		const ev = fakeEvent('00000000-0000-4000-8000-000000000001', {
			'/api/batches/00000000-0000-4000-8000-000000000001': new Response(JSON.stringify(fakeBatch), {
				status: 200
			}),
			'/api/recipients/lookup?limit=200': new Response(JSON.stringify(fakeLookup), { status: 200 })
		});

		const got = await load(ev);
		expect(got?.batch.id).toBe('00000000-0000-4000-8000-000000000001');
		expect(got?.excludedRecipients).toEqual([
			{
				id: '00000000-0000-4000-8000-000000000099',
				name: 'Nowak Anna'
			}
		]);
	});

	it('falls back to id if lookupRecipients fails', async () => {
		const ev = fakeEvent('00000000-0000-4000-8000-000000000001', {
			'/api/batches/00000000-0000-4000-8000-000000000001': new Response(JSON.stringify(fakeBatch), {
				status: 200
			}),
			'/api/recipients/lookup?limit=200': new Response(null, { status: 500 })
		});

		const got = await load(ev);
		expect(got?.excludedRecipients).toEqual([
			{
				id: '00000000-0000-4000-8000-000000000099',
				name: '00000000-0000-4000-8000-000000000099'
			}
		]);
	});

	it('returns empty excludedRecipients when batch has no exclusions', async () => {
		const batchNoExclusions = { ...fakeBatch, excludedRecipientIds: [] };
		const ev = fakeEvent('00000000-0000-4000-8000-000000000001', {
			'/api/batches/00000000-0000-4000-8000-000000000001': new Response(
				JSON.stringify(batchNoExclusions),
				{ status: 200 }
			)
		});

		const got = await load(ev);
		expect(got?.excludedRecipients).toEqual([]);
	});
});
