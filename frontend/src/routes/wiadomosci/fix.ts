import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';
import type { Channel } from '$lib/api/recipients';

export type ContactNeeds = {
	needsEmail: boolean;
	needsPhone: boolean;
};

/** Reports which channels are missing or broken for a recipient. */
export function missingChannels(channels: Channel[]): ContactNeeds {
	return {
		needsEmail: !channels.includes('email'),
		needsPhone: !channels.includes('sms')
	};
}

/** Pre-validates user inputs before sending a PATCH /recipients/{id}/contact request. */
export function validateContactFixInput(
	input: { email?: string; phone?: string },
	needs: ContactNeeds
): string | null {
	const email = input.email?.trim() ?? '';
	const phone = input.phone?.trim() ?? '';

	if (needs.needsEmail && needs.needsPhone) {
		if (!email && !phone) {
			return 'Podaj adres e-mail lub numer telefonu do uzupełnienia.';
		}
	} else if (needs.needsEmail && !email) {
		return 'Wpisz brakujący adres e-mail.';
	} else if (needs.needsPhone && !phone) {
		return 'Wpisz brakujący numer telefonu.';
	}

	return null;
}

/** Formats an error from PATCH /recipients/{id}/contact into Polish user-facing text. */
export function contactFixErrorText(err: unknown): string {
	if (!(err instanceof ApiError)) {
		return 'Nie udało się zapisać danych kontaktowych. Sprawdź połączenie z serwerem.';
	}
	const body = errorBody(err);
	switch (body?.code) {
		case 'duplicate_contact':
			return 'Ten adres e-mail lub numer telefonu jest już przypisany do innego odbiorcy.';
		case 'contact_not_fixable':
			return 'Te dane kontaktowe zostały już uzupełnione lub nie wymagają poprawy.';
		case 'invalid_input': {
			const hasEmail = body.fields.some((f) => f.field === 'email');
			const hasPhone = body.fields.some((f) => f.field === 'phone');
			if (hasEmail && hasPhone) {
				return 'Podaj poprawny adres e-mail i numer telefonu.';
			}
			if (hasEmail) {
				return 'Podaj poprawny adres e-mail (np. jan@example.test).';
			}
			if (hasPhone) {
				return 'Podaj poprawny numer telefonu (np. +48 500 100 101).';
			}
			return 'Wprowadzone dane kontaktowe są nieprawidłowe.';
		}
		case 'invalid_request':
			return 'Wprowadź dane do uzupełnienia.';
		case 'not_found':
			return 'Odbiorca nie został znaleziony (mógł zostać usunięty).';
		default:
			return 'Nie udało się zapisać danych kontaktowych. Spróbuj ponownie za chwilę.';
	}
}
