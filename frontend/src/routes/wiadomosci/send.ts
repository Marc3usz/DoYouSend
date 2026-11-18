// Polish texts for the composer's final confirmation (POST /batches).
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';
import type { SendEstimate } from '$lib/api/messages';
import { overSMSLimit, plural, smsText } from './estimate';

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

/** Everything that makes two confirmations "the same send". */
export type DraftSnapshot = {
	subject: string;
	body: string;
	channels: readonly ('email' | 'sms')[];
	senderId?: string;
	selection: {
		groupIds: readonly string[];
		recipientIds: readonly string[];
		excludedRecipientIds: readonly string[];
	};
};

/** A stable text form of a draft; the order of picked groups or people does not matter. */
export function draftFingerprint(d: DraftSnapshot): string {
	const sorted = (xs: readonly string[]) => [...xs].sort();
	return JSON.stringify([
		d.subject,
		d.body,
		sorted(d.channels),
		d.senderId ?? '',
		sorted(d.selection.groupIds),
		sorted(d.selection.recipientIds),
		sorted(d.selection.excludedRecipientIds)
	]);
}

export type IdempotencyState = { fingerprint: string; key: string };

/**
 * The Idempotency-Key for confirming `draft`. The same draft keeps its key, so
 * confirming again after a failure whose outcome is unknown (the request may
 * have reached the backend) returns the batch already sent instead of a second
 * one. Any change to the draft gets a fresh key.
 */
export function idempotencyFor(
	previous: IdempotencyState | null,
	draft: DraftSnapshot,
	makeKey: () => string = () => newIdempotencyKey()
): IdempotencyState {
	const fingerprint = draftFingerprint(draft);
	if (previous && previous.fingerprint === fingerprint) return previous;
	return { fingerprint, key: makeKey() };
}

/** Whether the draft may be confirmed: the backend would refuse it otherwise. */
export function canConfirm(
	subject: string,
	est: SendEstimate | null,
	counting: boolean,
	channels?: ('email' | 'sms')[]
): boolean {
	const requiresSubject = !channels || channels.includes('email');
	const subjectOk = !requiresSubject || subject.trim() !== '';
	return (
		!counting &&
		subjectOk &&
		est !== null &&
		est.recipientCount > 0 &&
		est.template.parts > 0 &&
		est.unknownPlaceholders.length === 0 &&
		// SMS only and over the part limit: nothing would go out.
		!(channels?.length === 1 && channels[0] === 'sms' && overSMSLimit(est))
	);
}

/** One sentence the sender confirms: who gets what over which channel. */
export function confirmText(est: SendEstimate): string {
	const parts: string[] = [];
	if (est.emailCount > 0) {
		parts.push(`${est.emailCount} ${plural(est.emailCount, 'e-mail', 'e-maile', 'e-maili')}`);
	}
	if (est.smsCount > 0) {
		parts.push(smsText(est.totalSmsParts));
	}
	const what = parts.length > 0 ? parts.join(' i ') : '0 wiadomości';
	// "do" takes the genitive: do 1 odbiorcy, do 3 odbiorców.
	const n = est.recipientCount;
	return `Wyślesz ${what} do ${n} ${n === 1 ? 'odbiorcy' : 'odbiorców'}.`;
}

/** Text for a confirmation the backend refused or that did not reach it. */
export function sendFailureText(err: unknown): string {
	const body = errorBody(err);
	switch (body?.code) {
		case 'invalid_input':
			if (body.fields.some((f) => f.field === 'senderId')) {
				return 'Nie możesz wysyłać z tego adresu. Odśwież stronę i wybierz nadawcę ponownie.';
			}
			if (body.fields.some((f) => f.field === 'subject')) return 'Uzupełnij temat e-maila.';
			if (body.fields.some((f) => f.field === 'body')) {
				return 'Treść jest pusta albo zawiera nieznane pole — dostępne są tylko {{imie}} i {{nazwisko}}.';
			}
			return 'Serwer odrzucił wysyłkę. Odśwież stronę i spróbuj ponownie.';
		case 'empty_selection':
			return 'Wybierz co najmniej jedną grupę lub osobę.';
		case 'selection_changed':
			return 'Część zaznaczonych grup lub osób już nie istnieje. Odśwież stronę i wybierz odbiorców ponownie. Nic nie zostało wysłane.';
		case 'no_recipients':
			return 'Wybrane grupy i osoby nie dają teraz żadnego odbiorcy. Nic nie zostało wysłane.';
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
