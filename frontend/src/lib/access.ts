// Which pages a user may open. It mirrors the backend route policy
// (backend/internal/iam/policy.go) for pages; the backend still checks every
// API call on its own, so this is about not rendering useless pages.
import type { User } from './api/auth';

export type PageAccess = 'ok' | 'login' | 'forbidden';

/** Pages anyone may open, logged in or not. */
const publicPages = ['/login'];

/** Pages for administrators only: the recipient base, groups and the panel. */
const adminPages = ['/admin', '/odbiorcy', '/grupy'];

function under(path: string, prefix: string): boolean {
	return path === prefix || path.startsWith(`${prefix}/`);
}

export function canOpen(user: User | null, path: string): PageAccess {
	if (publicPages.some((p) => under(path, p))) return 'ok';
	if (!user) return 'login';
	if (adminPages.some((p) => under(path, p)) && user.role !== 'admin') return 'forbidden';
	return 'ok';
}

/** A safe place to go after logging in: a local path, never another site. */
export function nextPath(raw: string | null): string {
	if (!raw || !raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return '/';
	return under(raw, '/login') ? '/' : raw;
}

export type NavLink = { href: string; label: string };

/** The main navigation for a user's role. */
export function navLinks(user: User | null): NavLink[] {
	if (!user) return [];
	const links: NavLink[] = [{ href: '/wiadomosci', label: 'Nowa wiadomość' }];
	if (user.role === 'admin') {
		links.push(
			{ href: '/odbiorcy', label: 'Odbiorcy' },
			{ href: '/grupy', label: 'Grupy' },
			{ href: '/admin', label: 'Panel administratora' }
		);
	}
	return links;
}

export const roleLabels: Record<User['role'], string> = {
	admin: 'administrator',
	sender: 'wysyłający'
};
