import { error, fail, redirect } from '@sveltejs/kit';
import { ApiError } from '$lib/api/client';
import { deleteRecipient, getRecipient, updateRecipient } from '$lib/api/recipients';
import type { Actions, PageServerLoad } from './$types';
import { readForm, saveFailure, toInput } from '../form';
import { errorTexts, failureText, isNotFound } from '../messages';

export const load: PageServerLoad = async ({ params, fetch }) => {
	try {
		return { recipient: await getRecipient(params.id, fetch) };
	} catch (err) {
		// A malformed ID is answered with 400 invalid_input: for the user it is
		// the same as a recipient that does not exist.
		if (isNotFound(err) || (err instanceof ApiError && err.status === 400)) {
			error(404, errorTexts.notFound);
		}
		error(503, failureText(err));
	}
};

export const actions: Actions = {
	save: async ({ params, request, fetch }) => {
		const values = readForm(await request.formData());
		try {
			await updateRecipient(params.id, toInput(values), fetch);
		} catch (err) {
			const { status, errors, failure } = saveFailure(err);
			return fail(status, { values, errors, failure });
		}
		return { saved: true };
	},
	delete: async ({ params, fetch }) => {
		try {
			await deleteRecipient(params.id, fetch);
		} catch (err) {
			const status = err instanceof ApiError ? err.status : 503;
			return fail(status, { deleteFailure: failureText(err) });
		}
		redirect(303, '/odbiorcy?usunieto');
	}
};
