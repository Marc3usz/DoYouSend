import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import type { AuditEntry } from '$lib/api/admin';
import { actionText, actorText, currentMonth, detailsText, plnText, setupChecks } from './format';
import { failureOf } from './uzytkownicy/users';
import { loginFailureText } from '../login/messages';

// Intl separates the amount and the currency with a no-break space.
const spaces = (s: string) => s.replace(/\s/g, ' ');

const entry = (over: Partial<AuditEntry>): AuditEntry => ({
	id: 1,
	userId: null,
	userName: null,
	action: 'login',
	entity: null,
	entityId: null,
	details: {},
	createdAt: '2026-10-09T08:00:00Z',
	...over
});

describe('plnText', () => {
	it.each([
		[0, '0,00 zł'],
		[80, '0,08 zł'],
		[1234560, '1 234,56 zł']
	])('formats %d thousandths as %s', (milli, want) => {
		expect(spaces(plnText(milli))).toBe(want);
	});
});

describe('currentMonth', () => {
	it('uses the Warsaw calendar', () => {
		// 30 Sep 23:30 UTC is already 1 October in Warsaw.
		const m = currentMonth(new Date('2026-09-30T23:30:00Z'));
		expect([m.from, m.to]).toEqual(['2026-10-01', '2026-10-31']);
		expect(m.label).toMatch(/październik 2026/);
	});
	it('knows February of a leap year', () => {
		const m = currentMonth(new Date('2028-02-10T12:00:00Z'));
		expect(m.to).toBe('2028-02-29');
	});
});

describe('audit texts', () => {
	it('names known actions and passes others through', () => {
		expect(actionText('user_created')).toBe('Utworzenie konta');
		expect(actionText('batch_confirmed')).toBe('batch_confirmed');
	});
	it('describes details in Polish', () => {
		expect(detailsText(entry({ details: { role: 'admin', disabled: 'true' } }))).toBe(
			'rola: administrator, zablokowane'
		);
	});
	it('names the actor', () => {
		expect(actorText(entry({ userName: 'Anna Testowa' }))).toBe('Anna Testowa');
		expect(actorText(entry({ action: 'login_failed' }))).toBe('nieznany adres e-mail');
		expect(actorText(entry({ action: 'login_failed', userName: 'Anna Testowa' }))).toBe(
			'Anna Testowa (próba logowania)'
		);
		expect(actorText(entry({ userId: 'x' }))).toBe('usunięty użytkownik');
		expect(actorText(entry({}))).toBe('system');
	});
});

describe('setupChecks', () => {
	const local = {
		dryRun: true,
		email: { provider: 'mailpit', credentialsSet: false, eventsWebhook: false },
		sms: { provider: 'fake', credentialsSet: false, reportsWebhook: false }
	};
	it('is all clear for the local test setup', () => {
		expect(setupChecks(local).every((c) => c.ok)).toBe(true);
	});
	it('warns about a real gateway without a key or reports', () => {
		const checks = setupChecks({
			...local,
			dryRun: false,
			sms: { provider: 'smsapi', credentialsSet: false, reportsWebhook: false }
		});
		expect(checks.filter((c) => !c.ok)).toHaveLength(3);
	});
});

describe('failure texts', () => {
	const apiError = (status: number, body: object) => new ApiError(status, 'x', body);

	it('maps user form errors to fields', () => {
		const got = failureOf(
			apiError(400, {
				code: 'invalid_input',
				message: 'x',
				fields: [{ field: 'password', message: 'x' }]
			})
		);
		expect(got.fields.password).toMatch(/12 do 128/);
	});
	it('explains the last-admin guard', () => {
		expect(failureOf(apiError(409, { code: 'last_admin', message: 'x' })).failure).toMatch(
			/ostatni aktywny administrator/
		);
	});
	it('does not reveal why a login failed', () => {
		const got = loginFailureText(apiError(401, { code: 'invalid_credentials', message: 'x' }));
		expect(got.text).toMatch(/e-mail lub hasło/);
	});
	it('treats a dead backend as unavailable', () => {
		expect(loginFailureText(new TypeError('fetch failed')).status).toBe(503);
	});
});
