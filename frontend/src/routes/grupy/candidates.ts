// Candidates offered when adding people to a custom group: the search result
// minus the people who are already members.
import type { Recipient } from '$lib/api/recipients';

/** How many candidates to offer at once. */
export const CANDIDATE_LIMIT = 20;

/** The API's page limit (GET /recipients). */
const API_MAX_LIMIT = 200;

/**
 * How many search results to ask for. Members are removed after the search, so
 * the page must be able to hold every member plus the candidates; otherwise a
 * group that already contains the 20 best matches shows "nobody found".
 */
export function searchLimit(memberCount: number): number {
	return Math.min(API_MAX_LIMIT, CANDIDATE_LIMIT + Math.max(0, memberCount));
}

export type Candidates = {
	candidates: Recipient[];
	/** More people match than are shown: the search should be narrowed. */
	moreCandidates: boolean;
	/** Matches left out because they already belong to the group. */
	alreadyMembers: number;
};

export function pickCandidates(
	items: Recipient[],
	total: number,
	memberIds: ReadonlySet<string>
): Candidates {
	const fresh = items.filter((r) => !memberIds.has(r.id));
	return {
		candidates: fresh.slice(0, CANDIDATE_LIMIT),
		moreCandidates: fresh.length > CANDIDATE_LIMIT || total > items.length,
		alreadyMembers: items.length - fresh.length
	};
}
