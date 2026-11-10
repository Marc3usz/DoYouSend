import { error, fail, redirect } from '@sveltejs/kit';
import { ApiError } from '$lib/api/client';
import {
	addGroupMembers,
	deleteGroup,
	getGroup,
	listGroupMembers,
	removeGroupMembers,
	updateGroup
} from '$lib/api/groups';
import { lookupRecipients, type RecipientLookupPage } from '$lib/api/recipients';
import type { Actions, PageServerLoad } from './$types';
import { errorTexts, failureText, groupSaveFailure, isNotFound, readGroupForm } from '../messages';

/** Rows per page of the picker for adding people. */
const PICKER_PAGE_SIZE = 20;

export const load: PageServerLoad = async ({ params, fetch }) => {
	try {
		const [group, members] = await Promise.all([
			getGroup(params.id, fetch),
			listGroupMembers(params.id, fetch)
		]);
		// The picker's first page; on failure the picker shows an error and a retry.
		let people: RecipientLookupPage | null = null;
		if (group.kind === 'custom') {
			people = await lookupRecipients({ limit: PICKER_PAGE_SIZE }, fetch).catch(() => null);
		}
		return { group, members, people };
	} catch (err) {
		// A malformed ID is a 400 invalid_request: for the user, no such group.
		if (isNotFound(err) || (err instanceof ApiError && err.status === 400)) {
			error(404, errorTexts.notFound);
		}
		error(503, failureText(err));
	}
};

const statusOf = (err: unknown) => (err instanceof ApiError ? err.status : 503);

export const actions: Actions = {
	save: async ({ params, request, fetch }) => {
		const values = readGroupForm(await request.formData());
		try {
			await updateGroup(params.id, values, fetch);
		} catch (err) {
			const { status, errors, failure } = groupSaveFailure(err);
			return fail(status, { values, errors, failure });
		}
		return { saved: true };
	},
	delete: async ({ params, fetch }) => {
		try {
			await deleteGroup(params.id, fetch);
		} catch (err) {
			return fail(statusOf(err), { deleteFailure: failureText(err) });
		}
		redirect(303, '/grupy?usunieto');
	},
	add: async ({ params, request, fetch }) => {
		const ids = (await request.formData())
			.getAll('recipientId')
			.filter((v): v is string => typeof v === 'string');
		if (ids.length === 0) return fail(400, { memberFailure: errorTexts.noneSelected });
		try {
			const change = await addGroupMembers(params.id, ids, fetch);
			return { added: change.changed };
		} catch (err) {
			return fail(statusOf(err), { memberFailure: failureText(err) });
		}
	},
	remove: async ({ params, request, fetch }) => {
		const id = (await request.formData()).get('recipientId');
		if (typeof id !== 'string' || id === '') {
			return fail(400, { memberFailure: errorTexts.unexpected });
		}
		try {
			const change = await removeGroupMembers(params.id, [id], fetch);
			return { removed: change.changed };
		} catch (err) {
			return fail(statusOf(err), { memberFailure: failureText(err) });
		}
	}
};
