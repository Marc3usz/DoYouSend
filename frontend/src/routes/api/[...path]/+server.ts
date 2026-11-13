// Passes browser calls to /api/* on to the Go backend, so the built app
// (adapter-node) works without a separate reverse proxy. Under `npm run dev`
// the Vite proxy answers these paths first and this route is not reached.
import type { RequestHandler } from './$types';
import { apiOrigin, forwardedHeaders, relayedHeaders } from '$lib/server/backend';

// The global fetch, not event.fetch: this is a plain hop to another origin and
// must carry the browser's cookie header exactly as received.
const relay: RequestHandler = async ({ request, url }) => {
	const hasBody = request.method !== 'GET' && request.method !== 'HEAD';
	const response = await fetch(`${apiOrigin()}${url.pathname}${url.search}`, {
		method: request.method,
		headers: forwardedHeaders(request.headers),
		// The bytes as sent: a multipart import keeps its boundary.
		body: hasBody ? await request.arrayBuffer() : undefined,
		redirect: 'manual'
	});
	return new Response(response.body, {
		status: response.status,
		statusText: response.statusText,
		headers: relayedHeaders(response.headers)
	});
};

export const GET = relay;
export const POST = relay;
export const PUT = relay;
export const PATCH = relay;
export const DELETE = relay;
