import { emailUsage, listAudit, listUsers, sendingConfig, smsUsage } from '$lib/api/admin';
import type { PageServerLoad } from './$types';
import { currentMonth } from './format';

// Each panel loads on its own, so one failing endpoint leaves the rest usable.
async function settle<T>(p: Promise<T>): Promise<T | null> {
	try {
		return await p;
	} catch {
		return null;
	}
}

export const load: PageServerLoad = async ({ fetch }) => {
	const month = currentMonth(new Date());
	const [config, usage, emailStats, users, audit] = await Promise.all([
		settle(sendingConfig(fetch)),
		settle(smsUsage({ from: month.from, to: month.to }, fetch)),
		settle(emailUsage({ from: month.from, to: month.to }, fetch)),
		settle(listUsers(fetch)),
		settle(listAudit({ limit: 8, offset: 0 }, fetch))
	]);
	return { month, config, usage, emailStats, users, audit };
};
