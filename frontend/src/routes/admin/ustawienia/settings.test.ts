import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import type { SettingItem, Settings } from '$lib/api/admin';
import { collectChanges, keptValues, settingsFailureOf } from './settings';

const item = (over: Partial<SettingItem>): SettingItem => ({
	key: 'sms_sender_name',
	group: 'sms',
	kind: 'text',
	value: 'SZKOLA-TEST',
	set: true,
	source: 'env',
	envVar: 'SMS_SENDER_NAME',
	unreadable: false,
	...over
});

const current: Settings = {
	dryRun: true,
	dryRunLocked: false,
	sendsExternally: false,
	items: [
		item({}),
		item({ key: 'sms_price_per_part_pln', kind: 'price', value: '0.08', source: 'panel' }),
		item({ key: 'sms_api_key', kind: 'secret', value: null, set: true, source: 'panel' }),
		item({ key: 'sendgrid_api_key', kind: 'secret', value: null, set: false, source: 'default' }),
		item({ key: 'dry_run', group: 'general', kind: 'bool', value: 'true' })
	]
};

const form = (entries: Record<string, string>) => {
	const d = new FormData();
	for (const [k, v] of Object.entries(entries)) d.set(k, v);
	return d;
};

describe('collectChanges', () => {
	it('sends only what changed, so .env values are not copied into the panel', () => {
		const got = collectChanges(
			form({
				sms_sender_name: 'SZKOLA-TEST',
				sms_price_per_part_pln: ' 0.09 ',
				dry_run: 'true',
				sms_api_key: '',
				sendgrid_api_key: ''
			}),
			current
		);
		expect(got).toEqual({ sms_price_per_part_pln: '0.09' });
	});
	it('sets a typed secret and resets panel values to null', () => {
		const got = collectChanges(
			form({
				sendgrid_api_key: ' SG.x ',
				'reset:sms_api_key': 'on',
				'reset:sms_sender_name': 'on'
			}),
			current
		);
		// sms_sender_name comes from .env: there is nothing to reset.
		expect(got).toEqual({ sendgrid_api_key: 'SG.x', sms_api_key: null });
	});
	it('never keeps secrets to show them again', () => {
		expect(keptValues(form({ sms_api_key: 'secret', sms_sender_name: 'A' }), current)).toEqual({
			sms_sender_name: 'A'
		});
	});
});

describe('settingsFailureOf', () => {
	const apiError = (status: number, body: object) => new ApiError(status, 'x', body);
	it('asks for confirmation of real sending', () => {
		const got = settingsFailureOf(apiError(409, { code: 'confirmation_required', message: 'x' }));
		expect(got.needsConfirmation).toBe(true);
		expect(got.failure).toMatch(/WYSYŁAJ/);
	});
	it('explains a configuration the operator cannot start with', () => {
		const got = settingsFailureOf(
			apiError(400, {
				code: 'invalid_input',
				message: 'x',
				fields: [
					{ field: 'config', message: 'SENDGRID_API_KEY is required' },
					{ field: 'sms_price_per_part_pln', message: 'x' }
				]
			})
		);
		expect(got.failure).toMatch(/SENDGRID_API_KEY is required/);
		expect(got.fields.sms_price_per_part_pln).toMatch(/0\.08/);
	});
	it('explains the server lock', () => {
		expect(
			settingsFailureOf(apiError(409, { code: 'dry_run_locked', message: 'x' })).failure
		).toMatch(/DRY_RUN_LOCK/);
	});
});
