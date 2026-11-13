// Polish texts for the composer's pre-send summary. Every number comes from the
// backend (POST /messages/preview); nothing here counts characters or SMS parts.
import { ApiError } from '$lib/api/client';
import type { SendEstimate } from '$lib/api/messages';
import { plural, recipientsText } from '$lib/format/text';

export { costText, plural, recipientsText, smsText } from '$lib/format/text';

// Verbs agree with the counted noun: 1 odbiorca ma, 2 odbiorcy mają, 5 odbiorców ma.
const lacks = (n: number) => plural(n, 'nie ma', 'nie mają', 'nie ma');
const gets = (n: number) => plural(n, 'dostanie', 'dostaną', 'dostanie');

/** SMS parts per recipient: one number, or a range when personalisation changes length. */
export function partsText(min: number, max: number): string {
	return min === max ? `${min}` : `${min}–${max}`;
}

const placeholderList = (names: string[]) => names.map((n) => `{{${n}}}`).join(', ');

/** What an unreachable recipient lacks for the chosen channels. */
function missingForChannels(channels: readonly ('email' | 'sms')[]): string {
	if (channels.length === 1) {
		return channels[0] === 'email' ? 'działającego e-maila' : 'działającego numeru telefonu';
	}
	return 'ani e-maila, ani telefonu';
}

/**
 * Problems the sender should see before confirming, most serious first.
 * channels are the ones chosen in the composer (both by default).
 */
export function estimateWarnings(
	est: SendEstimate,
	channels: readonly ('email' | 'sms')[] = ['email', 'sms']
): string[] {
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
			`${recipientsText(n)} ${lacks(n)} ${missingForChannels(channels)} — nie ${gets(n)} wiadomości.`
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
		return 'Serwer odrzucił wybór odbiorców. Odśwież stronę i wybierz odbiorców ponownie.';
	}
	return 'Nie udało się policzyć podsumowania. Spróbuj ponownie za chwilę.';
}
