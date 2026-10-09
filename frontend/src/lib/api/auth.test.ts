import { describe, expect, it } from 'vitest';
import { login, sessionFromSetCookie } from './auth';

describe('sessionFromSetCookie', () => {
	it('reads the token and Max-Age', () => {
		const before = Date.now();
		const got = sessionFromSetCookie(
			'dys_session=abc-123_X; Path=/; Expires=Fri, 09 Oct 2026 22:00:00 GMT; Max-Age=43200; HttpOnly; SameSite=Lax'
		);
		expect(got?.token).toBe('abc-123_X');
		expect(got?.expires?.getTime()).toBeGreaterThanOrEqual(before + 43200 * 1000);
	});

	it('falls back to Expires', () => {
		const got = sessionFromSetCookie(
			'dys_session=t; Path=/; Expires=Fri, 09 Oct 2026 22:00:00 GMT'
		);
		expect(got?.expires?.toISOString()).toBe('2026-10-09T22:00:00.000Z');
	});

	it('finds the session among folded cookies', () => {
		expect(sessionFromSetCookie('other=1; Path=/, dys_session=t2; HttpOnly')?.token).toBe('t2');
	});

	it.each([null, '', 'other=1', 'dys_session=; Max-Age=-1'])('has no session in %j', (header) => {
		expect(sessionFromSetCookie(header)).toBeNull();
	});
});

describe('login', () => {
	it('returns the user with the session from Set-Cookie', async () => {
		const user = { id: 'u1', email: 'admin@example.test', fullName: 'Piotr', role: 'admin' };
		const fakeFetch: typeof fetch = async (input) => {
			expect(String(input)).toBe('/api/auth/login');
			return new Response(JSON.stringify(user), {
				status: 200,
				headers: { 'set-cookie': 'dys_session=tok; Path=/; Max-Age=60; HttpOnly' }
			});
		};
		const got = await login('admin@example.test', 'haslo-admina-123', fakeFetch);
		expect(got.user).toEqual(user);
		expect(got.token).toBe('tok');
	});

	it('fails when the backend sets no session', async () => {
		const fakeFetch: typeof fetch = async () => new Response('{}', { status: 200 });
		await expect(login('a@example.test', 'x', fakeFetch)).rejects.toThrow(/no session/);
	});
});
