// URL state of the recipient list: /odbiorcy?q=...&type=...&klasa=3A&issue=...&page=2.
// Keeping it in the URL makes a filtered view linkable and the back button work.
import type { Channel, RecipientListQuery, RecipientType } from '$lib/api/recipients';

export const PAGE_SIZE = 50;

export type ListParams = {
	q: string;
	type: RecipientType | '';
	/** Class filter as typed; the backend normalizes it ("3 a" = "3A"). */
	klasa: string;
	issue: Channel | '';
	page: number;
};

/** Reads the list parameters; unknown values fall back to "no filter". */
export function parseListParams(search: URLSearchParams): ListParams {
	const type = search.get('type');
	const issue = search.get('issue');
	const page = Number.parseInt(search.get('page') ?? '', 10);
	return {
		q: (search.get('q') ?? '').trim().slice(0, 100),
		type: type === 'parent' || type === 'student' ? type : '',
		klasa: (search.get('klasa') ?? '').replace(/\s+/g, '').toUpperCase().slice(0, 6),
		issue: issue === 'email' || issue === 'sms' ? issue : '',
		page: Number.isFinite(page) && page > 0 ? page : 1
	};
}

/** The API query for these parameters. */
export function toApiQuery(p: ListParams): RecipientListQuery {
	return {
		q: p.q || undefined,
		type: p.type || undefined,
		class: p.klasa || undefined,
		issue: p.issue || undefined,
		limit: PAGE_SIZE,
		offset: (p.page - 1) * PAGE_SIZE
	};
}

/** The list URL for params, leaving out defaults so links stay short. */
export function listHref(p: ListParams): string {
	const search = new URLSearchParams();
	if (p.q) search.set('q', p.q);
	if (p.type) search.set('type', p.type);
	if (p.klasa) search.set('klasa', p.klasa);
	if (p.issue) search.set('issue', p.issue);
	if (p.page > 1) search.set('page', String(p.page));
	return search.size > 0 ? `/odbiorcy?${search}` : '/odbiorcy';
}

export function pageCount(total: number): number {
	return Math.max(1, Math.ceil(total / PAGE_SIZE));
}
