import { listRecipients } from '$lib/api/recipients';
import type { PageServerLoad } from './$types';
import {
	filterFixItems,
	isTruncated,
	mergeFixItems,
	parseFixView,
	unreachableCount
} from '../health';
import { failureText } from '../messages';

/** The API's page limit: the fix-it list shows up to this many people per channel. */
const LIMIT = 200;

export const load: PageServerLoad = async ({ url, fetch }) => {
	const view = parseFixView(url.searchParams.get('pokaz'));
	try {
		const [noEmail, noPhone] = await Promise.all([
			listRecipients({ issue: 'email', limit: LIMIT }, fetch),
			listRecipients({ issue: 'sms', limit: LIMIT }, fetch)
		]);
		const all = mergeFixItems(noEmail.items, noPhone.items);
		return {
			view,
			limit: LIMIT,
			failure: null,
			items: filterFixItems(all, view),
			summary: {
				noEmail: noEmail.total,
				noPhone: noPhone.total,
				unreachable: unreachableCount(all),
				// With a cut-off list the number of unreachable people is a lower bound.
				truncated:
					isTruncated(noEmail.items.length, noEmail.total) ||
					isTruncated(noPhone.items.length, noPhone.total)
			}
		};
	} catch (err) {
		return { view, limit: LIMIT, failure: failureText(err), items: [], summary: null };
	}
};
