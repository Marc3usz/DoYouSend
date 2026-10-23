// Polish texts for the composer's final confirmation (POST /batches).
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';
import type { SendEstimate } from '$lib/api/messages';
import { plural, smsText } from './estimate';

/**
 * A new UUID v4 for the Idempotency-Key. crypto.randomUUID exists only in secure
 * contexts (HTTPS, localhost); a school opening the app over plain HTTP on its
 * network still has crypto.getRandomValues.
 */
export function newIdempotencyKey(
	c: Pick<Crypto, 'getRandomValues'> & Partial<Crypto> = crypto
): string {
	if (typeof c.randomUUID === 'function') return c.randomUUID();
	const b = c.getRandomValues(new Uint8Array(16));
	b[6] = ((b[6] ?? 0) & 0x0f) | 0x40;
	b[8] = ((b[8] ?? 0) & 0x3f) | 0x80;
	const hex = Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
	return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

/** Whether the draft may be confirmed: the backend would refuse it otherwise. */
export function canConfirm(subject: string, est: SendEstimate | null, counting: boolean): boolean {
	return (
		!counting &&
		subject.trim() !== '' &&
		est !== null &&
		est.recipientCount > 0 &&
		est.template.parts > 0 &&
		est.unknownPlaceholders.length === 0
	);
}

/** One sentence the sender confirms: who gets what over which channel. */
export function confirmText(est: SendEstimate): string {
	const emails = `${est.emailCount} ${plural(est.emailCount, 'e-mail', 'e-maile', 'e-maili')}`;
	const sms = est.smsCount > 0 ? ` i ${smsText(est.totalSmsParts)}` : '';
	// "do" takes the genitive: do 1 odbiorcy, do 3 odbiorców.
	const n = est.recipientCount;
	return `Wyślesz ${emails}${sms} do ${n} ${n === 1 ? 'odbiorcy' : 'odbiorców'}.`;
}

/** Text for a confirmation the backend refused or that did not reach it. */
export function sendFailureText(err: unknown): string {
	const body = errorBody(err);
	switch (body?.code) {
		case 'invalid_input':
			if (body.fields.some((f) => f.field === 'subject')) return 'Uzupełnij temat e-maila.';
			if (body.fields.some((f) => f.field === 'body')) {
				return 'Treść jest pusta albo zawiera nieznane pole — dostępne są tylko {{imie}} i {{nazwisko}}.';
			}
			return 'Serwer odrzucił wysyłkę. Odśwież stronę i spróbuj ponownie.';
		case 'empty_selection':
			return 'Zaznacz co najmniej jedną grupę.';
		case 'selection_changed':
			return 'Część zaznaczonych grup lub osób już nie istnieje. Odśwież stronę i wybierz odbiorców ponownie. Nic nie zostało wysłane.';
		case 'no_recipients':
			return 'Zaznaczone grupy nie mają teraz żadnych odbiorców. Nic nie zostało wysłane.';
		case 'idempotency_conflict':
			return 'To potwierdzenie było już użyte dla innej treści. Wróć do edycji i zatwierdź wysyłkę ponownie.';
		case 'unauthenticated':
			return 'Sesja wygasła. Zaloguj się ponownie — nic nie zostało wysłane.';
	}
	if (err instanceof ApiError && err.status === 503) {
		return 'Serwer jest właśnie restartowany. Spróbuj ponownie za chwilę — nic nie zostało wysłane.';
	}
	// The request may or may not have reached the backend; the same Idempotency-Key
	// makes a retry safe either way.
	return 'Nie udało się potwierdzić wysyłki. Spróbuj ponownie — ponowne kliknięcie nie wyśle wiadomości drugi raz.';
}
