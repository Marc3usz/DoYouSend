import { getBatch } from '$lib/api/batches';
import { listGroups, type Group } from '$lib/api/groups';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ url, fetch }) => {
	let groups: Group[] = [];
	let groupsFailed = false;
	try {
		groups = await listGroups(fetch);
	} catch {
		groupsFailed = true;
	}

	const copyId = url.searchParams.get('kopia');
	let initialDraft: { subject: string; body: string; groupIds: string[] } | null = null;
	if (copyId) {
		try {
			const b = await getBatch(copyId, fetch);
			initialDraft = {
				subject: b.subject,
				body: b.body,
				groupIds: b.groups.map((g) => g.id)
			};
		} catch {
			// If copyId does not exist or caller has no access, start with empty draft.
		}
	}

	return { groups, groupsFailed, initialDraft };
};
