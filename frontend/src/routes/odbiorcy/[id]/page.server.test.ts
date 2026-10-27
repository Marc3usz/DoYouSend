import { describe, expect, it } from 'vitest';
import { errorTexts } from '../messages';
import { actions, load } from './+page.server';

const id = '00000000-0000-4000-8000-000000000001';

type LoadEvent = Parameters<typeof load>[0];
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

const loadEvent = (fetch: typeof globalThis.fetch) =>
	({ params: { id }, fetch }) as unknown as LoadEvent;
const actionEvent = (fetch: typeof globalThis.fetch) =>
	({ params: { id }, fetch }) as unknown as ActionEvent;

describe('odbiorcy/[id] load', () => {
	it('returns the recipient', async () => {
		const recipient = { id, firstName: 'Anna', lastName: 'Testowa' };
		await expect(load(loadEvent(answer(200, recipient)))).resolves.toEqual({ recipient });
	});

	it('answers not_found from the API with a 404 page', async () => {
		await expect(load(loadEvent(answer(404, notFound)))).rejects.toMatchObject({
			status: 404,
			body: { message: errorTexts.notFound }
		});
	});

	it('treats a malformed ID (400) as a missing recipient', async () => {
		await expect(load(loadEvent(answer(400)))).rejects.toMatchObject({ status: 404 });
	});

	it('answers other failures with 503', async () => {
		await expect(load(loadEvent(answer(500)))).rejects.toMatchObject({ status: 503 });
	});
});

describe('odbiorcy/[id] delete', () => {
	it('redirects to the list after deleting', async () => {
		const calls: string[] = [];
		const fetch: typeof globalThis.fetch = async (input, init) => {
			calls.push(`${init?.method} ${String(input)}`);
			return answer(204)(input, init);
		};
		await expect(action('delete')(actionEvent(fetch))).rejects.toMatchObject({
			status: 303,
			location: '/odbiorcy?usunieto'
		});
		expect(calls).toEqual([`DELETE /api/recipients/${id}`]);
	});

	it('keeps the API status when deleting fails', async () => {
		await expect(action('delete')(actionEvent(answer(404, notFound)))).resolves.toMatchObject({
			status: 404,
			data: { deleteFailure: errorTexts.notFound }
		});
	});

	it('reads a plain 404 as an API without DATABASE_URL, not a missing recipient', async () => {
		await expect(action('delete')(actionEvent(answer(404)))).resolves.toMatchObject({
			status: 404,
			data: { deleteFailure: errorTexts.unavailable }
		});
	});
});
