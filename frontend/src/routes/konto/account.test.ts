import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import { checkPasswordForm, passwordFailureOf } from './account';

describe('checkPasswordForm', () => {
	const ok = {
		currentPassword: 'stare-haslo-123',
		newPassword: 'nowe-haslo-12345',
		repeat: 'nowe-haslo-12345'
	};

	it('accepts a valid change', () => {
		expect(checkPasswordForm(ok)).toEqual({});
	});

	it.each([
		['missing current password', { currentPassword: '' }, 'currentPassword'],
		['too short', { newPassword: 'krotkie', repeat: 'krotkie' }, 'newPassword'],
		[
			'same as before',
			{ newPassword: 'stare-haslo-123', repeat: 'stare-haslo-123' },
			'newPassword'
		],
		['not repeated', { repeat: 'inne-haslo-12345' }, 'repeat']
	] as const)('%s', (_, change, field) => {
		expect(Object.keys(checkPasswordForm({ ...ok, ...change }))).toContain(field);
	});
});

describe('passwordFailureOf', () => {
	const apiError = (status: number, body: unknown) => new ApiError(status, 'x', body);

	it('marks a wrong current password', () => {
		expect(
			passwordFailureOf(apiError(403, { code: 'wrong_password', message: 'x' })).fields
		).toEqual({ currentPassword: 'Obecne hasło jest nieprawidłowe.' });
	});

	it('marks a rejected new password', () => {
		const got = passwordFailureOf(
			apiError(400, {
				code: 'invalid_input',
				message: 'x',
				fields: [{ field: 'newPassword', message: 'x' }]
			})
		);
		expect(got.fields.newPassword).toMatch(/od 12 do 128/);
	});

	it('has a general message for a dead backend', () => {
		expect(passwordFailureOf(new TypeError('fetch failed')).failure).toMatch(/Nie udało się/);
	});
});
