// Polish texts about a batch shared by the composer and the history pages.
import type { Channel } from '$lib/api/recipients';

export type ChannelMode = 'both' | 'email' | 'sms';

/**
 * The channel choice of a batch. A batch recorded before the channels were
 * stored has none listed and went out on both.
 */
export function channelModeOf(channels: readonly Channel[] | undefined | null): ChannelMode {
	if (!channels || channels.length === 0) return 'both';
	const email = channels.includes('email');
	const sms = channels.includes('sms');
	if (email && !sms) return 'email';
	if (sms && !email) return 'sms';
	return 'both';
}

export const channelModeLabels: Record<ChannelMode, string> = {
	both: 'E-mail i SMS',
	email: 'Tylko e-mail',
	sms: 'Tylko SMS'
};

/** Whether a batch used the channel at all. */
export function usesChannel(
	channels: readonly Channel[] | undefined | null,
	channel: Channel
): boolean {
	const mode = channelModeOf(channels);
	return mode === 'both' || mode === channel;
}

/** How many characters of the body stand in for a missing subject. */
const TITLE_BODY_CHARS = 60;

/**
 * What to call a batch on screen. An SMS-only batch may have no subject, so
 * the start of its body is shown instead; the body itself is never changed.
 */
export function batchTitle(batch: { subject: string; body?: string }): string {
	const subject = batch.subject.trim();
	if (subject !== '') return subject;
	const body = (batch.body ?? '').trim().replace(/\s+/g, ' ');
	if (body === '') return '(SMS bez tematu)';
	const cut = body.length > TITLE_BODY_CHARS ? `${body.slice(0, TITLE_BODY_CHARS)}…` : body;
	return `SMS: ${cut}`;
}
