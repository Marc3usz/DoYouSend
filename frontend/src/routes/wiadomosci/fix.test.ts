import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import { contactFixErrorText, missingChannels, validateContactFixInput } from './fix';

describe('missingChannels', () => {
	it('detects both channels missing when empty', () => {
		expect(missingChannels([])).toEqual({ needsEmail: true, needsPhone: true });
	});

	it('detects missing phone when only email present', () => {
		expect(missingChannels(['email'])).toEqual({ needsEmail: false, needsPhone: true });
	});

	it('detects missing email when only sms present', () => {
		expect(missingChannels(['sms'])).toEqual({ needsEmail: true, needsPhone: false });
	});

	it('detects none missing when both channels present', () => {
		expect(missingChannels(['email', 'sms'])).toEqual({ needsEmail: false, needsPhone: false });
	});
});

describe('validateContactFixInput', () => {
	it('requires at least one value when both are missing', () => {
		const needs = { needsEmail: true, needsPhone: true };
		expect(validateContactFixInput({ email: '', phone: '' }, needs)).toBe(
			'Podaj adres e-mail lub numer telefonu do uzupełnienia.'
		);
		expect(validateContactFixInput({ email: '   ', phone: '   ' }, needs)).toBe(
			'Podaj adres e-mail lub numer telefonu do uzupełnienia.'
		);
		expect(validateContactFixInput({ email: 'a@example.test' }, needs)).toBeNull();
		expect(validateContactFixInput({ phone: '+48 500 100 101' }, needs)).toBeNull();
		expect(
			validateContactFixInput({ email: 'a@example.test', phone: '+48 500 100 101' }, needs)
		).toBeNull();
	});

	it('requires email when only email is missing', () => {
		const needs = { needsEmail: true, needsPhone: false };
		expect(validateContactFixInput({ email: '' }, needs)).toBe('Wpisz brakujący adres e-mail.');
		expect(validateContactFixInput({ email: 'a@example.test' }, needs)).toBeNull();
	});

	it('requires phone when only phone is missing', () => {
		const needs = { needsEmail: false, needsPhone: true };
		expect(validateContactFixInput({ phone: '' }, needs)).toBe('Wpisz brakujący numer telefonu.');
		expect(validateContactFixInput({ phone: '+48 500 100 101' }, needs)).toBeNull();
	});
});

describe('contactFixErrorText', () => {
	it('handles non-ApiError', () => {
		expect(contactFixErrorText(new Error('boom'))).toContain('Nie udało się zapisać');
	});

	it('handles duplicate_contact', () => {
		const err = new ApiError(409, 'conflict', {
			code: 'duplicate_contact',
			message: 'duplicate contact',
			fields: [{ field: 'email', message: 'duplicate' }],
			ids: []
		});
		expect(contactFixErrorText(err)).toBe(
			'Ten adres e-mail lub numer telefonu jest już przypisany do innego odbiorcy.'
		);
	});

	it('handles contact_not_fixable', () => {
		const err = new ApiError(409, 'conflict', {
			code: 'contact_not_fixable',
			message: 'channel is working',
			fields: [],
			ids: []
		});
		expect(contactFixErrorText(err)).toBe(
			'Te dane kontaktowe zostały już uzupełnione lub nie wymagają poprawy.'
		);
	});

	it('handles invalid_input for email only', () => {
		const err = new ApiError(400, 'bad request', {
			code: 'invalid_input',
			message: 'validation failed',
			fields: [{ field: 'email', message: 'invalid email' }],
			ids: []
		});
		expect(contactFixErrorText(err)).toBe('Podaj poprawny adres e-mail (np. jan@example.test).');
	});

	it('handles invalid_input for phone only', () => {
		const err = new ApiError(400, 'bad request', {
			code: 'invalid_input',
			message: 'validation failed',
			fields: [{ field: 'phone', message: 'invalid phone' }],
			ids: []
		});
		expect(contactFixErrorText(err)).toBe('Podaj poprawny numer telefonu (np. +48 500 100 101).');
	});

	it('handles invalid_input for both email and phone', () => {
		const err = new ApiError(400, 'bad request', {
			code: 'invalid_input',
			message: 'validation failed',
			fields: [
				{ field: 'email', message: 'invalid' },
				{ field: 'phone', message: 'invalid' }
			],
			ids: []
		});
		expect(contactFixErrorText(err)).toBe('Podaj poprawny adres e-mail i numer telefonu.');
	});

	it('handles not_found', () => {
		const err = new ApiError(404, 'not found', {
			code: 'not_found',
			message: 'recipient not found',
			fields: [],
			ids: []
		});
		expect(contactFixErrorText(err)).toBe('Odbiorca nie został znaleziony (mógł zostać usunięty).');
	});

	it('handles generic server error', () => {
		const err = new ApiError(500, 'server error', {
			code: 'internal',
			message: 'db error',
			fields: [],
			ids: []
		});
		expect(contactFixErrorText(err)).toBe(
			'Nie udało się zapisać danych kontaktowych. Spróbuj ponownie za chwilę.'
		);
	});
});
