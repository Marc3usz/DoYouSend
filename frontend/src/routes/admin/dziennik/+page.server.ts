import { listAudit } from '$lib/api/admin';
import type { PageServerLoad } from './$types';

const PAGE_SIZE = 50;

export const load: PageServerLoad = async ({ url, fetch }) => {
	const raw = Number(url.searchParams.get('strona') ?? '1');
	const page = Number.isInteger(raw) && raw > 0 ? raw : 1;
	try {
		const audit = await listAudit({ limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE }, fetch);
		return { page, pageSize: PAGE_SIZE, audit, failed: false };
	} catch {
		return { page, pageSize: PAGE_SIZE, audit: null, failed: true };
	}
};
