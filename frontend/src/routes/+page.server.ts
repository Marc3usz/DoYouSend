import { getRecipientCounts } from '$lib/api/recipients';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ parent, fetch }) => {
	const { user } = await parent();
	// Only the recipient base is admin-only; the tile is a hint, so a failure just hides it.
	const counts = user?.role === 'admin' ? await getRecipientCounts(fetch).catch(() => null) : null;
	return { counts };
};
