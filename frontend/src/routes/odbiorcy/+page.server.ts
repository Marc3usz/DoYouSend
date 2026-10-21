import { getRecipientCounts, listRecipients, type RecipientPage } from '$lib/api/recipients';
import type { PageServerLoad } from './$types';
import { parseListParams, toApiQuery } from './list';
import { failureText } from './messages';

export const load: PageServerLoad = async ({ url, fetch }) => {
	const params = parseListParams(url.searchParams);
	// The summary is a nice-to-have: if only it fails, the list still renders.
	// A filtered or paged view hides the panel, so it does not pay for the 5 requests.
	const unfiltered = !(params.q || params.type || params.klasa || params.issue);
	const counts = unfiltered ? getRecipientCounts(fetch).catch(() => null) : Promise.resolve(null);
	try {
		const page: RecipientPage = await listRecipients(toApiQuery(params), fetch);
		return { params, page, failure: null, counts: await counts };
	} catch (err) {
		return { params, page: null, failure: failureText(err), counts: await counts };
	}
};
