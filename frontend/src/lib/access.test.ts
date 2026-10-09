import { describe, expect, it } from 'vitest';
import type { User } from './api/auth';
import { canOpen, navLinks, nextPath } from './access';

const user = (role: User['role']): User => ({
	id: '00000000-0000-4000-8000-000000000001',
	email: 'osoba@example.test',
	fullName: 'Anna Testowa',
	role,
	disabled: false,
	createdAt: '2026-10-01T08:00:00Z',
	lastLoginAt: null
});

describe('canOpen', () => {
	it.each([
		['/login', null, 'ok'],
		['/', null, 'login'],
		['/wiadomosci', null, 'login'],
		['/wiadomosci', 'sender', 'ok'],
		['/odbiorcy', 'sender', 'forbidden'],
		['/odbiorcy/import', 'sender', 'forbidden'],
		['/grupy/1', 'sender', 'forbidden'],
		['/admin', 'sender', 'forbidden'],
		['/admin/uzytkownicy', 'admin', 'ok'],
		['/administracja-cos', 'sender', 'ok']
	] as const)('%s for %s is %s', (path, role, want) => {
		expect(canOpen(role ? user(role) : null, path)).toBe(want);
	});
});

describe('nextPath', () => {
	it.each([
		[null, '/'],
		['/odbiorcy?q=jan', '/odbiorcy?q=jan'],
		['https://zla-strona.example', '/'],
		['//zla-strona.example', '/'],
		['/\\zla-strona.example', '/'],
		['/login', '/'],
		['odbiorcy', '/']
	])('%s -> %s', (raw, want) => {
		expect(nextPath(raw)).toBe(want);
	});
});

describe('navLinks', () => {
	it('shows a sender only the composer', () => {
		expect(navLinks(user('sender')).map((l) => l.href)).toEqual(['/wiadomosci']);
	});
	it('shows an admin the panel', () => {
		expect(navLinks(user('admin')).map((l) => l.href)).toContain('/admin');
	});
	it('is empty before logging in', () => {
		expect(navLinks(null)).toEqual([]);
	});
});
