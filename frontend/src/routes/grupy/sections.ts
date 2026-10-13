// Splits the group list (already in the backend's order: fixed built-in
// groups, class groups by class, custom groups by name) into the three
// sections of the groups page.
import type { Group } from '$lib/api/groups';

export type ClassRow = { className: string; students?: Group; parents?: Group };

export type GroupSections = { builtIn: Group[]; classes: ClassRow[]; custom: Group[] };

export function groupSections(groups: Group[]): GroupSections {
	const out: GroupSections = { builtIn: [], classes: [], custom: [] };
	const rows = new Map<string, ClassRow>();
	for (const g of groups) {
		if (g.kind === 'custom') {
			out.custom.push(g);
			continue;
		}
		if (g.className === null) {
			out.builtIn.push(g);
			continue;
		}
		let row = rows.get(g.className);
		if (!row) {
			row = { className: g.className };
			rows.set(g.className, row);
			out.classes.push(row);
		}
		if (g.audience === 'students') row.students = g;
		else row.parents = g;
	}
	return out;
}
