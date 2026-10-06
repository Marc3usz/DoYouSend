import { fail, redirect } from '@sveltejs/kit';
import { createGroup, listGroups, type Group } from '$lib/api/groups';
import type { Actions, PageServerLoad } from './$types';
import { failureText, groupSaveFailure, readGroupForm } from './messages';

export const load: PageServerLoad = async ({ fetch }) => {
	try {
		return { groups: await listGroups(fetch), failure: null };
	} catch (err) {
		return { groups: [], failure: failureText(err) };
	}
};

export const actions: Actions = {
	create: async ({ request, fetch }) => {
		const values = readGroupForm(await request.formData());
		let created: Group;
		try {
			created = await createGroup(values, fetch);
		} catch (err) {
			const { status, errors, failure } = groupSaveFailure(err);
			return fail(status, { values, errors, failure });
		}
		redirect(303, `/grupy/${created.id}?utworzono`);
	}
};
