import { getRecipientCounts, listAllRecipients } from '$lib/api/recipients';
import type { PageServerLoad } from './$types';
import { filterFixItems, mergeFixItems, parseFixView } from '../health';
import { failureText } from '../messages';
import { pageParam } from '$lib/format/text';

/** People per page of the fix-it list. */
const PAGE_SIZE = 50;

export const load: PageServerLoad = async ({ url, fetch }) => {
	const view = parseFixView(url.searchParams.get('pokaz'));
	try {
		// The whole lists, so nobody with a problem is left out; only the view is paged.
		const [noEmail, noPhone, counts] = await Promise.all([
			listAllRecipients({ issue: 'email' }, fetch),
			listAllRecipients({ issue: 'sms' }, fetch),
			getRecipientCounts(fetch)
		]);
		const filtered = filterFixItems(mergeFixItems(noEmail, noPhone), view);
		const pages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
		const page = Math.min(pageParam(url.searchParams.get('strona')), pages);
		return {
			view,
			failure: null,
			items: filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE),
			total: filtered.length,
			page,
			pages,
			summary: {
				noEmail: counts.noEmail,
				noPhone: counts.noPhone,
				unreachable: counts.unreachable
			}
		};
	} catch (err) {
		return {
			view,
			failure: failureText(err),
			items: [],
			total: 0,
			page: 1,
			pages: 1,
			summary: null
		};
	}
};
