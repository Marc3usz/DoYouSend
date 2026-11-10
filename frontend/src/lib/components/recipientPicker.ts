// State helpers of RecipientPicker: the filters, the API query they map to and
// the "select the whole page" checkbox.
import type { RecipientListQuery, RecipientLookup, RecipientType } from '$lib/api/recipients';

export type PickerFilters = {
	q: string;
	type: RecipientType | '';
	/** Class as typed; normalized like the backend does ("3 a" = "3A"). */
	klasa: string;
};

export const emptyFilters: PickerFilters = { q: '', type: '', klasa: '' };

/** The GET /recipients/lookup query for the filters and a 1-based page. */
export function pickerQuery(
	f: PickerFilters,
	page: number,
	pageSize: number
): Omit<RecipientListQuery, 'issue'> {
	const klasa = f.klasa.replace(/\s+/g, '').toUpperCase().slice(0, 6);
	return {
		q: f.q.trim().slice(0, 100) || undefined,
		type: f.type || undefined,
		class: klasa || undefined,
		limit: pageSize,
		offset: (Math.max(1, page) - 1) * pageSize
	};
}

export function pickerPageCount(total: number, pageSize: number): number {
	return Math.max(1, Math.ceil(total / pageSize));
}

export type PageSelection = 'none' | 'some' | 'all';

/**
 * How much of the selectable part of a page is selected; drives the header
 * checkbox. Locked rows (e.g. already in the group) do not count.
 */
export function pageSelection(
	items: readonly RecipientLookup[],
	selected: ReadonlySet<string>,
	locked: ReadonlySet<string>
): PageSelection {
	const selectable = items.filter((r) => !locked.has(r.id));
	const picked = selectable.filter((r) => selected.has(r.id)).length;
	if (picked === 0) return 'none';
	return picked === selectable.length ? 'all' : 'some';
}
