import { fail } from '@sveltejs/kit';
import { createUser, listUsers, resetPassword, updateUser } from '$lib/api/admin';
import type { Actions, PageServerLoad } from './$types';
import { asRole, failureOf, readText } from './users';

export const load: PageServerLoad = async ({ fetch }) => {
	try {
		return { users: await listUsers(fetch), loadFailed: false };
	} catch {
		return { users: [], loadFailed: true };
	}
};

export const actions: Actions = {
	create: async ({ request, fetch }) => {
		const data = await request.formData();
		const values = {
			email: readText(data, 'email'),
			fullName: readText(data, 'fullName'),
			role: readText(data, 'role')
		};
		try {
			const created = await createUser(
				{ ...values, role: asRole(values.role) ?? 'sender', password: readText(data, 'password') },
				fetch
			);
			return { action: 'create' as const, done: `Utworzono konto ${created.email}.` };
		} catch (err) {
			const { status, fields, failure } = failureOf(err);
			return fail(status, { action: 'create' as const, values, fields, failure });
		}
	},

	update: async ({ request, fetch }) => {
		const data = await request.formData();
		const id = readText(data, 'id');
		const role = asRole(readText(data, 'role'));
		const disabled = readText(data, 'disabled');
		try {
			const updated = await updateUser(
				id,
				{
					...(role ? { role } : {}),
					...(disabled === 'true' || disabled === 'false' ? { disabled: disabled === 'true' } : {})
				},
				fetch
			);
			return { action: 'update' as const, id, done: `Zapisano zmiany konta ${updated.email}.` };
		} catch (err) {
			const { status, failure } = failureOf(err);
			return fail(status, { action: 'update' as const, id, failure });
		}
	},

	password: async ({ request, fetch }) => {
		const data = await request.formData();
		const id = readText(data, 'id');
		try {
			await resetPassword(id, readText(data, 'password'), fetch);
			return {
				action: 'password' as const,
				id,
				done: 'Ustawiono nowe hasło. Użytkownik został wylogowany ze wszystkich urządzeń.'
			};
		} catch (err) {
			const { status, fields, failure } = failureOf(err);
			return fail(status, { action: 'password' as const, id, failure: fields.password ?? failure });
		}
	}
};
