import { error } from '@sveltejs/kit';
import { getBatch } from '$lib/api/batches';
import { ApiError } from '$lib/api/client';
import { lookupNames } from '$lib/api/recipients';
import type { PageServerLoad } from './$types';

export type ExcludedPerson = {
	id: string;
	name: string;
};

export const load: PageServerLoad = async ({ params, fetch }) => {
	try {
		const batch = await getBatch(params.id, fetch);
		let excludedRecipients: ExcludedPerson[] = [];

		if (batch.excludedRecipientIds && batch.excludedRecipientIds.length > 0) {
			try {
				const lookupMap = await lookupNames(batch.excludedRecipientIds, fetch);
				excludedRecipients = batch.excludedRecipientIds.map((id) => ({
					id,
					name: lookupMap.get(id) || id
				}));
			} catch {
				excludedRecipients = batch.excludedRecipientIds.map((id) => ({ id, name: id }));
			}
		}

		return { batch, excludedRecipients };
	} catch (err) {
		// Another sender's batch is a 404 too: the backend does not say it exists.
		if (err instanceof ApiError && err.status === 404) {
			error(404, 'Nie ma takiej wysyłki.');
		}
		error(502, 'Nie udało się wczytać wysyłki. Odśwież stronę.');
	}
};
