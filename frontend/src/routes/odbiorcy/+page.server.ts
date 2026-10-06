import { listRecipients, type RecipientPage } from '$lib/api/recipients';
import type { PageServerLoad } from './$types';
import { parseListParams, toApiQuery } from './list';
import { failureText } from './messages';

export const load: PageServerLoad = async ({ url, fetch }) => {
	const params = parseListParams(url.searchParams);
	try {
		const page: RecipientPage = await listRecipients(toApiQuery(params), fetch);
		return { params, page, failure: null };
	} catch (err) {
		return { params, page: null, failure: failureText(err) };
	}
};
