// Data-quality view of the recipient base: how many people can be reached on
// each channel, and the merged list of people whose contact data needs fixing.
// Pure functions, so the numbers on screen are testable without the API.
import type { Channel, Recipient, RecipientCounts } from '$lib/api/recipients';

export type ChannelCoverage = {
	channel: Channel;
	label: string;
	reachable: number;
	missing: number;
	/** Whole percent, 100 for an empty base so an empty bar does not look like an alarm. */
	percent: number;
	/** Link to the recipient list filtered to this problem. */
	href: string;
};

function percentOf(part: number, total: number): number {
	if (total <= 0 || part >= total) return 100;
	// Never show 100% while somebody is still missing.
	return Math.min(99, Math.max(0, Math.round((part / total) * 100)));
}

/** Per-channel coverage. A count above the total (a race between requests) is clamped. */
export function coverage(counts: RecipientCounts): ChannelCoverage[] {
	const row = (channel: Channel, label: string, missing: number): ChannelCoverage => {
		const bad = Math.min(Math.max(missing, 0), Math.max(counts.total, 0));
		return {
			channel,
			label,
			reachable: counts.total - bad,
			missing: bad,
			percent: percentOf(counts.total - bad, counts.total),
			href: `/odbiorcy?issue=${channel}`
		};
	};
	return [row('email', 'E-mail', counts.noEmail), row('sms', 'SMS', counts.noPhone)];
}

/** One person on the fix-it list, with every channel that needs attention. */
export type FixItem = {
	recipient: Recipient;
	channels: Channel[];
};

/**
 * Merges the "no e-mail" and "no phone" lists into one list. Someone in both
 * lists appears once (the same person must not be listed twice). Those with
 * every channel broken come first, then by name.
 */
export function mergeFixItems(noEmail: Recipient[], noPhone: Recipient[]): FixItem[] {
	const byId = new Map<string, FixItem>();
	// The channels come from the person's own issues, not from which (possibly
	// cut-off) list they were found in; the source list is only a fallback.
	const add = (r: Recipient, source: Channel) => {
		const own = r.issues.map((i) => i.channel);
		const channels = own.length > 0 ? own : [source];
		const existing = byId.get(r.id);
		if (existing) {
			for (const c of channels) if (!existing.channels.includes(c)) existing.channels.push(c);
			return;
		}
		byId.set(r.id, { recipient: r, channels: [...new Set(channels)] });
	};
	for (const r of noEmail) add(r, 'email');
	for (const r of noPhone) add(r, 'sms');
	// Both channels broken first: those people cannot be reached at all.
	return [...byId.values()].sort(
		(a, b) =>
			b.channels.length - a.channels.length ||
			a.recipient.lastName.localeCompare(b.recipient.lastName, 'pl') ||
			a.recipient.firstName.localeCompare(b.recipient.firstName, 'pl') ||
			a.recipient.id.localeCompare(b.recipient.id)
	);
}

/** People who cannot get the message on any channel. */
export function unreachableCount(items: FixItem[]): number {
	return items.filter((i) => i.channels.length > 1).length;
}

/**
 * Whether a list is cut off: the API returned fewer rows than it counted, so
 * the fix-it list must say it is partial instead of looking complete.
 */
export function isTruncated(shown: number, total: number): boolean {
	return total > shown;
}

/** "1 osoba", "2 osoby", "5 osób": Polish plural for a count of people. */
export function peopleLabel(n: number): string {
	const abs = Math.abs(n);
	if (abs === 1) return `${n} osoba`;
	const last = abs % 10;
	const lastTwo = abs % 100;
	if (last >= 2 && last <= 4 && !(lastTwo >= 12 && lastTwo <= 14)) return `${n} osoby`;
	return `${n} osób`;
}

export type FixView = 'all' | 'email' | 'sms' | 'both';

/** Reads ?pokaz=; unknown values mean "all". */
export function parseFixView(raw: string | null): FixView {
	return raw === 'email' || raw === 'sms' || raw === 'both' ? raw : 'all';
}

/** The fix-it list narrowed to one problem; "both" means unreachable on every channel. */
export function filterFixItems(items: FixItem[], view: FixView): FixItem[] {
	switch (view) {
		case 'email':
			return items.filter((i) => i.channels.includes('email'));
		case 'sms':
			return items.filter((i) => i.channels.includes('sms'));
		case 'both':
			return items.filter((i) => i.channels.length > 1);
		default:
			return items;
	}
}
