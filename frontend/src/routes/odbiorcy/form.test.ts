import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import { formErrors, readForm, saveFailure, toInput } from './form';
import { errorTexts } from './messages';

describe('readForm and toInput', () => {
	it('sends blank contacts as null and keeps the rest as typed', () => {
		const data = new FormData();
		data.set('firstName', ' Jan ');
		data.set('lastName', 'Kowalski');
		data.set('email', '   ');
		data.set('phone', '500 100 101');
		data.set('type', 'parent');
		data.set('classes', ' 1b, 3A ;; ');
		expect(toInput(readForm(data))).toEqual({
			firstName: ' Jan ',
			lastName: 'Kowalski',
			email: null,
			phone: '500 100 101',
			type: 'parent',
			classes: ['1b', '3A']
		});
	});

	it('sends no class when the field is blank', () => {
		expect(toInput(readForm(new FormData())).classes).toEqual([]);
	});

	it('passes an unknown or missing type on as empty for the backend to report', () => {
		expect(toInput(readForm(new FormData())).type).toBe('');
		const data = new FormData();
		data.set('type', 'teacher');
		expect(toInput(readForm(data)).type).toBe('');
	});
});

describe('formErrors', () => {
	it('maps backend field keys to form fields, ignoring unknown ones', () => {
		const err = new ApiError(400, 'x', {
			code: 'invalid_input',
			message: 'invalid input',
			fields: [
				{ field: 'last_name', message: 'required' },
				{ field: 'contact', message: 'e-mail or phone required' },
				{ field: 'whatever', message: 'x' }
			]
		});
		expect(formErrors(err)).toEqual({
			fields: {
				lastName: 'Wpisz nazwisko.',
				contact: expect.stringContaining('e-mail albo telefon')
			}
		});
	});

	it('names the recipient who already has the contact', () => {
		const err = new ApiError(409, 'x', {
			code: 'duplicate_contact',
			message: 'contact is already used',
			fields: [{ field: 'phone', message: 'x' }],
			ids: ['11111111-1111-4111-8111-000000000001']
		});
		expect(formErrors(err)).toEqual({
			fields: { phone: 'Ten numer telefonu ma już inny odbiorca.' },
			duplicateOf: '11111111-1111-4111-8111-000000000001'
		});
	});

	it('leaves other errors to the caller', () => {
		expect(formErrors(new ApiError(404, 'x', { code: 'not_found', message: 'x' }))).toBeUndefined();
		expect(formErrors(new TypeError('fetch failed'))).toBeUndefined();
	});
});

describe('saveFailure', () => {
	it('keeps the status of a field error and has no general text', () => {
		const err = new ApiError(400, 'x', { code: 'invalid_input', message: 'x', fields: [] });
		expect(saveFailure(err)).toEqual({ status: 400, errors: { fields: {} }, failure: null });
	});

	it('reports an unreachable API as 503 with a general text', () => {
		expect(saveFailure(new TypeError('fetch failed'))).toEqual({
			status: 503,
			errors: { fields: {} },
			failure: errorTexts.unavailable
		});
	});
});
