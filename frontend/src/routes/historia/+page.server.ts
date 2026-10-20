import { listBatches } from '$lib/api/batches';
import type { PageServerLoad } from './$types';
import { isBatchStatus, pageParam } from './format';

const PAGE_SIZE = 50;

export const load: PageServerLoad = async ({ url, fetch }) => {
	const page = pageParam(url.searchParams.get('strona'));
	const raw = url.searchParams.get('status');
	const status = isBatchStatus(raw) ? raw : undefined;
	try {
		const batches = await listBatches(
			{ status, limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE },
			fetch
		);
		return { page, pageSize: PAGE_SIZE, status, batches, failed: false };
	} catch {
		return { page, pageSize: PAGE_SIZE, status, batches: null, failed: true };
	}
};
