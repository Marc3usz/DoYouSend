import { error, redirect, type Handle, type HandleFetch } from '@sveltejs/kit';
import { env } from '$env/dynamic/public';
import { SESSION_COOKIE, type User } from '$lib/api/auth';
import { canOpen } from '$lib/access';

function apiOrigin(): string {
	return (env.PUBLIC_API_URL || 'http://localhost:8080').replace(/\/+$/, '');
}

// Every page knows who is logged in. The backend decides access to the API
// on its own (iam middleware); this only keeps pages from rendering for
// someone who could not use them anyway.
export const handle: Handle = async ({ event, resolve }) => {
	event.locals.user = await currentUser(event.cookies.get(SESSION_COOKIE));

	const path = event.url.pathname;
	const access = canOpen(event.locals.user, path);
	if (access === 'login') {
		const next = path === '/' ? '' : `?next=${encodeURIComponent(path + event.url.search)}`;
		redirect(303, `/login${next}`);
	}
	if (access === 'forbidden') {
		error(403, 'Ta strona jest dostępna tylko dla administratora.');
	}
	return resolve(event);
};

async function currentUser(token: string | undefined): Promise<User | null> {
	if (!token) return null;
	try {
		const response = await fetch(`${apiOrigin()}/api/auth/me`, {
			headers: { cookie: `${SESSION_COOKIE}=${token}` }
		});
		return response.ok ? ((await response.json()) as User) : null;
	} catch {
		// Backend down: treat as logged out rather than failing every page.
		return null;
	}
}

// In the browser /api/* reaches the Go backend through the Vite dev proxy
// (vite.config.ts). Server-side load functions and form actions do not go
// through that proxy: SvelteKit would look for an /api route of its own and
// answer 404. This hook sends those requests straight to the backend, with
// the session cookie of the page request.
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
