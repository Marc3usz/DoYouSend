import { describe, expect, it } from 'vitest';
import type { BatchRecipient, Delivery } from '$lib/api/batches';
import {
	attemptsText,
	deliveryOf,
	deliveryText,
	isBatchStatus,
	pageParam,
	stillSending
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
		[undefined, 'brak kanału'],
		[sms(), 'Wysłano'],
		[sms({ status: 'delivered', parts: 3 }), 'Doręczono (3 części)'],
		[
			sms({ status: 'failed', error: 'permanent: invalid recipient' }),
			'Błąd: permanent: invalid recipient'
		],
		[sms({ channel: 'email', status: 'pending', parts: 1, attempts: 0 }), 'Czeka'],
		[sms({ status: 'sent', attempts: 2 }), 'Wysłano (2 próby)'],
		[sms({ status: 'delivered', parts: 3, attempts: 2 }), 'Doręczono (3 części, 2 próby)'],
		[
			sms({ status: 'failed', error: 'transient: rate limit exceeded', attempts: 3 }),
			'Błąd (3 próby): transient: rate limit exceeded'
		],
		[sms({ channel: 'email', status: 'delivered', attempts: 4 }), 'Doręczono (4 próby)']
	])('%#', (d, want) => {
		expect(deliveryText(d)).toBe(want);
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
