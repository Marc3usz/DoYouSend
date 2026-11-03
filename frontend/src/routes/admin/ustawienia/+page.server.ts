import { fail } from '@sveltejs/kit';
import { getSettings, updateSettings } from '$lib/api/admin';
import type { Actions, PageServerLoad } from './$types';
import { collectChanges, CONFIRM_PHRASE, keptValues, settingsFailureOf } from './settings';

export const load: PageServerLoad = async ({ fetch }) => {
	try {
		return { settings: await getSettings(fetch), loadFailed: false };
	} catch {
		return { settings: null, loadFailed: true };
	}
};

export const actions: Actions = {
	save: async ({ request, fetch }) => {
		const data = await request.formData();
		let current;
		try {
			current = await getSettings(fetch);
		} catch (err) {
			return fail(settingsFailureOf(err).status, {
				failure: 'Nie udało się wczytać ustawień. Odśwież stronę.',
				fields: {},
				values: {},
				needsConfirmation: false
			});
		}
		const changes = collectChanges(data, current);
		if (Object.keys(changes).length === 0) {
			return { done: 'Nic się nie zmieniło.' };
		}
		const confirmed =
			String(data.get('confirm') ?? '')
				.trim()
				.toUpperCase() === CONFIRM_PHRASE;
		try {
			const saved = await updateSettings(changes, confirmed, fetch);
			return {
				done: saved.sendsExternally
					? 'Zapisano. Wysyłka działa naprawdę — wiadomości trafiają do odbiorców.'
					: 'Zapisano. Zmiany działają od razu.'
			};
		} catch (err) {
			const f = settingsFailureOf(err);
			return fail(f.status, { ...f, values: keptValues(data, current) });
		}
	}
};
