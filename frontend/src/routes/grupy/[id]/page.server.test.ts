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

type Call = { method: string; url: string; body: unknown };

/** answer() that also records each request, to check what the action sent. */
function recording(status: number, body?: unknown) {
	const calls: Call[] = [];
	const fetch: typeof globalThis.fetch = async (input, init) => {
		const sent = typeof init?.body === 'string' ? JSON.parse(init.body) : undefined;
		calls.push({ method: init?.method ?? 'GET', url: String(input), body: sent });
		return answer(status, body)(input, init);
	};
	return { fetch, calls };
}

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
		const jan = '00000000-0000-4000-8000-000000000002';
		const fields: [string, string][] = [
			['recipientId', anna],
			['recipientId', jan]
		];
		const api = recording(200, { changed: 2 });
		await expect(action('add')(event(fields, api.fetch))).resolves.toEqual({ added: 2 });
		expect(api.calls).toEqual([
			{ method: 'POST', url: `/api/groups/${id}/members`, body: { recipientIds: [anna, jan] } }
		]);
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
		const api = recording(200, { changed: 1 });
		await expect(action('remove')(event([['recipientId', anna]], api.fetch))).resolves.toEqual({
			removed: 1
		});
		expect(api.calls).toEqual([
			{ method: 'POST', url: `/api/groups/${id}/members/remove`, body: { recipientIds: [anna] } }
		]);
	});
});

describe('grupy/[id] delete', () => {
	it('redirects to the list after deleting', async () => {
		const api = recording(204);
		await expect(action('delete')(event([], api.fetch))).rejects.toMatchObject({
			status: 303,
			location: '/grupy?usunieto'
		});
		expect(api.calls).toEqual([{ method: 'DELETE', url: `/api/groups/${id}`, body: undefined }]);
	});

	it('answers 503 when the API is unreachable', async () => {
		const offline: typeof fetch = async () => {
			throw new TypeError('fetch failed');
		};
		await expect(action('delete')(event([], offline))).resolves.toMatchObject({ status: 503 });
	});
});
