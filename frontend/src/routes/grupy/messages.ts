// Polish texts of the group screens. The backend sends stable codes and field
// keys; its English message is never shown on its own.
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';
import type { Group } from '$lib/api/groups';
import { errorTexts as recipientErrorTexts } from '../odbiorcy/messages';

export const errorTexts = {
	unavailable: recipientErrorTexts.unavailable,
	unexpected: recipientErrorTexts.unexpected,
	notFound: 'Nie ma takiej grupy — mogła zostać usunięta.',
	systemGroup:
		'Grupy systemowej nie można zmienić: jej skład wynika z typu odbiorcy (rodzic albo uczeń).',
	unknownRecipient:
		'Części wybranych osób nie ma już w bazie — nikogo nie dodano. Odśwież wyszukiwanie i spróbuj ponownie.',
	noneSelected: 'Zaznacz co najmniej jedną osobę.'
} as const;

export function kindLabel(kind: Group['kind']): string {
	return kind === 'system' ? 'systemowa' : 'własna';
}

/** "3 osoby", "1 osoba", "5 osób". */
export function peopleCount(n: number): string {
	const lastTwo = n % 100;
	const last = n % 10;
	if (n === 1) return '1 osoba';
	if (last >= 2 && last <= 4 && (lastTwo < 12 || lastTwo > 14)) return `${n} osoby`;
	return `${n} osób`;
}

/** The text for a failed group request; see odbiorcy/messages.ts failureText. */
export function failureText(err: unknown): string {
	if (!(err instanceof ApiError)) return errorTexts.unavailable;
	switch (errorBody(err)?.code) {
		case undefined:
			return err.status >= 500 || err.status === 404
				? errorTexts.unavailable
				: errorTexts.unexpected;
		case 'not_found':
			return errorTexts.notFound;
		case 'system_group':
			return errorTexts.systemGroup;
		case 'unknown_recipient':
			return errorTexts.unknownRecipient;
		default:
			return errorTexts.unexpected;
	}
}

export function isNotFound(err: unknown): boolean {
	return errorBody(err)?.code === 'not_found';
}

export type GroupFormValues = { name: string; description: string };

export type GroupFormErrors = Partial<Record<keyof GroupFormValues, string>>;

export function readGroupForm(data: FormData): GroupFormValues {
	const text = (key: string) => {
		const value = data.get(key);
		return typeof value === 'string' ? value : '';
	};
	return { name: text('name'), description: text('description') };
}

/** What a rejected save hands back to the form. */
export type GroupSaveFailure = { status: number; errors: GroupFormErrors; failure: string | null };

export function groupSaveFailure(err: unknown): GroupSaveFailure {
	const status =
		err instanceof ApiError && err.status >= 400 && err.status < 600 ? err.status : 503;
	const body = errorBody(err);
	if (body?.code === 'name_taken') {
		return { status, errors: { name: 'Grupa o tej nazwie już istnieje.' }, failure: null };
	}
	if (body?.code === 'invalid_input') {
		const errors: GroupFormErrors = {};
		for (const { field } of body.fields) {
			if (field === 'name') errors.name = 'Wpisz nazwę (do 100 znaków, bez znaków sterujących).';
			if (field === 'description') errors.description = 'Opis może mieć najwyżej 500 znaków.';
		}
		return { status, errors, failure: null };
	}
	return { status, errors: {}, failure: failureText(err) };
}
