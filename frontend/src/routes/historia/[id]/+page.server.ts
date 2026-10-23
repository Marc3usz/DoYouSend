import { error } from '@sveltejs/kit';
import { getBatch } from '$lib/api/batches';
import { ApiError } from '$lib/api/client';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params, fetch }) => {
	try {
		return { batch: await getBatch(params.id, fetch) };
	} catch (err) {
		// Another sender's batch is a 404 too: the backend does not say it exists.
		if (err instanceof ApiError && err.status === 404) {
			error(404, 'Nie ma takiej wysyłki.');
		}
		error(502, 'Nie udało się wczytać wysyłki. Odśwież stronę.');
	}
};
