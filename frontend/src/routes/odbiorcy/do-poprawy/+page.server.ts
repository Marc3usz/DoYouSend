import { getRecipientCounts, listRecipients } from '$lib/api/recipients';
import type { PageServerLoad } from './$types';
import { filterFixItems, isTruncated, mergeFixItems, parseFixView } from '../health';
import { failureText } from '../messages';

/** The API's page limit: the fix-it list shows up to this many people per channel. */
const LIMIT = 200;

export const load: PageServerLoad = async ({ url, fetch }) => {
	const view = parseFixView(url.searchParams.get('pokaz'));
	try {
		const [noEmail, noPhone, counts] = await Promise.all([
			listRecipients({ issue: 'email', limit: LIMIT }, fetch),
			listRecipients({ issue: 'sms', limit: LIMIT }, fetch),
			getRecipientCounts(fetch)
		]);
		const all = mergeFixItems(noEmail.items, noPhone.items);
		return {
			view,
			limit: LIMIT,
			failure: null,
			items: filterFixItems(all, view),
			summary: {
				noEmail: counts.noEmail,
				noPhone: counts.noPhone,
				// Exact, from the stats endpoint, even when the lists below are cut off.
				unreachable: counts.unreachable,
				truncated:
					isTruncated(noEmail.items.length, noEmail.total) ||
					isTruncated(noPhone.items.length, noPhone.total)
			}
		};
	} catch (err) {
		return { view, limit: LIMIT, failure: failureText(err), items: [], summary: null };
	}
};
