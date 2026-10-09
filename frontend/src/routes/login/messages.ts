// Polish texts for a failed login. The backend answers every wrong e-mail,
// password or disabled account the same way, so the text does not say which.
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';

export function loginFailureText(err: unknown): { status: number; text: string } {
	switch (errorBody(err)?.code) {
		case 'invalid_credentials':
			return {
				status: 401,
				text: 'Nieprawidłowy e-mail lub hasło, albo konto jest zablokowane.'
			};
		case 'too_many_attempts':
			return {
				status: 429,
				text: 'Zbyt wiele nieudanych prób. Odczekaj 15 minut albo poproś administratora o nowe hasło.'
			};
	}
	const status = err instanceof ApiError ? err.status : 503;
	return {
		status: status >= 500 ? 503 : status,
		text: 'Nie udało się zalogować — serwer nie odpowiada. Spróbuj ponownie za chwilę.'
	};
}
