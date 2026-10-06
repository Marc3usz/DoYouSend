// The ErrorBody schema of docs/api/openapi.yaml, shared by every domain.
import { ApiError } from './client';

export const errorCodes = [
	'invalid_request',
	'invalid_input',
	'empty_selection',
	'not_found',
	'duplicate_contact',
	'recipient_in_use',
	'name_taken',
	'system_group',
	'unknown_recipient',
	'internal'
] as const;

export type ErrorCode = (typeof errorCodes)[number];

export type ErrorFieldDetail = { field: string; message: string };

export type ErrorBody = {
	code: ErrorCode;
	/** English diagnostic detail; never shown to the user on its own. */
	message: string;
	fields: ErrorFieldDetail[];
	ids: string[];
};

/** Returns the ErrorBody the backend sent with err, if it sent one. */
export function errorBody(err: unknown): ErrorBody | undefined {
	if (!(err instanceof ApiError) || typeof err.body !== 'object' || err.body === null) {
		return undefined;
	}
	const { code, message, fields, ids } = err.body as Record<string, unknown>;
	const known: readonly string[] = errorCodes;
	if (typeof code !== 'string' || !known.includes(code) || typeof message !== 'string') {
		return undefined;
	}
	return {
		code: code as ErrorCode,
		message,
		fields: Array.isArray(fields) ? fields.filter(isFieldDetail) : [],
		ids: Array.isArray(ids) ? ids.filter((id): id is string => typeof id === 'string') : []
	};
}

function isFieldDetail(value: unknown): value is ErrorFieldDetail {
	if (typeof value !== 'object' || value === null) return false;
	const { field, message } = value as Record<string, unknown>;
	return typeof field === 'string' && typeof message === 'string';
}
