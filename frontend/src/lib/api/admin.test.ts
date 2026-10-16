import { describe, expect, it } from 'vitest';
import { emailProfile, emailUsage, smsProfile, smsUsage } from './admin';

describe('emailUsage', () => {
	it('calls GET /api/stats/email with query params', async () => {
		const expected = {
			totalMessages: 10,
			deliveredMessages: 8,
			sentMessages: 1,
			failedMessages: 1,
			inFlightMessages: 0
		};
		const fakeFetch: typeof fetch = async (input) => {
			expect(String(input)).toBe('/api/stats/email?from=2026-10-01&to=2026-10-31');
			return new Response(JSON.stringify(expected), { status: 200 });
		};
		const got = await emailUsage({ from: '2026-10-01', to: '2026-10-31' }, fakeFetch);
		expect(got).toEqual(expected);
	});

	it('calls GET /api/stats/email without query params', async () => {
		const fakeFetch: typeof fetch = async (input) => {
			expect(String(input)).toBe('/api/stats/email');
			return new Response(JSON.stringify({ totalMessages: 0 }), { status: 200 });
		};
		const got = await emailUsage({}, fakeFetch);
		expect(got.totalMessages).toBe(0);
	});
});

describe('smsUsage', () => {
	it('calls GET /api/stats/sms with query params', async () => {
		const fakeFetch: typeof fetch = async (input) => {
			expect(String(input)).toBe('/api/stats/sms?from=2026-10-01&to=2026-10-31');
			return new Response(JSON.stringify({ totalMessages: 5, totalParts: 10 }), { status: 200 });
		};
		const got = await smsUsage({ from: '2026-10-01', to: '2026-10-31' }, fakeFetch);
		expect(got.totalMessages).toBe(5);
	});
});

describe('smsProfile', () => {
	it('calls GET /api/admin/providers/sms/profile', async () => {
		const expected = {
			points: 125.5,
			name: 'SMSAPI Test',
			userType: 'developer'
		};
		const fakeFetch: typeof fetch = async (input) => {
			expect(String(input)).toBe('/api/admin/providers/sms/profile');
			return new Response(JSON.stringify(expected), { status: 200 });
		};
		const got = await smsProfile(fakeFetch);
		expect(got).toEqual(expected);
	});
});

describe('emailProfile', () => {
	it('calls GET /api/admin/providers/email/profile', async () => {
		const expected = {
			provider: 'sendgrid',
			remainCredits: 95,
			totalCredits: 100,
			usedCredits: 5,
			resetFrequency: 'day',
			nextReset: '2026-10-17'
		};
		const fakeFetch: typeof fetch = async (input) => {
			expect(String(input)).toBe('/api/admin/providers/email/profile');
			return new Response(JSON.stringify(expected), { status: 200 });
		};
		const got = await emailProfile(fakeFetch);
		expect(got).toEqual(expected);
	});
});
