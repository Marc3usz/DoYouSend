import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import { errorTexts, failureText, groupSaveFailure, peopleCount, readGroupForm } from './messages';

describe('peopleCount', () => {
	it('uses the Polish plural forms', () => {
		expect([0, 1, 2, 4, 5, 12, 14, 21, 22, 25, 104, 112].map(peopleCount)).toEqual([
			'0 osób',
			'1 osoba',
			'2 osoby',
			'4 osoby',
			'5 osób',
			'12 osób',
			'14 osób',
			'21 osób',
			'22 osoby',
			'25 osób',
			'104 osoby',
			'112 osób'
		]);
	});
});

describe('failureText', () => {
	it('maps the group codes', () => {
		const err = (code: string) => new ApiError(409, 'x', { code, message: 'x' });
		expect(failureText(err('system_group'))).toBe(errorTexts.systemGroup);
		expect(failureText(err('not_found'))).toBe(errorTexts.notFound);
		expect(failureText(err('unknown_recipient'))).toBe(errorTexts.unknownRecipient);
	});

	it('treats a network error or a bare 404 as an unavailable API', () => {
		expect(failureText(new TypeError('fetch failed'))).toBe(errorTexts.unavailable);
		expect(failureText(new ApiError(404, 'x'))).toBe(errorTexts.unavailable);
	});
});

describe('groupSaveFailure', () => {
	it('puts a taken name on the name field', () => {
		const err = new ApiError(409, 'x', { code: 'name_taken', message: 'x', fields: [] });
		expect(groupSaveFailure(err)).toEqual({
			status: 409,
			errors: { name: 'Grupa o tej nazwie już istnieje.' },
			failure: null
		});
	});

	it('maps field errors and ignores unknown fields', () => {
		const err = new ApiError(400, 'x', {
			code: 'invalid_input',
			message: 'x',
			fields: [
				{ field: 'description', message: 'too long' },
				{ field: 'other', message: 'x' }
			]
		});
		expect(groupSaveFailure(err).errors).toEqual({
			description: 'Opis może mieć najwyżej 500 znaków.'
		});
	});

	it('gives a general text for anything else', () => {
		expect(groupSaveFailure(new TypeError('fetch failed'))).toEqual({
			status: 503,
			errors: {},
			failure: errorTexts.unavailable
		});
	});
});

describe('readGroupForm', () => {
	it('reads missing fields as empty', () => {
		const data = new FormData();
		data.set('name', 'Chór');
		expect(readGroupForm(data)).toEqual({ name: 'Chór', description: '' });
	});
});
