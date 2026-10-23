import { getBatch } from '$lib/api/batches';
import { listGroups, type Group } from '$lib/api/groups';
import type { RecipientLookup } from '$lib/api/recipients';
import type { PageServerLoad } from './$types';

export type InitialDraft = {
	subject: string;
	body: string;
	groupIds: string[];
	recipients: RecipientLookup[];
};

export const load: PageServerLoad = async ({ url, fetch }) => {
	let groups: Group[] = [];
	let groupsFailed = false;
	try {
		groups = await listGroups(fetch);
	} catch {
		groupsFailed = true;
	}

	const copyId = url.searchParams.get('kopia');
	let initialDraft: InitialDraft | null = null;
	if (copyId) {
		try {
			const b = await getBatch(copyId, fetch);
			const manualIds = new Set(b.recipientIds ?? []);
			const manualRecipients: RecipientLookup[] = (b.recipients ?? [])
				.filter((r) => manualIds.has(r.recipientId))
				.map((r) => ({
					id: r.recipientId,
					firstName: r.firstName,
					lastName: r.lastName,
					type: 'parent',
					classes: [],
					issues: []
				}));
			initialDraft = {
				subject: b.subject,
				body: b.body,
				groupIds: b.groups.map((g) => g.id),
				recipients: manualRecipients
			};
		} catch {
			// If copyId does not exist or caller has no access, start with empty draft.
		}
	}

	return { groups, groupsFailed, initialDraft };
};
