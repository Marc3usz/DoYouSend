// Group endpoints of the recipients domain (DEV A), see docs/api/openapi.yaml.
import { api } from './client';
import type { Channel, Recipient } from './recipients';

export type Group = {
	id: string;
	name: string;
	description: string;
	/** system = computed from recipient data, read-only. */
	kind: 'system' | 'custom';
	memberCount: number;
	/** null for built-in groups. */
	createdAt: string | null;
};

export type GroupInput = { name: string; description: string };

export type MembershipChange = { changed: number; unchanged: number };

export type RecipientSelection = {
	groupIds?: string[];
	recipientIds?: string[];
	excludedRecipientIds?: string[];
};

export type ResolvedRecipient = {
	recipient: Recipient;
	viaGroupIds: string[];
	direct: boolean;
	/** Channels the message will actually go out on. */
	channels: Channel[];
};

export type Resolution = {
	recipients: ResolvedRecipient[];
	unknownGroupIds: string[];
	unknownRecipientIds: string[];
	excludedIds: string[];
	mergedDuplicates: number;
	summary: { total: number; complete: number; partial: number; unreachable: number };
};

type Fetch = typeof globalThis.fetch;

const groupPath = (id: string) => `/groups/${encodeURIComponent(id)}`;

/** GET /groups: built-in groups first, then custom ones by name. */
export function listGroups(fetch?: Fetch): Promise<Group[]> {
	return api<Group[]>('/groups', { fetch });
}

export function getGroup(id: string, fetch?: Fetch): Promise<Group> {
	return api<Group>(groupPath(id), { fetch });
}

export function createGroup(input: GroupInput, fetch?: Fetch): Promise<Group> {
	return api<Group>('/groups', { method: 'POST', body: input, fetch });
}

export function updateGroup(id: string, input: GroupInput, fetch?: Fetch): Promise<Group> {
	return api<Group>(groupPath(id), { method: 'PUT', body: input, fetch });
}

export function deleteGroup(id: string, fetch?: Fetch): Promise<void> {
	return api<void>(groupPath(id), { method: 'DELETE', fetch });
}

/** GET /groups/{id}/members, by last name and first name. */
export function listGroupMembers(id: string, fetch?: Fetch): Promise<Recipient[]> {
	return api<Recipient[]>(`${groupPath(id)}/members`, { fetch });
}

/** All or nothing: an unknown ID fails the whole call with unknown_recipient. */
export function addGroupMembers(
	id: string,
	recipientIds: string[],
	fetch?: Fetch
): Promise<MembershipChange> {
	return api<MembershipChange>(`${groupPath(id)}/members`, {
		method: 'POST',
		body: { recipientIds },
		fetch
	});
}

export function removeGroupMembers(
	id: string,
	recipientIds: string[],
	fetch?: Fetch
): Promise<MembershipChange> {
	return api<MembershipChange>(`${groupPath(id)}/members/remove`, {
		method: 'POST',
		body: { recipientIds },
		fetch
	});
}

/** POST /groups/resolve: the deduplicated final recipient list (ADR-0007). */
export function resolveSelection(
	selection: RecipientSelection,
	fetch?: Fetch
): Promise<Resolution> {
	return api<Resolution>('/groups/resolve', { method: 'POST', body: selection, fetch });
}
