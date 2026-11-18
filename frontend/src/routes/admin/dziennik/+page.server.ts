import { listAudit, listUsers } from '$lib/api/admin';
import { pageParam } from '$lib/format/text';
import type { PageServerLoad } from './$types';
import { auditFilterOf } from '../format';

const PAGE_SIZE = 50;

export const load: PageServerLoad = async ({ url, fetch }) => {
	const page = pageParam(url.searchParams.get('strona'));
	const filter = auditFilterOf(url.searchParams);
	// The user list only feeds the "Kto" filter; the log works without it.
	const users = listUsers(fetch).catch(() => null);
	try {
		const audit = await listAudit(
			{ limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE, ...filter },
			fetch
		);
		return { page, pageSize: PAGE_SIZE, audit, failed: false, filter, users: await users };
	} catch {
		return { page, pageSize: PAGE_SIZE, audit: null, failed: true, filter, users: await users };
	}
};
