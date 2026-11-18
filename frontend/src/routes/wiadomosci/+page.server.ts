import { getBatch } from '$lib/api/batches';
import { listGroups, type Group } from '$lib/api/groups';
import { lookupNames, lookupRecipients } from '$lib/api/recipients';
import { mySenders } from '$lib/api/senders';
import { channelModeOf, type ChannelMode } from '$lib/format/batches';
import type { PageServerLoad } from './$types';

/** A person picked by hand; the composer shows only the name. */
export type PickedPerson = { id: string; firstName: string; lastName: string };

export type InitialDraft = {
	subject: string;
	body: string;
	channelMode: ChannelMode;
	groupIds: string[];
	recipients: PickedPerson[];
	excludedRecipientIds: string[];
	/** Names of the excluded people ("Nazwisko Imię"); missing ones show a short id. */
	excludedPeople: { id: string; name: string }[];
};

/** Rows per page of the recipient picker. */
const PICKER_PAGE_SIZE = 10;

export const load: PageServerLoad = async ({ url, fetch }) => {
	// The picker's first page; on failure the picker shows an error and a retry.
	const people = lookupRecipients({ limit: PICKER_PAGE_SIZE }, fetch).catch(() => null);
	// Without the list the picker is hidden and the backend uses its default sender.
	const senders = mySenders(fetch).catch(() => null);
	let groups: Group[] = [];
	let groupsFailed = false;
	try {
		groups = await listGroups(fetch);
	} catch {
		groupsFailed = true;
	}

	const copyId = url.searchParams.get('kopia');
	let initialDraft: InitialDraft | null = null;
	let copiedBatchSubject: string | null = null;
	let copyFailed = false;
	if (copyId) {
		try {
			const b = await getBatch(copyId, fetch);
			copiedBatchSubject = b.subject;
			const manualIds = new Set(b.recipientIds ?? []);
			const recipients: PickedPerson[] = (b.recipients ?? [])
				.filter((r) => manualIds.has(r.recipientId))
				.map((r) => ({ id: r.recipientId, firstName: r.firstName, lastName: r.lastName }));
			const excludedRecipientIds = b.excludedRecipientIds ?? [];
			let excludedPeople: { id: string; name: string }[] = [];
			if (excludedRecipientIds.length > 0) {
				const names = await lookupNames(excludedRecipientIds, fetch).catch(
					() => new Map<string, string>()
				);
				excludedPeople = excludedRecipientIds.flatMap((id) => {
					const name = names.get(id);
					return name ? [{ id, name }] : [];
				});
			}
			initialDraft = {
				subject: b.subject,
				body: b.body,
				channelMode: channelModeOf(b.channels),
				groupIds: b.groups.map((g) => g.id),
				recipients,
				excludedRecipientIds,
				excludedPeople
			};
		} catch {
			// Unknown, someone else's or unreachable: start empty and say so.
			copyFailed = true;
		}
	}

	return {
		groups,
		groupsFailed,
		initialDraft,
		copiedBatchSubject,
		copyFailed,
		people: await people,
		senders: await senders
	};
};
