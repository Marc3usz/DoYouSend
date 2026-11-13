// Polish texts about a recipient shared by the recipient, group and composer pages.
import type { ContactIssue, RecipientType } from '$lib/api/recipients';

export function recipientTypeLabel(type: RecipientType): string {
	return type === 'parent' ? 'rodzic' : 'uczeń';
}

const issueTexts: Record<ContactIssue['channel'], Record<ContactIssue['reason'], string>> = {
	email: { missing: 'Brak e-maila', invalid: 'Błędny e-mail' },
	sms: { missing: 'Brak telefonu', invalid: 'Błędny numer' }
};

export function issueText(issue: ContactIssue): string {
	return issueTexts[issue.channel][issue.reason];
}

/** Explains on a list what an issue means for sending. */
export function issueHint(issue: ContactIssue): string {
	const channel = issue.channel === 'email' ? 'e-mail' : 'SMS';
	return `Ta osoba nie dostanie wiadomości kanałem ${channel}, dopóki dane nie zostaną poprawione.`;
}

/** Texts for a request that failed without a specific reason, shared by every domain. */
export const commonErrorTexts = {
	unavailable: 'Serwer nie odpowiada. Spróbuj ponownie za chwilę.',
	unexpected: 'Coś poszło nie tak. Spróbuj ponownie za chwilę.'
} as const;
