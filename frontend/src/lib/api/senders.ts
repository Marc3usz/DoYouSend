// E-mail sender addresses: what a user may send from (GET /me/senders) and the
// admin's domains and delegated addresses. See docs/api/openapi.yaml.
import { api } from './client';

type Fetch = typeof globalThis.fetch;

export type SenderKind = 'shared' | 'own' | 'delegated';

export type SenderUnusableReason = 'domain_not_authenticated' | 'not_verified';

/** One address a user may pick as the e-mail From. */
export type SenderOption = {
	/** "shared", "own" or the UUID of a delegated address. */
	id: string;
	email: string;
	name: string;
	kind: SenderKind;
	usable: boolean;
	reason: SenderUnusableReason | null;
};

export type SenderOptions = { defaultId: string; items: SenderOption[] };

/** GET /me/senders: the addresses the logged-in user may send from. */
export function mySenders(fetch?: Fetch): Promise<SenderOptions> {
	return api<SenderOptions>('/me/senders', { fetch });
}

export type DnsRecord = {
	type: 'cname' | 'txt' | 'mx';
	host: string;
	data: string;
	valid: boolean;
};

/** A domain authenticated at the e-mail provider (SendGrid domain authentication). */
export type SenderDomain = {
	id: string;
	domain: string;
	status: 'pending' | 'valid' | 'missing';
	dnsRecords: DnsRecord[];
	createdAt: string;
	checkedAt: string | null;
	validatedAt: string | null;
};

/** `live` is false in test mode: the records are simulated, nothing reaches the provider. */
export type SenderDomainList = { live: boolean; items: SenderDomain[] };

const domainPath = (id: string) => `/admin/sender-domains/${encodeURIComponent(id)}`;

export function listSenderDomains(fetch?: Fetch): Promise<SenderDomainList> {
	return api<SenderDomainList>('/admin/sender-domains', { fetch });
}

export function addSenderDomain(domain: string, fetch?: Fetch): Promise<SenderDomain> {
	return api<SenderDomain>('/admin/sender-domains', { method: 'POST', body: { domain }, fetch });
}

/** Asks the provider to check the DNS records again. */
export function validateSenderDomain(id: string, fetch?: Fetch): Promise<SenderDomain> {
	return api<SenderDomain>(`${domainPath(id)}/validate`, { method: 'POST', fetch });
}

export function removeSenderDomain(id: string, fetch?: Fetch): Promise<void> {
	return api<void>(domainPath(id), { method: 'DELETE', fetch });
}

/** A shared address the admin delegates to chosen users. */
export type SenderAddress = {
	id: string;
	email: string;
	name: string;
	users: { id: string; fullName: string }[];
	usable: boolean;
	reason: SenderUnusableReason | string | null;
	createdAt: string;
};

export type SenderAddressInput = { email: string; name: string; userIds?: string[] };

const addressPath = (id: string) => `/admin/sender-addresses/${encodeURIComponent(id)}`;

export function listSenderAddresses(fetch?: Fetch): Promise<{ items: SenderAddress[] }> {
	return api<{ items: SenderAddress[] }>('/admin/sender-addresses', { fetch });
}

export function addSenderAddress(input: SenderAddressInput, fetch?: Fetch): Promise<SenderAddress> {
	return api<SenderAddress>('/admin/sender-addresses', { method: 'POST', body: input, fetch });
}

export function updateSenderAddress(
	id: string,
	change: { name?: string; userIds?: string[] },
	fetch?: Fetch
): Promise<SenderAddress> {
	return api<SenderAddress>(addressPath(id), { method: 'PATCH', body: change, fetch });
}

export function removeSenderAddress(id: string, fetch?: Fetch): Promise<void> {
	return api<void>(addressPath(id), { method: 'DELETE', fetch });
}
