// Endpoints of the iam domain: logging in and out, see docs/api/openapi.yaml.
import { api } from './client';

export type Role = 'admin' | 'sender';

export type User = {
	id: string;
	email: string;
	fullName: string;
	role: Role;
	disabled: boolean;
	createdAt: string;
	lastLoginAt: string | null;
};

/** Name of the session cookie the backend sets (ADR-0010). */
export const SESSION_COOKIE = 'dys_session';

/** The logged-in user: GET /auth/me. Fails with ApiError 401 without a session. */
export function me(fetch?: typeof globalThis.fetch): Promise<User> {
	return api<User>('/auth/me', { fetch });
}

/** POST /auth/logout: ends the session of the cookie sent with the request. */
export function logout(fetch?: typeof globalThis.fetch): Promise<void> {
	return api<void>('/auth/logout', { method: 'POST', fetch });
}

export type LoginResult = { user: User; token: string; expires: Date | null };

/**
 * POST /auth/login. The backend answers with the session in a Set-Cookie
 * header; the login form action reads it from there and sets the same cookie
 * on its own response, so it also works where /api is not proxied.
 */
export async function login(
	email: string,
	password: string,
	fetch: typeof globalThis.fetch
): Promise<LoginResult> {
	// Local to this call, so concurrent logins on the server never mix.
	const captured: { session: ReturnType<typeof sessionFromSetCookie> } = { session: null };
	const user = await api<User>('/auth/login', {
		method: 'POST',
		body: { email, password },
		fetch: async (input, init) => {
			const response = await fetch(input, init);
			captured.session = sessionFromSetCookie(response.headers.get('set-cookie'));
			return response;
		}
	});
	if (!captured.session) throw new Error('login response has no session cookie');
	return { user, ...captured.session };
}

/** Picks the session token and expiry out of a Set-Cookie header value. */
export function sessionFromSetCookie(
	header: string | null
): { token: string; expires: Date | null } | null {
	if (!header) return null;
	// Several cookies may be folded into one header, separated by ", " —
	// but Expires dates contain a comma too, so split on the cookie name.
	const start = header.indexOf(`${SESSION_COOKIE}=`);
	if (start < 0) return null;
	const parts = header
		.slice(start)
		.split(';')
		.map((p) => p.trim());
	const token = (parts[0] ?? '').slice(SESSION_COOKIE.length + 1);
	if (!token) return null;
	let expires: Date | null = null;
	for (const part of parts.slice(1)) {
		const [rawName = '', ...rest] = part.split('=');
		const name = rawName.toLowerCase();
		const value = rest.join('=');
		// Max-Age wins over Expires (RFC 6265, 5.3).
		if (name === 'max-age' && /^\d+$/.test(value)) {
			expires = new Date(Date.now() + Number(value) * 1000);
			break;
		}
		if (name === 'expires') {
			const date = new Date(value);
			if (!Number.isNaN(date.getTime())) expires = date;
		}
	}
	return { token, expires };
}
