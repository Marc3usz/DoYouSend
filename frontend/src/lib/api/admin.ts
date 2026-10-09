// Administration endpoints (iam domain, DEV D), see docs/api/openapi.yaml:
// users, the audit log, the sending setup and SMS usage.
import type { Role, User } from './auth';
import { api } from './client';

export type UserInput = { email: string; fullName: string; role: Role; password: string };
export type UserChange = { fullName?: string; role?: Role; disabled?: boolean };

/** GET /users: administrators first, then by name. */
export function listUsers(fetch?: typeof globalThis.fetch): Promise<User[]> {
	return api<User[]>('/users', { fetch });
}

export function createUser(input: UserInput, fetch?: typeof globalThis.fetch): Promise<User> {
	return api<User>('/users', { method: 'POST', body: input, fetch });
}

export function updateUser(
	id: string,
	change: UserChange,
	fetch?: typeof globalThis.fetch
): Promise<User> {
	return api<User>(`/users/${encodeURIComponent(id)}`, { method: 'PATCH', body: change, fetch });
}

/** PUT /users/{id}/password: sets a new password and ends the user's sessions. */
export function resetPassword(
	id: string,
	password: string,
	fetch?: typeof globalThis.fetch
): Promise<void> {
	return api<void>(`/users/${encodeURIComponent(id)}/password`, {
		method: 'PUT',
		body: { password },
		fetch
	});
}

export type AuditEntry = {
	id: number;
	userId: string | null;
	userName: string | null;
	action: string;
	entity: string | null;
	entityId: string | null;
	details: Record<string, string>;
	createdAt: string;
};

export type AuditPage = { items: AuditEntry[]; total: number };

export function listAudit(
	params: { limit: number; offset: number },
	fetch?: typeof globalThis.fetch
): Promise<AuditPage> {
	const q = new URLSearchParams({ limit: String(params.limit), offset: String(params.offset) });
	return api<AuditPage>(`/audit?${q}`, { fetch });
}

export type SendingConfig = {
	dryRun: boolean;
	email: {
		provider: string;
		from: string;
		sandbox: boolean;
		credentialsSet: boolean;
		eventsWebhook: boolean;
	};
	sms: {
		provider: string;
		senderName: string;
		testMode: boolean;
		credentialsSet: boolean;
		reportsWebhook: boolean;
		/** Thousandths of a złoty per SMS part. */
		pricePerPartMilli: number;
	};
};

export function sendingConfig(fetch?: typeof globalThis.fetch): Promise<SendingConfig> {
	return api<SendingConfig>('/admin/config', { fetch });
}

/** The SMSUsageStats schema (GET /stats/sms); costs in thousandths of a złoty. */
export type SmsUsage = {
	pricePerPartMilli: number;
	totalMessages: number;
	totalParts: number;
	plannedCostMilli: number;
	billedCostMilli: number;
	deliveredMessages: number;
	deliveredParts: number;
	deliveredCostMilli: number;
	sentMessages: number;
	sentParts: number;
	failedMessages: number;
	failedParts: number;
	inFlightMessages: number;
	inFlightParts: number;
};

/** GET /stats/sms for batches sent from `from` to `to` (YYYY-MM-DD, both inclusive). */
export function smsUsage(
	range: { from?: string; to?: string },
	fetch?: typeof globalThis.fetch
): Promise<SmsUsage> {
	const q = new URLSearchParams();
	if (range.from) q.set('from', range.from);
	if (range.to) q.set('to', range.to);
	const query = q.size > 0 ? `?${q}` : '';
	return api<SmsUsage>(`/stats/sms${query}`, { fetch });
}
