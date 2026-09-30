// Jedyne miejsce, przez które frontend rozmawia z backendem.
// Kontrakt endpointów: docs/api/openapi.yaml — zmiana kontraktu to osobny PR.

export class ApiError extends Error {
	constructor(
		readonly status: number,
		message: string,
		// Parsed JSON error body, when the backend sent one.
		readonly body?: unknown
	) {
		super(message);
		this.name = 'ApiError';
	}
}

type RequestOptions = {
	method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
	// FormData is sent as multipart/form-data, anything else as JSON.
	body?: unknown;
	fetch?: typeof globalThis.fetch;
};

export async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
	const doFetch = options.fetch ?? globalThis.fetch;
	const { body } = options;
	const isForm = body instanceof FormData;
	const response = await doFetch(`/api${path}`, {
		method: options.method ?? 'GET',
		// For FormData the browser sets Content-Type itself, with the boundary.
		headers: body && !isForm ? { 'Content-Type': 'application/json' } : undefined,
		body: isForm ? body : body ? JSON.stringify(body) : undefined
	});

	if (!response.ok) {
		const errorBody: unknown = await response.json().catch(() => undefined);
		throw new ApiError(
			response.status,
			`${options.method ?? 'GET'} ${path} -> ${response.status}`,
			errorBody
		);
	}
	return response.status === 204 ? (undefined as T) : ((await response.json()) as T);
}
