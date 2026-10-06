import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import { errorTexts, failureText, isNotFound, issueText } from './messages';

describe('issueText', () => {
	it('has a text for every channel and reason', () => {
		expect(issueText({ channel: 'email', reason: 'missing' })).toBe('Brak e-maila');
		expect(issueText({ channel: 'sms', reason: 'invalid' })).toBe('Błędny numer');
	});
});

describe('failureText', () => {
	it('maps contract codes', () => {
		const inUse = new ApiError(409, 'x', { code: 'recipient_in_use', message: 'x' });
		const notFound = new ApiError(404, 'x', { code: 'not_found', message: 'x' });
		expect(failureText(inUse)).toBe(errorTexts.inUse);
		expect(failureText(notFound)).toBe(errorTexts.notFound);
	});

	it('treats a network error or a bare 404 (API without a database) as unavailable', () => {
		expect(failureText(new TypeError('fetch failed'))).toBe(errorTexts.unavailable);
		expect(failureText(new ApiError(404, 'x'))).toBe(errorTexts.unavailable);
		expect(failureText(new ApiError(502, 'x'))).toBe(errorTexts.unavailable);
	});

	it('falls back to a generic text', () => {
		expect(failureText(new ApiError(400, 'x', { code: 'invalid_input', message: 'x' }))).toBe(
			errorTexts.unexpected
		);
	});
});

describe('isNotFound', () => {
	it('needs the not_found code, not just the status', () => {
		expect(isNotFound(new ApiError(404, 'x', { code: 'not_found', message: 'x' }))).toBe(true);
		expect(isNotFound(new ApiError(404, 'x'))).toBe(false);
	});
});
