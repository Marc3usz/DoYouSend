// Which groups a recipient is in, for the recipient detail page.
import type { Group } from '$lib/api/groups';
import type { Recipient } from '$lib/api/recipients';

export type MemberGroup = { id: string; name: string; kind: Group['kind'] };

/**
 * The person's custom groups (from groupIds) and class groups (from their type
 * and classes), sorted by name. null when the group list could not be loaded.
 * The "all parents" / "all students" groups are not listed: everyone of that
 * type is in them.
 */
export function memberGroups(
	r: Pick<Recipient, 'type' | 'classes' | 'groupIds'>,
	groups: Group[] | null
): MemberGroup[] | null {
	if (!groups) return null;
	const own = new Set(r.groupIds ?? []);
	const audience = r.type === 'student' ? 'students' : 'parents';
	return groups
		.filter((g) =>
			g.kind === 'custom'
				? own.has(g.id)
				: g.className !== null && g.audience === audience && r.classes.includes(g.className)
		)
		.map((g) => ({ id: g.id, name: g.name, kind: g.kind }))
		.sort((a, b) => a.name.localeCompare(b.name, 'pl'));
}
