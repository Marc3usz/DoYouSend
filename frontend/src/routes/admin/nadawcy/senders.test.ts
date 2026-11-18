import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import { addressFailureOf, domainFailureOf, readUserIds, usersText } from './senders';

const apiError = (status: number, body: unknown) => new ApiError(status, 'x', body);

describe('domainFailureOf', () => {
	it('marks the domain field for invalid input and duplicates', () => {
		expect(
			domainFailureOf(
				apiError(400, {
					code: 'invalid_input',
					message: 'x',
					fields: [{ field: 'domain', message: 'x' }]
				})
			).fields.domain
		).toMatch(/samą domenę/);
		expect(domainFailureOf(apiError(409, { code: 'domain_exists', message: 'x' })).fields).toEqual({
			domain: 'Ta domena jest już dodana.'
		});
	});

	it("shows the provider's reason when it rejects the domain", () => {
		const got = domainFailureOf(
			apiError(502, { code: 'provider_rejected', message: 'domain is invalid' })
		);
		expect(got.failure).toBe('SendGrid odrzucił żądanie: domain is invalid');
	});

	it('falls back to a generic text for a dead backend', () => {
		expect(domainFailureOf(new TypeError('fetch failed')).status).toBe(500);
	});
});

describe('addressFailureOf', () => {
	it('maps every invalid field', () => {
		const got = addressFailureOf(
			apiError(400, {
				code: 'invalid_input',
				message: 'x',
				fields: [
					{ field: 'email', message: 'x' },
					{ field: 'name', message: 'x' }
				]
			})
		);
		expect(Object.keys(got.fields)).toEqual(['email', 'name']);
	});

	it('marks a duplicate address', () => {
		expect(
			addressFailureOf(apiError(409, { code: 'address_exists', message: 'x' })).fields
		).toEqual({ email: 'Ten adres jest już dodany.' });
	});
});

describe('usersText', () => {
	it('lists names or says nobody', () => {
		expect(usersText([])).toBe('nikt');
		expect(usersText([{ fullName: 'Jan Testowy' }, { fullName: 'Anna Próbna' }])).toBe(
			'Jan Testowy, Anna Próbna'
		);
	});
});

describe('readUserIds', () => {
	it('keeps each checked id once', () => {
		const data = new FormData();
		data.append('userIds', 'u1');
		data.append('userIds', 'u2');
		data.append('userIds', 'u1');
		data.append('userIds', '');
		expect(readUserIds(data)).toEqual(['u1', 'u2']);
	});
});
