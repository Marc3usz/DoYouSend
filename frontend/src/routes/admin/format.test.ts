import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import type { AuditEntry } from '$lib/api/admin';
import {
	actionText,
	actorText,
	creditsText,
	currentMonth,
	detailsText,
	entityHref,
	monthOf,
	LOW_EMAIL_CREDITS_THRESHOLD,
	LOW_POINTS_THRESHOLD,
	plnText,
	pointsText,
	setupChecks
} from './format';
import { failureOf, senderFailureOf } from './uzytkownicy/users';
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

describe('pointsText', () => {
	it.each([
		[0, '0,00 pkt'],
		[125.5, '125,50 pkt'],
		[500, '500,00 pkt'],
		[1234.56, '1 234,56 pkt']
	])('formats %d points as %s', (points, want) => {
		expect(spaces(pointsText(points))).toBe(want);
	});

	it('defines warning threshold at 50 points', () => {
		expect(LOW_POINTS_THRESHOLD).toBe(50);
	});
});

describe('creditsText', () => {
	it('formats remain and total credits', () => {
		expect(creditsText(95, 100)).toBe('95 / 100 kredytów');
	});

	it('formats only remain credits', () => {
		expect(creditsText(95)).toBe('95 kredytów');
	});

	it('returns dash when undefined', () => {
		expect(creditsText(undefined)).toBe('—');
	});

	it('defines warning threshold at 20 credits', () => {
		expect(LOW_EMAIL_CREDITS_THRESHOLD).toBe(20);
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

describe('monthOf', () => {
	const now = new Date('2026-11-13T10:00:00Z');
	it('reads ?miesiac= for a past month and links both ways', () => {
		const m = monthOf('2026-01', now);
		expect([m.from, m.to, m.previous, m.next]).toEqual([
			'2026-01-01',
			'2026-01-31',
			'2025-12',
			'2026-02'
		]);
		expect(m.label).toMatch(/styczeń 2026/);
		expect(m.isCurrent).toBe(false);
	});
	it('has no next link for the current month', () => {
		const m = monthOf(null, now);
		expect(m.key).toBe('2026-11');
		expect(m.next).toBeNull();
		expect(m.isCurrent).toBe(true);
	});
	it.each(['2026-12', '2026-13', '26-01', 'abc'])('ignores %s', (raw) => {
		expect(monthOf(raw, now).key).toBe('2026-11');
	});
});

describe('audit texts', () => {
	it('names known actions and passes others through', () => {
		expect(actionText('user_created')).toBe('Utworzenie konta');
		expect(actionText('batch_sent')).toBe('Zatwierdzenie wysyłki');
		expect(actionText('batch_retried')).toBe('Ponowienie wysyłki');
		expect(actionText('recipients_imported')).toBe('Import odbiorców');
		expect(actionText('group_deleted')).toBe('inne zdarzenie (group_deleted)');
	});
	it('describes an import and a settings change in Polish', () => {
		expect(
			detailsText(
				entry({
					action: 'recipients_imported',
					details: { source: 'notion', imported: '12', invalid: '1', duplicates: '0' }
				})
			)
		).toBe('źródło: Notion, zapisano: 12, błędnych: 1, duplikatów: 0');
		expect(
			detailsText(
				entry({
					action: 'settings_changed',
					details: { keys: 'dry_run,sms_provider', dry_run: 'false', sms_provider: 'smsapi' }
				})
			)
		).toBe('zmienione ustawienia: 2, tryb testowy wyłączony, operator SMS: SMSAPI');
	});
	it('leaves unknown detail keys out instead of showing English', () => {
		expect(detailsText(entry({ details: { somethingNew: 'x' } }))).toBe('');
	});
	it('links an entry to what it is about', () => {
		expect(entityHref({ entity: 'batch', entityId: 'b1' })).toBe('/historia/b1');
		expect(entityHref({ entity: 'recipient', entityId: 'r1' })).toBe('/odbiorcy/r1');
		expect(entityHref({ entity: 'recipients', entityId: null })).toBeNull();
	});
	it('describes a confirmed batch by its size only', () => {
		expect(
			detailsText(
				entry({ action: 'batch_sent', details: { recipients: '9', groups: '1', smsParts: '7' } })
			)
		).toBe('odbiorcy: 9, grupy: 1, części SMS: 7');
	});
	it('describes a contact fix by its channels only', () => {
		expect(actionText('recipient_contact_fixed')).toBe('Uzupełnienie kontaktu odbiorcy');
		expect(
			detailsText(entry({ action: 'recipient_contact_fixed', details: { channels: 'email,sms' } }))
		).toBe('kanały: e-mail, SMS');
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
	it('shows the provider text when SendGrid rejects a sender', () => {
		const got = senderFailureOf(
			apiError(422, { code: 'sender_rejected', message: 'from_email: already exists' })
		);
		expect(got).toEqual({
			status: 422,
			failure: 'SendGrid odrzucił zgłoszenie: from_email: already exists'
		});
	});
	it('hides provider outages behind a retry hint', () => {
		const got = senderFailureOf(apiError(502, { code: 'provider_error', message: 'dial tcp' }));
		expect(got.status).toBe(502);
		expect(got.failure).not.toMatch(/dial/);
		expect(senderFailureOf(new TypeError('fetch failed')).status).toBe(500);
	});
	it('does not reveal why a login failed', () => {
		const got = loginFailureText(apiError(401, { code: 'invalid_credentials', message: 'x' }));
		expect(got.text).toMatch(/e-mail lub hasło/);
	});
	it('treats a dead backend as unavailable', () => {
		expect(loginFailureText(new TypeError('fetch failed')).status).toBe(503);
	});
});
