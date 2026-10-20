import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import type { SendEstimate } from '$lib/api/messages';
import { canConfirm, confirmText, newIdempotencyKey, sendFailureText } from './send';

const est: SendEstimate = {
	template: { encoding: 'GSM7', units: 20, parts: 1 },
	placeholders: [],
	unknownPlaceholders: [],
	recipientCount: 3,
	emailCount: 2,
	smsCount: 3,
	partialCount: 1,
	unreachableCount: 0,
	ucs2Count: 0,
	minPartsPerRecipient: 1,
	maxPartsPerRecipient: 2,
	totalSmsParts: 4,
	costMilli: 320,
	renderFailedIds: []
};

const apiError = (status: number, body: unknown) => new ApiError(status, 'POST /batches', body);

describe('canConfirm', () => {
	it.each([
		['ready', 'Zebranie', est, false, true],
		['still counting', 'Zebranie', est, true, false],
		['blank subject', '   ', est, false, false],
		['no estimate', 'Zebranie', null, false, false],
		['nobody selected', 'Zebranie', { ...est, recipientCount: 0 }, false, false],
		['empty body', 'Zebranie', { ...est, template: { ...est.template, parts: 0 } }, false, false],
		['unknown placeholder', 'Zebranie', { ...est, unknownPlaceholders: ['klasa'] }, false, false]
	])('%s', (_, subject, estimate, counting, want) => {
		expect(canConfirm(subject, estimate, counting)).toBe(want);
	});
});

describe('confirmText', () => {
	it('names e-mails, SMS parts and recipients', () => {
		expect(confirmText(est)).toBe('Wyślesz 2 e-maile i 4 SMS-y do 3 odbiorców.');
	});
	it('uses the genitive plural for many e-mails and recipients', () => {
		expect(confirmText({ ...est, emailCount: 5, recipientCount: 22 })).toBe(
			'Wyślesz 5 e-maili i 4 SMS-y do 22 odbiorców.'
		);
	});
	it('leaves SMS out when nobody gets one', () => {
		expect(confirmText({ ...est, emailCount: 1, smsCount: 0, recipientCount: 1 })).toBe(
			'Wyślesz 1 e-mail do 1 odbiorcy.'
		);
	});
});

describe('sendFailureText', () => {
	it.each([
		[
			apiError(400, {
				code: 'invalid_input',
				message: 'x',
				fields: [{ field: 'subject', message: 'x' }]
			}),
			'Uzupełnij temat e-maila.'
		],
		[
			apiError(409, { code: 'selection_changed', message: 'x', ids: ['g1'] }),
			'Część zaznaczonych grup lub osób już nie istnieje. Odśwież stronę i wybierz odbiorców ponownie. Nic nie zostało wysłane.'
		],
		[
			apiError(422, { code: 'no_recipients', message: 'x' }),
			'Zaznaczone grupy nie mają teraz żadnych odbiorców. Nic nie zostało wysłane.'
		],
		[
			apiError(409, { code: 'idempotency_conflict', message: 'x' }),
			'To potwierdzenie było już użyte dla innej treści. Wróć do edycji i zatwierdź wysyłkę ponownie.'
		],
		[
			apiError(503, { code: 'internal', message: 'x' }),
			'Serwer jest właśnie restartowany. Spróbuj ponownie za chwilę — nic nie zostało wysłane.'
		],
		[
			new TypeError('Failed to fetch'),
			'Nie udało się potwierdzić wysyłki. Spróbuj ponownie — ponowne kliknięcie nie wyśle wiadomości drugi raz.'
		]
	])('%#', (err, want) => {
		expect(sendFailureText(err)).toBe(want);
	});
});

describe('newIdempotencyKey', () => {
	const uuidV4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

	it('uses randomUUID when the context has it', () => {
		expect(newIdempotencyKey(crypto)).toMatch(uuidV4);
	});

	it('falls back to getRandomValues over plain HTTP', () => {
		const insecure = { getRandomValues: crypto.getRandomValues.bind(crypto) };
		const a = newIdempotencyKey(insecure);
		expect(a).toMatch(uuidV4);
		expect(newIdempotencyKey(insecure)).not.toBe(a);
	});
});
