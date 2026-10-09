// The user administration forms: reading them and turning backend errors
// into Polish messages.
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';
import type { Role } from '$lib/api/auth';

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
