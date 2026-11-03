import { fail } from '@sveltejs/kit';
import {
	createUser,
	listSenders,
	listUsers,
	removeSender,
	requestSender,
	resendSenderVerification,
	resetPassword,
	senderStatus,
	updateUser,
	type SenderIdentityList
} from '$lib/api/admin';
import type { Actions, PageServerLoad } from './$types';
import { asRole, failureOf, readText, senderFailureOf, senderStatusLabels } from './users';

export const load: PageServerLoad = async ({ fetch }) => {
	// The sender list is an extra: the user table still works without it.
	const sendersPromise = listSenders(fetch).catch((): SenderIdentityList | null => null);
	try {
		const [users, senders] = await Promise.all([listUsers(fetch), sendersPromise]);
		return { users, senders, loadFailed: false };
	} catch {
		return { users: [], senders: null, loadFailed: true };
	}
};

/** Runs one sender-identity action for the user in the form and reports it on that row. */
async function senderAction(request: Request, run: (id: string) => Promise<string>) {
	const id = readText(await request.formData(), 'id');
	try {
		return { action: 'sender' as const, id, done: await run(id) };
	} catch (err) {
		const { status, failure } = senderFailureOf(err);
		return fail(status, { action: 'sender' as const, id, failure });
	}
}

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
	},

	senderRequest: async ({ request, fetch }) =>
		senderAction(request, async (id) => {
			const s = await requestSender(id, fetch);
			return s.provider === 'fake'
				? 'Zgłoszono nadawcę w trybie testowym (DRY_RUN) — żaden e-mail nie został wysłany.'
				: `SendGrid wysłał link potwierdzający na ${s.fromEmail}.`;
		}),

	senderCheck: async ({ request, fetch }) =>
		senderAction(request, async (id) => {
			const s = await senderStatus(id, fetch);
			return `Stan nadawcy: ${senderStatusLabels[s.status]}.`;
		}),

	senderResend: async ({ request, fetch }) =>
		senderAction(request, async (id) => {
			await resendSenderVerification(id, fetch);
			return 'Wysłano ponownie link potwierdzający.';
		}),

	senderRemove: async ({ request, fetch }) =>
		senderAction(request, async (id) => {
			await removeSender(id, fetch);
			return 'Usunięto nadawcę.';
		})
};
