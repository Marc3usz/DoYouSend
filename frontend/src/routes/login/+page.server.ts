import { fail, redirect } from '@sveltejs/kit';
import { dev } from '$app/environment';
import { login, SESSION_COOKIE } from '$lib/api/auth';
import { nextPath } from '$lib/access';
import type { Actions, PageServerLoad } from './$types';
import { loginFailureText } from './messages';

export const load: PageServerLoad = ({ locals, url }) => {
	// Already logged in: the login form has nothing to offer.
	if (locals.user) redirect(303, nextPath(url.searchParams.get('next')));
	return {};
};

export const actions: Actions = {
	default: async ({ request, fetch, cookies, url }) => {
		const data = await request.formData();
		const email = String(data.get('email') ?? '');
		const password = String(data.get('password') ?? '');
		if (email.trim() === '' || password === '') {
			return fail(400, { email, failure: 'Wpisz e-mail i hasło.' });
		}

		let session;
		try {
			session = await login(email, password, fetch);
		} catch (err) {
			const { status, text } = loginFailureText(err);
			return fail(status, { email, failure: text });
		}
		cookies.set(SESSION_COOKIE, session.token, {
			path: '/',
			httpOnly: true,
			sameSite: 'lax',
			secure: !dev,
			...(session.expires ? { expires: session.expires } : {})
		});
		redirect(303, nextPath(url.searchParams.get('next')));
	}
};
