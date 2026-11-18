import { describe, expect, it } from 'vitest';
import type { SenderOption } from '$lib/api/senders';
import { initialSenderId, senderOptionText, senderReasonText } from './senders';

const opt = (over: Partial<SenderOption>): SenderOption => ({
	id: 'shared',
	email: 'szkola@example.test',
	name: 'Szkoła',
	kind: 'shared',
	usable: true,
	reason: null,
	...over
});

describe('senderReasonText', () => {
	it.each([
		[null, null],
		['domain_not_authenticated', 'domena nie jest uwierzytelniona'],
		['not_verified', 'adres nie jest zweryfikowany'],
		['something_new', 'adres jest niedostępny']
	])('%s', (reason, want) => {
		expect(senderReasonText(reason)).toBe(want);
	});
});

describe('senderOptionText', () => {
	it('names the address and its kind', () => {
		expect(senderOptionText(opt({}))).toBe('Szkoła <szkola@example.test> — wspólny adres szkoły');
	});
	it('says why an option is disabled', () => {
		expect(
			senderOptionText(
				opt({
					id: 'own',
					kind: 'own',
					name: '',
					email: 'jan@example.test',
					usable: false,
					reason: 'domain_not_authenticated'
				})
			)
		).toBe('jan@example.test — Twój adres (domena nie jest uwierzytelniona)');
	});
});

describe('initialSenderId', () => {
	it('picks the default when it can be used', () => {
		expect(
			initialSenderId({ defaultId: 'own', items: [opt({}), opt({ id: 'own', kind: 'own' })] })
		).toBe('own');
	});
	it('falls back to the first usable option', () => {
		expect(
			initialSenderId({
				defaultId: 'own',
				items: [opt({ id: 'own', kind: 'own', usable: false }), opt({})]
			})
		).toBe('shared');
	});
	it('leaves it to the backend without options', () => {
		expect(initialSenderId(null)).toBeUndefined();
		expect(initialSenderId({ defaultId: 'shared', items: [] })).toBeUndefined();
	});
});
