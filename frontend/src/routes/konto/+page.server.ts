import { fail } from '@sveltejs/kit';
import { changePassword } from '$lib/api/auth';
import { mySenders } from '$lib/api/senders';
import type { Actions, PageServerLoad } from './$types';
import { checkPasswordForm, passwordFailureOf } from './account';

export const load: PageServerLoad = async ({ fetch }) => {
	// Read-only: which addresses this user may send e-mail from.
	const senders = await mySenders(fetch).catch(() => null);
	return { senders };
};

const text = (data: FormData, name: string) => {
	const v = data.get(name);
	return typeof v === 'string' ? v : '';
};

export const actions: Actions = {
	password: async ({ request, fetch }) => {
		const data = await request.formData();
		const input = {
			currentPassword: text(data, 'currentPassword'),
			newPassword: text(data, 'newPassword'),
			repeat: text(data, 'repeat')
		};
		const fields = checkPasswordForm(input);
		if (Object.keys(fields).length > 0) {
			return fail(400, { fields, failure: null });
		}
		try {
			await changePassword(input.currentPassword, input.newPassword, fetch);
			return {
				done: 'Hasło zostało zmienione. Na innych urządzeniach trzeba będzie zalogować się ponownie.'
			};
		} catch (err) {
			const { status, fields, failure } = passwordFailureOf(err);
			return fail(status, { fields, failure });
		}
	}
};
