import { listGroups, type Group } from '$lib/api/groups';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ fetch }) => {
	try {
		return { groups: await listGroups(fetch), groupsFailed: false };
	} catch {
		return { groups: [] as Group[], groupsFailed: true };
	}
};
