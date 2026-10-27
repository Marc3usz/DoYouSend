// Batch endpoints of the delivery domain (domain B, MichalK252), see docs/api/openapi.yaml.
import { api } from './client';
import type { RecipientSelection } from './groups';
import type { Channel } from './recipients';

export type BatchStatus =
	'draft' | 'scheduled' | 'running' | 'done' | 'done_with_errors' | 'cancelled';

export type DeliveryStatus = 'pending' | 'sending' | 'sent' | 'delivered' | 'failed';

export type BatchRequest = {
	/** E-mail subject only. */
	subject: string;
	/** One body for both channels, sent byte for byte: never trim or reformat it. */
	body: string;
	selection: RecipientSelection;
};

export type BatchSummary = {
	id: string;
	subject: string;
	status: BatchStatus;
	createdBy: { id: string; fullName: string };
	createdAt: string;
	finishedAt: string | null;
	groups: { id: string; name: string }[];
	recipientCount: number;
	partialCount: number;
	failedCount: number;
	totalSmsParts: number;
	/** Thousandths of a złoty: 1000 = 1 zł. */
	costMilli: number;
};

export type BatchPage = { items: BatchSummary[]; total: number };

export type Delivery = {
	channel: Channel;
	status: DeliveryStatus;
	parts: number;
	attempts: number;
	error: string | null;
	updatedAt?: string;
};

export type BatchRecipient = {
	recipientId: string;
	firstName: string;
	lastName: string;
	/** The body this person got, identical on both channels; null when it could not render. */
	renderedBody: string | null;
	partial: boolean;
	unreachable: boolean;
	deliveries: Delivery[];
};

export type BatchDetail = BatchSummary & {
	body: string;
	recipientIds: string[];
	recipients: BatchRecipient[];
};

/**
 * POST /batches: the sender's final confirmation. idempotencyKey is a new UUID
 * for every opening of the confirmation step, so a double click or a retried
 * request returns the same batch instead of sending twice.
 */
export function sendBatch(
	request: BatchRequest,
	idempotencyKey: string,
	fetch?: typeof globalThis.fetch
): Promise<BatchDetail> {
	return api<BatchDetail>('/batches', {
		method: 'POST',
		body: request,
		headers: { 'Idempotency-Key': idempotencyKey },
		fetch
	});
}

export type BatchListQuery = { status?: BatchStatus; limit?: number; offset?: number };

/** GET /batches: history, newest first. Senders get their own batches only. */
export function listBatches(
	query: BatchListQuery,
	fetch?: typeof globalThis.fetch
): Promise<BatchPage> {
	const params = new URLSearchParams();
	for (const [key, value] of Object.entries(query)) {
		if (value !== undefined) params.set(key, String(value));
	}
	const search = params.size > 0 ? `?${params}` : '';
	return api<BatchPage>(`/batches${search}`, { fetch });
}

/** GET /batches/{id}: the status of both channels for every recipient. */
export function getBatch(id: string, fetch?: typeof globalThis.fetch): Promise<BatchDetail> {
	return api<BatchDetail>(`/batches/${encodeURIComponent(id)}`, { fetch });
}

/** POST /batches/{id}/retry: re-sends only failed deliveries of a batch. */
export function retryBatch(id: string, fetch?: typeof globalThis.fetch): Promise<BatchDetail> {
	return api<BatchDetail>(`/batches/${encodeURIComponent(id)}/retry`, {
		method: 'POST',
		fetch
	});
}
