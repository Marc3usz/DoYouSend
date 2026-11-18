import { describe, expect, it } from 'vitest';
import type { BatchRecipient, Delivery } from '$lib/api/batches';
import {
	attemptsText,
	deliveryOf,
	deliveryText,
	failureReasonText,
	excludedText,
	fromText,
	isBatchStatus,
	pageParam,
	stillSending,
	timesText
} from './format';

const sms = (over: Partial<Delivery> = {}): Delivery => ({
	channel: 'sms',
	status: 'sent',
	parts: 1,
	attempts: 1,
	error: null,
	...over
});

describe('attemptsText', () => {
	it.each([
		[1, '1 próba'],
		[2, '2 próby'],
		[3, '3 próby'],
		[4, '4 próby'],
		[5, '5 prób'],
		[12, '12 prób'],
		[22, '22 próby'],
		[25, '25 prób']
	])('%d -> %s', (n, want) => {
		expect(attemptsText(n)).toBe(want);
	});
});

describe('deliveryText', () => {
	it.each([
		[undefined, 'brak kontaktu'],
		[sms(), 'Wysłano'],
		[sms({ status: 'delivered', parts: 3 }), 'Doręczono (3 części)'],
		[
			sms({ status: 'failed', error: 'permanent: invalid recipient' }),
			'Błąd: nieprawidłowy adres lub numer'
		],
		[sms({ channel: 'email', status: 'pending', parts: 1, attempts: 0 }), 'Czeka'],
		[sms({ status: 'sent', attempts: 2 }), 'Wysłano (2 próby)'],
		[sms({ status: 'delivered', parts: 3, attempts: 2 }), 'Doręczono (3 części, 2 próby)'],
		[
			sms({ status: 'failed', error: 'transient: rate limit exceeded', attempts: 3 }),
			'Błąd (3 próby): operator chwilowo ograniczył wysyłkę'
		],
		[sms({ channel: 'email', status: 'delivered', attempts: 4 }), 'Doręczono (4 próby)']
	])('%#', (d, want) => {
		expect(deliveryText(d)).toBe(want);
	});

	it('says a skipped channel does not apply instead of blaming the recipient', () => {
		expect(deliveryText(undefined, false)).toBe('nie dotyczy');
	});
});

describe('timesText', () => {
	it.each([
		[1, '1 raz'],
		[2, '2 razy'],
		[5, '5 razy']
	])('%d -> %s', (n, want) => {
		expect(timesText(n)).toBe(want);
	});
});

describe('fromText', () => {
	it('names the sender with the address', () => {
		expect(fromText({ email: 'sekretariat@example.test', name: 'Sekretariat' })).toBe(
			'Sekretariat <sekretariat@example.test>'
		);
	});
	it('shows the bare address without a name and a dash for old batches', () => {
		expect(fromText({ email: 'a@example.test', name: '' })).toBe('a@example.test');
		expect(fromText(null)).toBe('—');
		expect(fromText(undefined)).toBe('—');
	});
});

describe('deliveryOf', () => {
	it('finds the channel or nothing', () => {
		const r: BatchRecipient = {
			recipientId: 'r1',
			firstName: 'Anna',
			lastName: 'Testowa',
			renderedBody: 'Czesc Anna',
			partial: true,
			unreachable: false,
			deliveries: [sms()]
		};
		expect(deliveryOf(r, 'sms')?.status).toBe('sent');
		expect(deliveryOf(r, 'email')).toBeUndefined();
	});
});

describe('isBatchStatus', () => {
	it.each([
		['done', true],
		['done_with_errors', true],
		['toString', false],
		['', false],
		[null, false]
	])('%s', (value, want) => {
		expect(isBatchStatus(value)).toBe(want);
	});
});

describe('pageParam', () => {
	it.each([
		[null, 1],
		['3', 3],
		['0', 1],
		['-2', 1],
		['1.5', 1],
		['abc', 1]
	])('%s', (raw, want) => {
		expect(pageParam(raw)).toBe(want);
	});
});

describe('stillSending', () => {
	it('only for running batches', () => {
		expect(stillSending('running')).toBe(true);
		expect(stillSending('done_with_errors')).toBe(false);
	});
});

describe('excludedText', () => {
	it.each([
		[1, '1 wykluczona osoba'],
		[2, '2 wykluczone osoby'],
		[4, '4 wykluczone osoby'],
		[5, '5 wykluczonych osób'],
		[11, '11 wykluczonych osób'],
		[12, '12 wykluczonych osób'],
		[14, '14 wykluczonych osób'],
		[21, '21 wykluczonych osób'],
		[22, '22 wykluczone osoby'],
		[25, '25 wykluczonych osób']
	])('%d -> %s', (n, want) => {
		expect(excludedText(n)).toBe(want);
	});
});

describe('failureReasonText', () => {
	it.each([
		['permanent: invalid recipient', 'nieprawidłowy adres lub numer'],
		['transient: gateway timeout', 'operator nie odpowiedział na czas'],
		['transient: rate limit exceeded', 'operator chwilowo ograniczył wysyłkę'],
		['unclassified provider error', 'nieznany wynik — wiadomość mogła dotrzeć'],
		[
			'unclassified: interrupted while sending (API restart)',
			'wysyłkę przerwał restart serwera — wynik nieznany'
		],
		[
			'unclassified: no delivery report available from carrier',
			'operator nie ma raportu doręczenia — wynik nieznany'
		],
		[
			'sms over the part limit: 2 parts, at most 1 allowed',
			'SMS dłuższy niż limit części z ustawień'
		],
		['render body: missing value', 'brak danych do pola w treści (np. imienia)'],
		['message rejected by carrier or invalid recipient number', 'nieprawidłowy adres lub numer'],
		['bounced: mailbox full', 'operator: bounced: mailbox full']
	])('%s', (raw, want) => {
		expect(failureReasonText(raw)).toBe(want);
	});
});
