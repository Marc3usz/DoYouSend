import { error, redirect, type Handle, type HandleFetch } from '@sveltejs/kit';
import { ApiError } from '$lib/api/client';
import { SESSION_COOKIE, me, type User } from '$lib/api/auth';
import { canOpen } from '$lib/access';
import { apiOrigin } from '$lib/server/backend';

// Every page knows who is logged in. The backend decides access to the API
// on its own (iam middleware); this only keeps pages from rendering for
// someone who could not use them anyway.
export const handle: Handle = async ({ event, resolve }) => {
	const session = await currentUser(event.cookies.get(SESSION_COOKIE), event.fetch);
	event.locals.user = session.user;

	const path = event.url.pathname;
	// The /api passthrough answers for itself: the backend checks the session.
	if (path.startsWith('/api/')) return resolve(event);

	const access = canOpen(event.locals.user, path);
	if (access === 'login') {
		const params = new URLSearchParams();
		if (path !== '/') params.set('next', path + event.url.search);
		// Tell the login page why: a dead backend is not a logged-out user.
		if (session.backendDown) params.set('powod', 'serwer');
		redirect(303, params.size > 0 ? `/login?${params}` : '/login');
	}
	if (access === 'forbidden') {
		error(403, 'Ta strona jest dostępna tylko dla administratora.');
	}
	return resolve(event);
};

async function currentUser(
	token: string | undefined,
	fetch: typeof globalThis.fetch
): Promise<{ user: User | null; backendDown: boolean }> {
	if (!token) return { user: null, backendDown: false };
	try {
		return { user: await me(fetch), backendDown: false };
	} catch (err) {
		// 401: the session expired. Anything else: the backend is not answering.
		const expired = err instanceof ApiError && err.status < 500;
		return { user: null, backendDown: !expired };
	}
}

// Server-side load functions and form actions do not go through the Vite dev
// proxy: SvelteKit would look for its own /api route. This hook sends those
// requests straight to the backend, with the session cookie of the page.
export const handleFetch: HandleFetch = async ({ event, request, fetch }) => {
	const url = new URL(request.url);
	if (url.origin !== event.url.origin || !url.pathname.startsWith('/api/')) {
		return fetch(request);
	}
	const headers = new Headers(request.headers);
	const token = event.cookies.get(SESSION_COOKIE);
	if (token) headers.set('cookie', `${SESSION_COOKIE}=${token}`);
	const hasBody = request.method !== 'GET' && request.method !== 'HEAD';
	return fetch(
		new Request(apiOrigin() + url.pathname + url.search, {
			method: request.method,
			headers,
			body: hasBody ? await request.arrayBuffer() : undefined
		})
	);
};
