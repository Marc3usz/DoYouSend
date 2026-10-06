// Polish texts of the recipients screens. The backend sends stable keys
// (field, code, channel, reason); the English message is never shown alone.
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';
import type { ContactIssue } from '$lib/api/recipients';

export { recipientTypeLabel } from './import/messages';

const issueTexts: Record<ContactIssue['channel'], Record<ContactIssue['reason'], string>> = {
	email: { missing: 'Brak e-maila', invalid: 'Błędny e-mail' },
	sms: { missing: 'Brak telefonu', invalid: 'Błędny numer' }
};

export function issueText(issue: ContactIssue): string {
	return issueTexts[issue.channel][issue.reason];
}

/** Explains on the list what an issue means for sending. */
export function issueHint(issue: ContactIssue): string {
	const channel = issue.channel === 'email' ? 'e-mail' : 'SMS';
	return `Ta osoba nie dostanie wiadomości kanałem ${channel}, dopóki dane nie zostaną poprawione.`;
}

export const errorTexts = {
	unavailable:
		'Nie udało się połączyć z API. Sprawdź, czy backend działa (make dev-api) i ma ustawione DATABASE_URL.',
	unexpected: 'Coś poszło nie tak. Spróbuj ponownie za chwilę.',
	notFound: 'Nie ma takiego odbiorcy — mógł zostać usunięty.',
	inUse:
		'Tego odbiorcy nie można usunąć, bo występuje w historii wysyłek. Historia musi pokazywać, do kogo poszła wiadomość.'
} as const;

/**
 * The text for a failed recipients request. A network error, or an error
 * without an ErrorBody (the API started without DATABASE_URL answers a plain
 * 404 for these routes), means the API is not usable rather than a missing
 * recipient.
 */
export function failureText(err: unknown): string {
	if (!(err instanceof ApiError)) return errorTexts.unavailable;
	const body = errorBody(err);
	switch (body?.code) {
		case undefined:
			return err.status >= 500 || err.status === 404
				? errorTexts.unavailable
				: errorTexts.unexpected;
		case 'not_found':
			return errorTexts.notFound;
		case 'recipient_in_use':
			return errorTexts.inUse;
		default:
			return errorTexts.unexpected;
	}
}

/** Whether err is the backend's not_found answer for one recipient. */
export function isNotFound(err: unknown): boolean {
	return errorBody(err)?.code === 'not_found';
}
