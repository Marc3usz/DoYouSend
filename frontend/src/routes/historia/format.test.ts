import { describe, expect, it } from 'vitest';
import type { BatchRecipient, Delivery } from '$lib/api/batches';
import { deliveryOf, deliveryText, isBatchStatus, pageParam, stillSending } from './format';

const sms = (over: Partial<Delivery> = {}): Delivery => ({
	channel: 'sms',
	status: 'sent',
	parts: 1,
	attempts: 1,
	error: null,
	...over
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
		[sms({ channel: 'email', status: 'pending', parts: 1 }), 'Czeka']
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
