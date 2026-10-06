import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import type { SendEstimate } from '$lib/api/messages';
import {
	costText,
	estimateWarnings,
	partsText,
	plural,
	previewFailureText,
	recipientsText,
	smsText
} from './estimate';

const empty: SendEstimate = {
	template: { encoding: 'GSM7', units: 0, parts: 0 },
	placeholders: [],
	unknownPlaceholders: [],
	recipientCount: 0,
	emailCount: 0,
	smsCount: 0,
	partialCount: 0,
	unreachableCount: 0,
	ucs2Count: 0,
	minPartsPerRecipient: 0,
	maxPartsPerRecipient: 0,
	totalSmsParts: 0,
	costMilli: 0,
	renderFailedIds: []
};

// Intl separates the amount and the currency with a no-break space.
const spaces = (s: string) => s.replace(/\s/g, ' ');

describe('costText', () => {
	it.each([
		[0, '0,00 zł'],
		[80, '0,08 zł'],
		[65, '0,065 zł'],
		[123450, '123,45 zł'],
		[1234500, '1234,50 zł']
	])('formats %d thousandths as %s', (milli, want) => {
		expect(spaces(costText(milli))).toBe(want);
	});
});

describe('plural', () => {
	it.each([
		[0, 'SMS-ów'],
		[1, 'SMS'],
		[2, 'SMS-y'],
		[4, 'SMS-y'],
		[5, 'SMS-ów'],
		[12, 'SMS-ów'],
		[22, 'SMS-y'],
		[112, 'SMS-ów']
	])('picks the Polish form for %d', (n, want) => {
		expect(plural(n, 'SMS', 'SMS-y', 'SMS-ów')).toBe(want);
	});

	it('builds counted phrases', () => {
		expect(smsText(3)).toBe('3 SMS-y');
		expect(recipientsText(1)).toBe('1 odbiorca');
		expect(recipientsText(3)).toBe('3 odbiorcy');
		expect(recipientsText(25)).toBe('25 odbiorców');
	});
});

describe('partsText', () => {
	it('shows one number when every recipient gets the same', () => {
		expect(partsText(2, 2)).toBe('2');
	});
	it('shows a range when personalisation changes the length', () => {
		expect(partsText(1, 3)).toBe('1–3');
	});
});

describe('estimateWarnings', () => {
	it('is empty for a clean estimate', () => {
		expect(estimateWarnings(empty)).toEqual([]);
	});

	it('names unknown placeholders and the supported ones', () => {
		expect(estimateWarnings({ ...empty, unknownPlaceholders: ['klasa', 'data'] })).toEqual([
			'Nieznane pola {{klasa}}, {{data}} — dostępne są tylko {{imie}} i {{nazwisko}}. Z takimi polami wiadomość nie zostanie wysłana.'
		]);
	});

	it('lists every recipient problem with a count', () => {
		const warnings = estimateWarnings({
			...empty,
			renderFailedIds: ['a', 'b'],
			unreachableCount: 1,
			partialCount: 5
		});
		expect(warnings).toEqual([
			'2 odbiorcy nie mają wartości dla użytego pola (np. pustego imienia) — nie dostaną wiadomości.',
			'1 odbiorca nie ma ani e-maila, ani telefonu — nie dostanie wiadomości.',
			'5 odbiorców dostanie wiadomość tylko jednym kanałem (brak e-maila albo telefonu).'
		]);
	});
});

describe('previewFailureText', () => {
	it('explains a rejected selection', () => {
		const err = new ApiError(400, 'POST', { code: 'invalid_input', message: 'x' });
		expect(previewFailureText(err)).toContain('wybór odbiorców');
	});
	it('falls back to a generic text', () => {
		expect(previewFailureText(new Error('network'))).toBe(
			'Nie udało się policzyć podsumowania. Spróbuj ponownie za chwilę.'
		);
	});
});
