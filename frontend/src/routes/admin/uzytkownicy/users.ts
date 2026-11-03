// The user administration forms: reading them and turning backend errors
// into Polish messages.
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';
import type { Role } from '$lib/api/auth';
import type { SenderIdentity } from '$lib/api/admin';

export type NewUserValues = { email: string; fullName: string; role: string };

export function readText(data: FormData, name: string): string {
	const value = data.get(name);
	return typeof value === 'string' ? value : '';
}

export function asRole(value: string): Role | undefined {
	return value === 'admin' || value === 'sender' ? value : undefined;
}

const fieldTexts: Record<string, string> = {
	email: 'Wpisz poprawny adres e-mail.',
	fullName: 'Wpisz imię i nazwisko (najwyżej 200 znaków).',
	role: 'Wybierz rolę.',
	password: 'Hasło musi mieć od 12 do 128 znaków.'
};

/** Per-field messages and a general one for a rejected save. */
export function failureOf(err: unknown): {
	status: number;
	fields: Record<string, string>;
	failure: string | null;
} {
	const body = errorBody(err);
	switch (body?.code) {
		case 'invalid_input': {
			const fields: Record<string, string> = {};
			for (const f of body.fields) fields[f.field] = fieldTexts[f.field] ?? 'Niepoprawna wartość.';
			return { status: 400, fields, failure: null };
		}
		case 'email_taken':
			return { status: 409, fields: { email: 'Ten adres ma już inne konto.' }, failure: null };
		case 'last_admin':
			return {
				status: 409,
				fields: {},
				failure:
					'To ostatni aktywny administrator — najpierw nadaj rolę administratora komuś innemu.'
			};
		case 'self_lockout':
			return {
				status: 409,
				fields: {},
				failure: 'Nie możesz zablokować własnego konta ani odebrać sobie roli administratora.'
			};
		case 'not_found':
			return { status: 404, fields: {}, failure: 'Tego konta już nie ma — odśwież stronę.' };
	}
	const status = err instanceof ApiError && err.status < 500 ? err.status : 500;
	return {
		status,
		fields: {},
		failure: 'Nie udało się zapisać zmian. Spróbuj ponownie za chwilę.'
	};
}

/** Polish labels of SenderIdentity.status (ADR-0016). */
export const senderStatusLabels: Record<SenderIdentity['status'], string> = {
	pending: 'czeka na potwierdzenie',
	verified: 'potwierdzony',
	missing: 'usunięty w SendGrid'
};

/** A Polish message for a failed sender-identity action. */
export function senderFailureOf(err: unknown): { status: number; failure: string } {
	const body = errorBody(err);
	switch (body?.code) {
		case 'sender_rejected':
			// The provider's own text is the only useful detail here (e.g. "already exists").
			return { status: 422, failure: `SendGrid odrzucił zgłoszenie: ${body.message}` };
		case 'sender_exists':
			return { status: 409, failure: 'To konto ma już nadawcę — odśwież stronę.' };
		case 'sender_already_verified':
			return { status: 409, failure: 'Adres jest już potwierdzony.' };
		case 'sender_missing':
			return {
				status: 409,
				failure: 'SendGrid nie zna już tego nadawcy. Usuń go i zgłoś ponownie.'
			};
		case 'sender_not_configured':
			return { status: 404, failure: 'To konto nie ma nadawcy — odśwież stronę.' };
		case 'invalid_input':
			return { status: 400, failure: 'Nie można zgłosić nadawcy dla zablokowanego konta.' };
		case 'not_found':
			return { status: 404, failure: 'Tego konta już nie ma — odśwież stronę.' };
		case 'provider_error':
			return {
				status: 502,
				failure: 'SendGrid nie odpowiedział poprawnie. Spróbuj ponownie za chwilę.'
			};
	}
	const status = err instanceof ApiError && err.status < 500 ? err.status : 500;
	return { status, failure: 'Nie udało się wykonać operacji. Spróbuj ponownie za chwilę.' };
}
