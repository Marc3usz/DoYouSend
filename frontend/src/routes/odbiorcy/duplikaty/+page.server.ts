import { listDuplicates } from '$lib/api/recipients';
import type { PageServerLoad } from './$types';
import { failureText } from '../messages';

export const load: PageServerLoad = async ({ fetch }) => {
	try {
		return { groups: await listDuplicates(fetch), failure: null };
	} catch (err) {
		return { groups: [], failure: failureText(err) };
	}
};
