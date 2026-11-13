// Administration endpoints (iam domain D, Marc3usz), see docs/api/openapi.yaml:
// users, the audit log, the sending setup, SMS usage and e-mail usage.
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

/** The SenderIdentity schema: a user's e-mail sender identity (ADR-0016). */
export type SenderIdentity = {
	userId: string;
	provider: string;
	fromEmail: string;
	fromName: string;
	status: 'pending' | 'verified' | 'missing';
	createdAt: string;
	checkedAt: string | null;
	verifiedAt: string | null;
};

/** The SenderIdentityList schema; `live` is false when no e-mail really goes out (DRY_RUN). */
export type SenderIdentityList = { provider: string; live: boolean; items: SenderIdentity[] };

/** GET /admin/senders: every identity as last checked, without asking the provider. */
export function listSenders(fetch?: typeof globalThis.fetch): Promise<SenderIdentityList> {
	return api<SenderIdentityList>('/admin/senders', { fetch });
}

function senderPath(userId: string): string {
	return `/users/${encodeURIComponent(userId)}/sender`;
}

/** POST /users/{id}/sender: the provider e-mails a confirmation link to the user's address. */
export function requestSender(
	userId: string,
	fetch?: typeof globalThis.fetch
): Promise<SenderIdentity> {
	return api<SenderIdentity>(senderPath(userId), { method: 'POST', fetch });
}

/** GET /users/{id}/sender: a pending identity is checked with the provider first. */
export function senderStatus(
	userId: string,
	fetch?: typeof globalThis.fetch
): Promise<SenderIdentity> {
	return api<SenderIdentity>(senderPath(userId), { fetch });
}

export function resendSenderVerification(
	userId: string,
	fetch?: typeof globalThis.fetch
): Promise<void> {
	return api<void>(`${senderPath(userId)}/resend`, { method: 'POST', fetch });
}

export function removeSender(userId: string, fetch?: typeof globalThis.fetch): Promise<void> {
	return api<void>(senderPath(userId), { method: 'DELETE', fetch });
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

/** A setting of the sending configuration (SettingItem, ADR-0018). */
export type SettingItem = {
	key: string;
	group: 'general' | 'email' | 'sms' | 'notion';
	kind: 'text' | 'bool' | 'enum' | 'secret' | 'price' | 'port' | 'address' | 'int';
	options?: string[];
	/** null for secrets: only `set` says whether one is stored. */
	value: string | null;
	set: boolean;
	source: 'panel' | 'env' | 'default';
	envVar: string;
	unreadable: boolean;
};

export type Settings = {
	dryRun: boolean;
	dryRunLocked: boolean;
	sendsExternally: boolean;
	items: SettingItem[];
};

/** GET /admin/settings. */
export function getSettings(fetch?: typeof globalThis.fetch): Promise<Settings> {
	return api<Settings>('/admin/settings', { fetch });
}

/** PATCH /admin/settings: null drops the panel value (back to .env or the default). */
export function updateSettings(
	values: Record<string, string | null>,
	confirmLiveSending: boolean,
	fetch?: typeof globalThis.fetch
): Promise<Settings> {
	return api<Settings>('/admin/settings', {
		method: 'PATCH',
		body: { values, confirmLiveSending },
		fetch
	});
}

export type TestMessageResult = { dryRun: boolean; providerMessageId: string };

/**
 * POST /admin/settings/test: one test message through the current setup. An
 * e-mail goes to the admin's own address unless `to` is given; SMS needs `to`.
 */
export function sendTestMessage(
	input: { channel: 'email' | 'sms'; to?: string },
	fetch?: typeof globalThis.fetch
): Promise<TestMessageResult> {
	return api<TestMessageResult>('/admin/settings/test', { method: 'POST', body: input, fetch });
}

export type SmsProfile = {
	points: number;
	name: string;
	userType: string;
};

/** GET /admin/providers/sms/profile: fetches live points balance and account info from SMSAPI. */
export function smsProfile(fetch?: typeof globalThis.fetch): Promise<SmsProfile> {
	return api<SmsProfile>('/admin/providers/sms/profile', { fetch });
}

export type EmailProviderProfile = {
	provider: string;
	remainCredits?: number;
	totalCredits?: number;
	usedCredits?: number;
	resetFrequency?: string;
	nextReset?: string;
};

/** GET /admin/providers/email/profile: fetches live credits and profile from SendGrid or sandbox info from Mailpit. */
export function emailProfile(fetch?: typeof globalThis.fetch): Promise<EmailProviderProfile> {
	return api<EmailProviderProfile>('/admin/providers/email/profile', { fetch });
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
	/** "Sent" under DRY_RUN, by the fake gateway or in SMSAPI test mode: never billed. */
	simulatedMessages?: number;
	simulatedParts?: number;
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

/** The EmailUsageStats schema (GET /stats/email). */
export type EmailUsage = {
	totalMessages: number;
	deliveredMessages: number;
	sentMessages: number;
	failedMessages: number;
	inFlightMessages: number;
};

/** GET /stats/email for batches sent from `from` to `to` (YYYY-MM-DD, both inclusive). */
export function emailUsage(
	range: { from?: string; to?: string },
	fetch?: typeof globalThis.fetch
): Promise<EmailUsage> {
	const q = new URLSearchParams();
	if (range.from) q.set('from', range.from);
	if (range.to) q.set('to', range.to);
	const query = q.size > 0 ? `?${q}` : '';
	return api<EmailUsage>(`/stats/email${query}`, { fetch });
}
