// Polish texts for the composer's pre-send summary. Every number comes from the
// backend (POST /messages/preview); nothing here counts characters or SMS parts.
import { ApiError } from '$lib/api/client';
import type { SendEstimate } from '$lib/api/messages';

const MILLI_PER_ZLOTY = 1000;

const pln = new Intl.NumberFormat('pl-PL', {
	style: 'currency',
	currency: 'PLN',
	minimumFractionDigits: 2,
	maximumFractionDigits: 3,
	useGrouping: false
});

/** Formats a cost given in thousandths of a złoty (80 -> "0,08 zł"). */
export function costText(costMilli: number): string {
	return pln.format(costMilli / MILLI_PER_ZLOTY);
}

/** Picks the Polish plural form: 1 SMS, 2 SMS-y, 5 SMS-ów, 22 SMS-y. */
export function plural(n: number, one: string, few: string, many: string): string {
	if (n === 1) return one;
	const lastDigit = n % 10;
	const lastTwo = n % 100;
	return lastDigit >= 2 && lastDigit <= 4 && (lastTwo < 12 || lastTwo > 14) ? few : many;
}

export const smsText = (n: number) => `${n} ${plural(n, 'SMS', 'SMS-y', 'SMS-ów')}`;

export const recipientsText = (n: number) =>
	`${n} ${plural(n, 'odbiorca', 'odbiorcy', 'odbiorców')}`;

// Verbs agree with the counted noun: 1 odbiorca ma, 2 odbiorcy mają, 5 odbiorców ma.
const lacks = (n: number) => plural(n, 'nie ma', 'nie mają', 'nie ma');
const gets = (n: number) => plural(n, 'dostanie', 'dostaną', 'dostanie');

/** SMS parts per recipient: one number, or a range when personalisation changes length. */
export function partsText(min: number, max: number): string {
	return min === max ? `${min}` : `${min}–${max}`;
}

const placeholderList = (names: string[]) => names.map((n) => `{{${n}}}`).join(', ');

/** Problems the sender should see before confirming, most serious first. */
export function estimateWarnings(est: SendEstimate): string[] {
	const warnings: string[] = [];
	if (est.unknownPlaceholders.length > 0) {
		const label = est.unknownPlaceholders.length === 1 ? 'Nieznane pole' : 'Nieznane pola';
		warnings.push(
			`${label} ${placeholderList(est.unknownPlaceholders)} — dostępne są tylko {{imie}} i {{nazwisko}}. Z takimi polami wiadomość nie zostanie wysłana.`
		);
	}
	const failed = est.renderFailedIds.length;
	if (failed > 0) {
		warnings.push(
			`${recipientsText(failed)} ${lacks(failed)} wartości dla użytego pola (np. pustego imienia) — nie ${gets(failed)} wiadomości.`
		);
	}
	if (est.unreachableCount > 0) {
		const n = est.unreachableCount;
		warnings.push(
			`${recipientsText(n)} ${lacks(n)} ani e-maila, ani telefonu — nie ${gets(n)} wiadomości.`
		);
	}
	if (est.partialCount > 0) {
		const n = est.partialCount;
		warnings.push(
			`${recipientsText(n)} ${gets(n)} wiadomość tylko jednym kanałem (brak e-maila albo telefonu).`
		);
	}
	return warnings;
}

/** Text for a preview request that failed as a whole. */
export function previewFailureText(err: unknown): string {
	if (err instanceof ApiError && err.status === 400) {
		return 'Serwer odrzucił wybór odbiorców. Odśwież stronę i zaznacz grupy ponownie.';
	}
	return 'Nie udało się policzyć podsumowania. Spróbuj ponownie za chwilę.';
}
