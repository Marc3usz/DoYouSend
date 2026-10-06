import { fail, redirect } from '@sveltejs/kit';
import { createRecipient, type Recipient } from '$lib/api/recipients';
import type { Actions } from './$types';
import { readForm, saveFailure, toInput } from '../form';

export const actions: Actions = {
	default: async ({ request, fetch }) => {
		const values = readForm(await request.formData());
		let created: Recipient;
		try {
			created = await createRecipient(toInput(values), fetch);
		} catch (err) {
			const { status, errors, failure } = saveFailure(err);
			return fail(status, { values, errors, failure });
		}
		redirect(303, `/odbiorcy/${created.id}?dodano`);
	}
};
