import type { HandleFetch } from '@sveltejs/kit';
import { env } from '$env/dynamic/public';

// In the browser /api/* reaches the Go backend through the Vite dev proxy
// (vite.config.ts). Server-side load functions and form actions do not go
// through that proxy: SvelteKit would look for an /api route of its own and
// answer 404. This hook sends those requests straight to the backend.
//
// TODO(iam): forward the session cookie once logging in exists (DEV D).
export const handleFetch: HandleFetch = async ({ event, request, fetch }) => {
	const url = new URL(request.url);
	if (url.origin !== event.url.origin || !url.pathname.startsWith('/api/')) {
		return fetch(request);
	}
	const apiOrigin = (env.PUBLIC_API_URL || 'http://localhost:8080').replace(/\/+$/, '');
	const hasBody = request.method !== 'GET' && request.method !== 'HEAD';
	return fetch(
		new Request(apiOrigin + url.pathname + url.search, {
			method: request.method,
			headers: request.headers,
			body: hasBody ? await request.arrayBuffer() : undefined
		})
	);
};
