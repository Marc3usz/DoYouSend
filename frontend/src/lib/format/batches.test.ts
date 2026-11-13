import { describe, expect, it } from 'vitest';
import { batchTitle, channelModeOf, usesChannel } from './batches';

describe('channelModeOf', () => {
	it.each([
		[undefined, 'both'],
		[null, 'both'],
		[[], 'both'],
		[['email', 'sms'], 'both'],
		[['sms', 'email'], 'both'],
		[['email'], 'email'],
		[['sms'], 'sms']
	] as const)('%j -> %s', (channels, want) => {
		expect(channelModeOf(channels)).toBe(want);
	});
});

describe('usesChannel', () => {
	it('treats a batch without recorded channels as sent on both', () => {
		expect(usesChannel(undefined, 'email')).toBe(true);
		expect(usesChannel(undefined, 'sms')).toBe(true);
	});
	it('knows the channel a single-channel batch skipped', () => {
		expect(usesChannel(['sms'], 'email')).toBe(false);
		expect(usesChannel(['sms'], 'sms')).toBe(true);
		expect(usesChannel(['email'], 'sms')).toBe(false);
	});
});

describe('batchTitle', () => {
	it('uses the subject when there is one', () => {
		expect(batchTitle({ subject: ' Zebranie ', body: 'Treść' })).toBe('Zebranie');
	});
	it('falls back to the start of the body for an SMS without a subject', () => {
		expect(batchTitle({ subject: '', body: 'Zebranie\nodbędzie się jutro.' })).toBe(
			'SMS: Zebranie odbędzie się jutro.'
		);
	});
	it('cuts a long body', () => {
		const title = batchTitle({ subject: '  ', body: 'a'.repeat(200) });
		expect(title).toBe(`SMS: ${'a'.repeat(60)}…`);
	});
	it('has a placeholder when the list row carries no body', () => {
		expect(batchTitle({ subject: '' })).toBe('(SMS bez tematu)');
	});
});
