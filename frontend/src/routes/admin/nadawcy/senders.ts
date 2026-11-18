// Polish texts of the sender administration: domains authenticated at the
// e-mail provider and addresses delegated to chosen users.
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';
import type { DnsRecord, SenderDomain } from '$lib/api/senders';

export const domainStatusLabels: Record<SenderDomain['status'], string> = {
	pending: 'czeka na rekordy DNS',
	valid: 'uwierzytelniona',
	missing: 'usunięta u operatora'
};

export const dnsTypeLabels: Record<DnsRecord['type'], string> = {
	cname: 'CNAME',
	txt: 'TXT',
	mx: 'MX'
};

export type ActionFailure = { status: number; failure: string; fields: Record<string, string> };

function generic(err: unknown, what: string): ActionFailure {
	const status = err instanceof ApiError && err.status < 500 ? err.status : 500;
	return { status, fields: {}, failure: `Nie udało się ${what}. Spróbuj ponownie za chwilę.` };
}

/** A failed domain action (add, check DNS, remove). */
export function domainFailureOf(err: unknown, what = 'zapisać domeny'): ActionFailure {
	const body = errorBody(err);
	switch (body?.code) {
		case 'invalid_input':
			return {
				status: 400,
				fields: { domain: 'Wpisz samą domenę, np. szkola.example.test — bez „@” i „https://”.' },
				failure: ''
			};
		case 'domain_exists':
			return {
				status: 409,
				fields: { domain: 'Ta domena jest już dodana.' },
				failure: ''
			};
		case 'provider_rejected':
			// The provider's own text is the only useful detail here.
			return { status: 502, fields: {}, failure: `SendGrid odrzucił żądanie: ${body.message}` };
		case 'provider_error':
			return {
				status: 502,
				fields: {},
				failure: 'SendGrid nie odpowiedział poprawnie. Spróbuj ponownie za chwilę.'
			};
		case 'not_found':
			return { status: 404, fields: {}, failure: 'Tej domeny już nie ma — odśwież stronę.' };
	}
	return generic(err, what);
}

const addressFieldTexts: Record<string, string> = {
	email: 'Wpisz poprawny adres e-mail, np. sekretariat@example.test.',
	name: 'Wpisz nazwę nadawcy (do 100 znaków), np. Sekretariat szkoły.',
	userIds: 'Część wybranych osób nie ma już aktywnego konta — odśwież stronę.'
};

/** A failed delegated-address action (add, change, remove). */
export function addressFailureOf(err: unknown, what = 'zapisać adresu'): ActionFailure {
	const body = errorBody(err);
	switch (body?.code) {
		case 'invalid_input': {
			const fields: Record<string, string> = {};
			for (const f of body.fields) {
				fields[f.field] = addressFieldTexts[f.field] ?? 'Niepoprawna wartość.';
			}
			return { status: 400, fields, failure: '' };
		}
		case 'address_exists':
			return { status: 409, fields: { email: 'Ten adres jest już dodany.' }, failure: '' };
		case 'not_found':
			return { status: 404, fields: {}, failure: 'Tego adresu już nie ma — odśwież stronę.' };
	}
	return generic(err, what);
}

/** "Jan, Anna i 3 inne osoby" style list of delegated users, or a dash. */
export function usersText(users: { fullName: string }[]): string {
	if (users.length === 0) return 'nikt';
	return users.map((u) => u.fullName).join(', ');
}

/** The checked user IDs of a delegation form. */
export function readUserIds(data: FormData): string[] {
	return [
		...new Set(data.getAll('userIds').filter((v): v is string => typeof v === 'string' && v !== ''))
	];
}
