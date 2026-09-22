// Jedyne miejsce, przez które frontend rozmawia z backendem.
// Kontrakt endpointów: docs/api/openapi.yaml — zmiana kontraktu to osobny PR.

export class ApiError extends Error {
	constructor(
		readonly status: number,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

type RequestOptions = {
	method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
	body?: unknown;
	fetch?: typeof globalThis.fetch;
};

export async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
	const doFetch = options.fetch ?? globalThis.fetch;
	const response = await doFetch(`/api${path}`, {
		method: options.method ?? 'GET',
		headers: options.body ? { 'Content-Type': 'application/json' } : undefined,
		body: options.body ? JSON.stringify(options.body) : undefined
	});

	if (!response.ok) {
		throw new ApiError(response.status, `${options.method ?? 'GET'} ${path} -> ${response.status}`);
	}
	return response.status === 204 ? (undefined as T) : ((await response.json()) as T);
}
