// Endpoints of the messaging domain (DEV B), see docs/api/openapi.yaml.
import { api } from './client';
import type { RecipientSelection } from './groups';

export type MessageDraft = {
	/** E-mail subject only. */
	subject: string;
	/** One body for both channels, sent byte for byte: never trim or reformat it. */
	body: string;
	selection?: RecipientSelection;
};

export type SmsLength = {
	encoding: 'GSM7' | 'UCS2';
	units: number;
	parts: number;
};

export type SendEstimate = {
	template: SmsLength;
	placeholders: string[];
	unknownPlaceholders: string[];
	recipientCount: number;
	emailCount: number;
	smsCount: number;
	partialCount: number;
	unreachableCount: number;
	ucs2Count: number;
	minPartsPerRecipient: number;
	maxPartsPerRecipient: number;
	totalSmsParts: number;
	/** Thousandths of a złoty: 1000 = 1 zł. */
	costMilli: number;
	renderFailedIds: string[];
};

/** Pre-send summary of a draft: POST /messages/preview. */
export function previewMessage(
	draft: MessageDraft,
	fetch?: typeof globalThis.fetch
): Promise<SendEstimate> {
	return api<SendEstimate>('/messages/preview', { method: 'POST', body: draft, fetch });
}
