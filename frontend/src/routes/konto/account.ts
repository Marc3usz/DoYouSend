// The account page: changing one's own password.
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';

/** The backend's minimum; checked here too so a typo is caught before the request. */
export const MIN_PASSWORD_LENGTH = 12;

export type PasswordFields = Partial<Record<'currentPassword' | 'newPassword' | 'repeat', string>>;

/** Problems the form can see without asking the backend. */
export function checkPasswordForm(input: {
	currentPassword: string;
	newPassword: string;
	repeat: string;
}): PasswordFields {
	const fields: PasswordFields = {};
	if (input.currentPassword === '') fields.currentPassword = 'Wpisz obecne hasło.';
	if (input.newPassword.length < MIN_PASSWORD_LENGTH) {
		fields.newPassword = `Nowe hasło musi mieć co najmniej ${MIN_PASSWORD_LENGTH} znaków.`;
	} else if (input.newPassword === input.currentPassword) {
		fields.newPassword = 'Nowe hasło musi się różnić od obecnego.';
	}
	if (input.repeat !== input.newPassword) fields.repeat = 'Hasła nie są takie same.';
	return fields;
}

/** A rejected password change as Polish field messages or one general message. */
export function passwordFailureOf(err: unknown): {
	status: number;
	fields: PasswordFields;
	failure: string | null;
} {
	const body = errorBody(err);
	switch (body?.code) {
		case 'wrong_password':
			return {
				status: 403,
				fields: { currentPassword: 'Obecne hasło jest nieprawidłowe.' },
				failure: null
			};
		case 'invalid_input':
			return {
				status: 400,
				fields: {
					newPassword: `Nowe hasło musi mieć od ${MIN_PASSWORD_LENGTH} do 128 znaków.`
				},
				failure: null
			};
		case 'too_many_attempts':
			return {
				status: 429,
				fields: {},
				failure: 'Za dużo nieudanych prób. Odczekaj chwilę i spróbuj ponownie.'
			};
		case 'unauthenticated':
			return { status: 401, fields: {}, failure: 'Sesja wygasła. Zaloguj się ponownie.' };
	}
	const status = err instanceof ApiError && err.status < 500 ? err.status : 500;
	return {
		status,
		fields: {},
		failure: 'Nie udało się zmienić hasła. Spróbuj ponownie za chwilę.'
	};
}
