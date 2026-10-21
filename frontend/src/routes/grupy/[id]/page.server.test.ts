import { describe, expect, it } from 'vitest';
import { errorTexts } from '../messages';
import { actions } from './+page.server';

const id = '00000000-0000-4000-8000-0000000000bb';
const anna = '00000000-0000-4000-8000-000000000001';

type ActionEvent = Parameters<NonNullable<(typeof actions)[string]>>[0];

/** The named action; Actions types every action as optional. */
function action(name: string) {
	const fn = actions[name];
	if (!fn) throw new Error(`no action ${name}`);
	return fn;
}

/** A fetch answering every call with status and an optional JSON body. */
const answer =
	(status: number, body?: unknown): typeof fetch =>
	async () =>
		new Response(body === undefined ? null : JSON.stringify(body), { status });

const notFound = { code: 'not_found', message: 'not found' };

const noCall: typeof fetch = async () => {
	throw new Error('the API must not be called');
};

function event(fields: [string, string][], fetch: typeof globalThis.fetch): ActionEvent {
	const form = new FormData();
	for (const [k, v] of fields) form.append(k, v);
	const request = new Request('http://localhost/grupy/x', { method: 'POST', body: form });
	return { params: { id }, request, fetch } as unknown as ActionEvent;
}

describe('grupy/[id] add', () => {
	it('refuses an empty selection without calling the API', async () => {
		await expect(action('add')(event([], noCall))).resolves.toMatchObject({
			status: 400,
			data: { memberFailure: errorTexts.noneSelected }
		});
	});

	it('reports how many people were added', async () => {
		const fields: [string, string][] = [
			['recipientId', anna],
			['recipientId', '00000000-0000-4000-8000-000000000002']
		];
		await expect(action('add')(event(fields, answer(200, { changed: 2 })))).resolves.toEqual({
			added: 2
		});
	});

	it('keeps the API status when adding fails', async () => {
		const res = await action('add')(event([['recipientId', anna]], answer(404, notFound)));
		expect(res).toMatchObject({ status: 404, data: { memberFailure: errorTexts.notFound } });
	});
});

describe('grupy/[id] remove', () => {
	it('refuses a request without a recipient', async () => {
		await expect(action('remove')(event([['recipientId', '']], noCall))).resolves.toMatchObject({
			status: 400,
			data: { memberFailure: errorTexts.unexpected }
		});
	});

	it('reports how many people were removed', async () => {
		await expect(
			action('remove')(event([['recipientId', anna]], answer(200, { changed: 1 })))
		).resolves.toEqual({ removed: 1 });
	});
});

describe('grupy/[id] delete', () => {
	it('redirects to the list after deleting', async () => {
		await expect(action('delete')(event([], answer(204)))).rejects.toMatchObject({
			status: 303,
			location: '/grupy?usunieto'
		});
	});

	it('answers 503 when the API is unreachable', async () => {
		const offline: typeof fetch = async () => {
			throw new TypeError('fetch failed');
		};
		await expect(action('delete')(event([], offline))).resolves.toMatchObject({ status: 503 });
	});
});
