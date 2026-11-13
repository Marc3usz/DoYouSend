// Server-only access to the Go backend. In the browser /api/* goes through the
// Vite dev proxy under `npm run dev` and through routes/api/[...path] in a
// build; on the server load functions reach it through handleFetch. All three
// use this origin.
import { env } from '$env/dynamic/public';

/** The backend origin, PUBLIC_API_URL without a trailing slash. */
export function apiOrigin(): string {
	return (env.PUBLIC_API_URL || 'http://localhost:8080').replace(/\/+$/, '');
}

/**
 * Request headers worth passing to the backend. Hop-by-hop headers and the
 * browser's Host would confuse it; everything the API reads (cookie,
 * Content-Type with the multipart boundary, Idempotency-Key, Accept) stays.
 */
const droppedRequestHeaders = new Set([
	'host',
	'connection',
	'keep-alive',
	'transfer-encoding',
	'upgrade',
	'content-length',
	'accept-encoding'
]);

export function forwardedHeaders(source: Headers): Headers {
	const headers = new Headers();
	source.forEach((value, name) => {
		if (!droppedRequestHeaders.has(name.toLowerCase())) headers.set(name, value);
	});
	return headers;
}

/** Response headers the browser must not get from a relayed body. */
const droppedResponseHeaders = new Set([
	'connection',
	'keep-alive',
	'transfer-encoding',
	'content-encoding',
	'content-length'
]);

export function relayedHeaders(source: Headers): Headers {
	const headers = new Headers();
	source.forEach((value, name) => {
		if (!droppedResponseHeaders.has(name.toLowerCase())) headers.append(name, value);
	});
	// Headers.forEach folds several Set-Cookie into one line; keep them apart.
	const cookies = source.getSetCookie?.() ?? [];
	if (cookies.length > 0) {
		headers.delete('set-cookie');
		for (const c of cookies) headers.append('set-cookie', c);
	}
	return headers;
}
