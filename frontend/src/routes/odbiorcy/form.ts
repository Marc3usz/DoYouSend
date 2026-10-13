// The add/edit recipient form: reading the submitted values and turning
// backend errors into per-field Polish messages.
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';
import type { Recipient, RecipientInput } from '$lib/api/recipients';
import { failureText } from './messages';

export type RecipientFormValues = {
	firstName: string;
	lastName: string;
	email: string;
	phone: string;
	type: string;
	/** Classes as typed, comma-separated: "3A" or "1B, 3A". */
	classes: string;
};

export type FormField = keyof RecipientFormValues | 'contact';

export type RecipientFormErrors = {
	fields: Partial<Record<FormField, string>>;
	/** Set when the e-mail or phone belongs to another recipient. */
	duplicateOf?: string;
};

export function emptyValues(): RecipientFormValues {
	return { firstName: '', lastName: '', email: '', phone: '', type: '', classes: '' };
}

export function valuesOf(r: Recipient): RecipientFormValues {
	return {
		firstName: r.firstName,
		lastName: r.lastName,
		email: r.email ?? '',
		phone: r.phone ?? '',
		type: r.type,
		classes: r.classes.join(', ')
	};
}

export function readForm(data: FormData): RecipientFormValues {
	const text = (name: string) => {
		const value = data.get(name);
		return typeof value === 'string' ? value : '';
	};
	return {
		firstName: text('firstName'),
		lastName: text('lastName'),
		email: text('email'),
		phone: text('phone'),
		type: text('type'),
		classes: text('classes')
	};
}

/** The request body: blank contacts are null, the type is passed on for the backend to check. */
export function toInput(v: RecipientFormValues): RecipientInput {
	return {
		firstName: v.firstName,
		lastName: v.lastName,
		email: v.email.trim() === '' ? null : v.email,
		phone: v.phone.trim() === '' ? null : v.phone,
		type: v.type === 'parent' || v.type === 'student' ? v.type : '',
		classes: splitClasses(v.classes)
	};
}

/** "1b, 3A" -> ["1b", "3A"]; normalizing and checking is the backend's job. */
export function splitClasses(raw: string): string[] {
	return raw
		.split(/[,;]/)
		.map((c) => c.trim())
		.filter((c) => c !== '');
}

// Backend field keys (openapi.yaml ErrorBody.fields) -> form fields and texts.
const fieldTexts: Record<string, [FormField, string]> = {
	first_name: ['firstName', 'Wpisz imię.'],
	last_name: ['lastName', 'Wpisz nazwisko.'],
	type: ['type', 'Wybierz, czy to rodzic, czy uczeń.'],
	email: ['email', 'To nie wygląda na poprawny adres e-mail.'],
	phone: ['phone', 'Wpisz numer w formacie +48 500 100 101 albo 500 100 101.'],
	contact: ['contact', 'Podaj e-mail albo telefon — bez nich nie da się wysłać wiadomości.'],
	classes: [
		'classes',
		'Wpisz klasy jak „3A” albo „1B, 3A” (cyfra i litery). Uczeń może mieć tylko jedną klasę.'
	]
};

const duplicateTexts: Record<string, [FormField, string]> = {
	email: ['email', 'Ten adres e-mail ma już inny odbiorca.'],
	phone: ['phone', 'Ten numer telefonu ma już inny odbiorca.']
};

/**
 * Field errors for a rejected save, or undefined when err is not a
 * validation or duplicate error (the caller then shows a generic message).
 */
export function formErrors(err: unknown): RecipientFormErrors | undefined {
	const body = errorBody(err);
	if (body?.code === 'invalid_input') {
		const fields: RecipientFormErrors['fields'] = {};
		for (const { field } of body.fields) {
			const known = fieldTexts[field];
			if (known && !fields[known[0]]) fields[known[0]] = known[1];
		}
		return { fields };
	}
	if (body?.code === 'duplicate_contact') {
		const fields: RecipientFormErrors['fields'] = {};
		for (const { field } of body.fields) {
			const known = duplicateTexts[field];
			if (known) fields[known[0]] = known[1];
		}
		return { fields, duplicateOf: body.ids[0] };
	}
	return undefined;
}

/** What a rejected save hands back to the form: per-field texts, or one general text. */
export type SaveFailure = {
	status: number;
	errors: RecipientFormErrors;
	failure: string | null;
};

export function saveFailure(err: unknown): SaveFailure {
	const status =
		err instanceof ApiError && err.status >= 400 && err.status < 600 ? err.status : 503;
	const errors = formErrors(err);
	if (errors) return { status, errors, failure: null };
	return { status, errors: { fields: {} }, failure: failureText(err) };
}
