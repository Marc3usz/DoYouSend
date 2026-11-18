import { fail } from '@sveltejs/kit';
import { getSettings, sendTestMessage, updateSettings } from '$lib/api/admin';
import type { Actions, PageServerLoad } from './$types';
import {
	collectChanges,
	CONFIRM_PHRASE,
	keptValues,
	settingsFailureOf,
	testFailureOf
} from './settings';

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
				action: 'save' as const,
				failure: 'Nie udało się wczytać ustawień. Odśwież stronę.',
				fields: {},
				values: {},
				needsConfirmation: false
			});
		}
		const changes = collectChanges(data, current);
		if (Object.keys(changes).length === 0) {
			return { action: 'save' as const, done: 'Nic się nie zmieniło.' };
		}
		const confirmed =
			String(data.get('confirm') ?? '')
				.trim()
				.toUpperCase() === CONFIRM_PHRASE;
		try {
			const saved = await updateSettings(changes, confirmed, fetch);
			return {
				action: 'save' as const,
				done: saved.sendsExternally
					? 'Zapisano. Wysyłka działa naprawdę — wiadomości trafiają do odbiorców.'
					: 'Zapisano. Zmiany działają od razu.'
			};
		} catch (err) {
			const f = settingsFailureOf(err);
			return fail(f.status, { action: 'save' as const, ...f, values: keptValues(data, current) });
		}
	},

	test: async ({ request, fetch }) => {
		const data = await request.formData();
		const channel = data.get('channel') === 'sms' ? 'sms' : 'email';
		const to = String(data.get('to') ?? '').trim();
		if (channel === 'sms' && to === '') {
			return fail(400, {
				action: 'test' as const,
				channel,
				to,
				failure: 'Podaj numer telefonu, na który ma przyjść SMS testowy.'
			});
		}
		try {
			const result = await sendTestMessage({ channel, ...(to ? { to } : {}) }, fetch);
			const where = channel === 'email' ? (to ? to : 'Twój adres') : to;
			return {
				action: 'test' as const,
				channel,
				to,
				dryRun: result.dryRun,
				done: result.dryRun
					? `Tryb testowy — nic nie wyszło na zewnątrz. Wiadomość do: ${where} została tylko zapisana.`
					: `Wysłano wiadomość testową do: ${where} (identyfikator operatora: ${result.providerMessageId}).`
			};
		} catch (err) {
			const { status, failure } = testFailureOf(err);
			return fail(status, { action: 'test' as const, channel, to, failure });
		}
	}
};
