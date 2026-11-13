import { describe, expect, it } from 'vitest';
import { load } from './+page.server';
import type { PageServerLoad } from './$types';

type LoadEvent = Parameters<PageServerLoad>[0];

const groups = [{ id: 'g1', name: 'Rada rodziców', memberCount: 5, kind: 'custom' }];

const batch = {
	id: '00000000-0000-4000-8000-000000000001',
	subject: 'Zebranie rodziców',
	body: 'Wiadomość testowa dla {{imie}}',
	groups: [{ id: 'g1', name: 'Rada rodziców' }],
	recipientIds: ['00000000-0000-4000-8000-000000000002'],
	recipients: [
		{
			recipientId: '00000000-0000-4000-8000-000000000002',
			firstName: 'Jan',
			lastName: 'Kowalski'
		}
	]
};

const senders = {
	defaultId: 'shared',
	items: [
		{
			id: 'shared',
			email: 'szkola@example.test',
			name: 'Szkoła',
			kind: 'shared',
			usable: true,
			reason: null
		}
	]
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

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

describe('wiadomosci load', () => {
	it('loads groups with no initial draft when kopia is omitted', async () => {
		const ev = fakeEvent('http://localhost/wiadomosci', { '/api/groups': json(groups) });
		const got = await load(ev);
		expect(got).toEqual({
			groups,
			groupsFailed: false,
			initialDraft: null,
			copiedBatchSubject: null,
			copyFailed: false,
			people: null,
			senders: null
		});
	});

	it('loads the sender options', async () => {
		const ev = fakeEvent('http://localhost/wiadomosci', {
			'/api/groups': json(groups),
			'/api/me/senders': json(senders)
		});
		const got = await load(ev);
		expect(got?.senders).toEqual(senders);
	});

	it('prefills draft from batch when kopia is provided and batch exists', async () => {
		const ev = fakeEvent('http://localhost/wiadomosci?kopia=00000000-0000-4000-8000-000000000001', {
			'/api/groups': json(groups),
			'/api/batches/00000000-0000-4000-8000-000000000001': json(batch)
		});
		const got = await load(ev);
		expect(got?.copiedBatchSubject).toBe('Zebranie rodziców');
		expect(got?.copyFailed).toBe(false);
		expect(got?.initialDraft).toEqual({
			subject: 'Zebranie rodziców',
			body: 'Wiadomość testowa dla {{imie}}',
			channelMode: 'both',
			groupIds: ['g1'],
			excludedRecipientIds: [],
			excludedPeople: [],
			recipients: [
				{ id: '00000000-0000-4000-8000-000000000002', firstName: 'Jan', lastName: 'Kowalski' }
			]
		});
	});

	it('restores the channel choice of an SMS-only batch', async () => {
		const ev = fakeEvent('http://localhost/wiadomosci?kopia=b2', {
			'/api/groups': json(groups),
			'/api/batches/b2': json({ ...batch, id: 'b2', subject: '', channels: ['sms'] })
		});
		const got = await load(ev);
		expect(got?.initialDraft?.channelMode).toBe('sms');
	});

	it('names the excluded recipients of the copied batch', async () => {
		const excludedId = '00000000-0000-4000-8000-000000000099';
		const ev = fakeEvent('http://localhost/wiadomosci?kopia=00000000-0000-4000-8000-000000000001', {
			'/api/groups': json(groups),
			'/api/batches/00000000-0000-4000-8000-000000000001': json({
				...batch,
				excludedRecipientIds: [excludedId]
			}),
			'/api/recipients/lookup?limit=200&offset=0': json({
				items: [
					{
						id: excludedId,
						firstName: 'Anna',
						lastName: 'Nowak',
						type: 'parent',
						classes: [],
						issues: []
					}
				],
				total: 1
			})
		});
		const got = await load(ev);
		expect(got?.initialDraft?.excludedRecipientIds).toEqual([excludedId]);
		expect(got?.initialDraft?.excludedPeople).toEqual([{ id: excludedId, name: 'Nowak Anna' }]);
	});

	it('says so when the batch to copy cannot be loaded', async () => {
		const ev = fakeEvent('http://localhost/wiadomosci?kopia=non-existent', {
			'/api/groups': json(groups),
			'/api/batches/non-existent': new Response(null, { status: 404 })
		});
		const got = await load(ev);
		expect(got?.initialDraft).toBeNull();
		expect(got?.copyFailed).toBe(true);
	});

	it('loads the first page of the recipient picker', async () => {
		const people = {
			items: [
				{
					id: '00000000-0000-4000-8000-000000000003',
					firstName: 'Anna',
					lastName: 'Testowa',
					type: 'parent',
					classes: ['3A'],
					issues: []
				}
			],
			total: 1
		};
		const ev = fakeEvent('http://localhost/wiadomosci', {
			'/api/groups': json(groups),
			'/api/recipients/lookup?limit=10': json(people)
		});
		const got = await load(ev);
		expect(got?.people).toEqual(people);
	});
});
