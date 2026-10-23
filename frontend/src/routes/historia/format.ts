// Polish texts for the batch history. Every number and status comes from the
// backend (GET /batches, GET /batches/{id}).
import type { Channel } from '$lib/api/recipients';
import type { BatchRecipient, BatchStatus, Delivery, DeliveryStatus } from '$lib/api/batches';

export const batchStatusLabels: Record<BatchStatus, string> = {
	draft: 'Szkic',
	scheduled: 'Zaplanowana',
	running: 'W trakcie',
	done: 'Wysłano',
	done_with_errors: 'Wysłano z błędami',
	cancelled: 'Anulowana'
};

export const deliveryStatusLabels: Record<DeliveryStatus, string> = {
	pending: 'Czeka',
	sending: 'Wysyłanie',
	sent: 'Wysłano',
	delivered: 'Doręczono',
	failed: 'Błąd'
};

/** Statuses the history can be filtered by: the ones the API actually sets. */
export const filterStatuses: BatchStatus[] = ['running', 'done', 'done_with_errors'];

export function isBatchStatus(value: string | null): value is BatchStatus {
	return value !== null && Object.hasOwn(batchStatusLabels, value);
}

const dateTime = new Intl.DateTimeFormat('pl-PL', {
	dateStyle: 'medium',
	timeStyle: 'short',
	timeZone: 'Europe/Warsaw'
});

export function dateTimeText(iso: string | null): string {
	return iso ? dateTime.format(new Date(iso)) : '—';
}

/** The delivery of one channel, or undefined when the batch did not use it for the person. */
export function deliveryOf(r: BatchRecipient, channel: Channel): Delivery | undefined {
	return r.deliveries.find((d) => d.channel === channel);
}

/** One table cell: the status, the SMS parts and the classified reason of a failure. */
export function deliveryText(d: Delivery | undefined): string {
	if (!d) return 'brak kanału';
	const label = deliveryStatusLabels[d.status];
	const parts = d.channel === 'sms' && d.parts > 1 ? ` (${d.parts} części)` : '';
	const reason = d.status === 'failed' && d.error ? `: ${d.error}` : '';
	return `${label}${parts}${reason}`;
}

/** Whether a history page should keep refreshing: something is still being sent. */
export function stillSending(status: BatchStatus): boolean {
	return status === 'running';
}

/** Page number from ?strona=, 1 for anything that is not a positive integer. */
export function pageParam(raw: string | null): number {
	const n = Number(raw ?? '1');
	return Number.isInteger(n) && n > 0 ? n : 1;
}
