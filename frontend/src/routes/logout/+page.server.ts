import { redirect } from '@sveltejs/kit';
import { logout, SESSION_COOKIE } from '$lib/api/auth';
import type { Actions, PageServerLoad } from './$types';

// Only the POST action logs out; a GET (a prefetched link) must not.
export const load: PageServerLoad = () => redirect(303, '/');

export const actions: Actions = {
	default: async ({ fetch, cookies }) => {
		try {
			await logout(fetch);
		} catch {
			// An already ended session or a backend hiccup: the cookie goes anyway.
		}
		cookies.delete(SESSION_COOKIE, { path: '/' });
		redirect(303, '/login');
	}
};
