// Endpoints of the recipients domain (DEV A), see docs/api/openapi.yaml.
import { api, ApiError } from './client';

export type RecipientType = 'parent' | 'student';

export type ImportedRecipient = {
	firstName: string;
	lastName: string;
	email: string | null;
	phone: string | null;
	type: RecipientType;
	classes: string[];
};

export type ImportFieldError = {
	field:
		| 'first_name'
		| 'last_name'
		| 'type'
		| 'email'
		| 'phone'
		| 'classes'
		| 'contact'
		| 'encoding'
		| 'row';
	message: string;
};

export type ImportDuplicate = {
	row: number;
	field: 'email' | 'phone';
	duplicateOfRow?: number;
	existingRecipientId?: string;
};

export type ImportReport = {
	valid: { row: number; recipient: ImportedRecipient }[];
	invalid: { row: number; errors: ImportFieldError[] }[];
	duplicates: ImportDuplicate[];
};

export const importFileErrorCodes = [
	'invalid_request',
	'missing_file',
	'empty_file',
	'missing_columns',
	'invalid_header',
	'unsupported_format',
	'file_too_large',
	'too_many_rows',
	'unclosed_quote'
] as const;

export type ImportFileError = {
	code: (typeof importFileErrorCodes)[number];
	message: string;
};

/** Checks an import file without storing anything: POST /recipients/import/check. */
export function checkImportFile(
	file: File,
	fetch?: typeof globalThis.fetch
): Promise<ImportReport> {
	const body = new FormData();
	body.append('file', file);
	return api<ImportReport>('/recipients/import/check', { method: 'POST', body, fetch });
}

/**
 * Imports a file: POST /recipients/import. Stores the valid rows (all or
 * none) and returns the same report as the check, where valid are the rows
 * actually stored.
 */
export function importFile(file: File, fetch?: typeof globalThis.fetch): Promise<ImportReport> {
	const body = new FormData();
	body.append('file', file);
	return api<ImportReport>('/recipients/import', { method: 'POST', body, fetch });
}

/** Returns the whole-file error the backend reported, if err carries one. */
export function importFileError(err: unknown): ImportFileError | undefined {
	if (!(err instanceof ApiError) || typeof err.body !== 'object' || err.body === null) {
		return undefined;
	}
	const { code, message } = err.body as Record<string, unknown>;
	const known: readonly string[] = importFileErrorCodes;
	if (typeof code !== 'string' || !known.includes(code) || typeof message !== 'string') {
		return undefined;
	}
	return { code: code as ImportFileError['code'], message };
}

export type Channel = 'email' | 'sms';

export type ContactIssue = { channel: Channel; reason: 'missing' | 'invalid' };

export type Recipient = {
	id: string;
	firstName: string;
	lastName: string;
	email: string | null;
	phone: string | null;
	type: RecipientType;
	/** Normalized class names (ADR-0009): a student's class, or the classes of a parent's children. */
	classes: string[];
	/** Custom groups; only in getRecipient. */
	groupIds?: string[];
	/** Channels the recipient cannot be reached on; empty when both work. */
	issues: ContactIssue[];
	createdAt: string;
	updatedAt: string;
};

export type RecipientInput = {
	firstName: string;
	lastName: string;
	email: string | null;
	phone: string | null;
	/** '' lets the backend report a missing type together with the other field errors. */
	type: RecipientType | '';
	/** Replaces the classes; the backend normalizes "3 a" to "3A". */
	classes: string[];
};

export type RecipientPage = { items: Recipient[]; total: number };

export type RecipientListQuery = {
	q?: string;
	type?: RecipientType;
	/** Only recipients of this class (ADR-0009). */
	class?: string;
	issue?: Channel;
	limit?: number;
	offset?: number;
};

/** GET /recipients: one page, sorted by last name and first name. */
export function listRecipients(
	query: RecipientListQuery,
	fetch?: typeof globalThis.fetch
): Promise<RecipientPage> {
	const params = new URLSearchParams();
	for (const [key, value] of Object.entries(query)) {
		if (value !== undefined && value !== '') params.set(key, String(value));
	}
	const search = params.size > 0 ? `?${params}` : '';
	return api<RecipientPage>(`/recipients${search}`, { fetch });
}

export function getRecipient(id: string, fetch?: typeof globalThis.fetch): Promise<Recipient> {
	return api<Recipient>(`/recipients/${encodeURIComponent(id)}`, { fetch });
}

export function createRecipient(
	input: RecipientInput,
	fetch?: typeof globalThis.fetch
): Promise<Recipient> {
	return api<Recipient>('/recipients', { method: 'POST', body: input, fetch });
}

export function updateRecipient(
	id: string,
	input: RecipientInput,
	fetch?: typeof globalThis.fetch
): Promise<Recipient> {
	return api<Recipient>(`/recipients/${encodeURIComponent(id)}`, {
		method: 'PUT',
		body: input,
		fetch
	});
}

export function deleteRecipient(id: string, fetch?: typeof globalThis.fetch): Promise<void> {
	return api<void>(`/recipients/${encodeURIComponent(id)}`, { method: 'DELETE', fetch });
}
