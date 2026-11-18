import {
	emailProfile,
	emailUsage,
	listAudit,
	listUsers,
	sendingConfig,
	smsProfile,
	smsUsage
} from '$lib/api/admin';
import type { PageServerLoad } from './$types';
import { monthOf } from './format';

// Each panel loads on its own, so one failing endpoint leaves the rest usable.
async function settle<T>(p: Promise<T>): Promise<T | null> {
	try {
		return await p;
	} catch {
		return null;
	}
}

export const load: PageServerLoad = async ({ fetch, url }) => {
	const month = monthOf(url.searchParams.get('miesiac'), new Date());
	const range = { from: month.from, to: month.to };
	const [config, usage, emailStats, users, audit, profile, emailProfileData] = await Promise.all([
		settle(sendingConfig(fetch)),
		settle(smsUsage(range, fetch)),
		settle(emailUsage(range, fetch)),
		settle(listUsers(fetch)),
		settle(listAudit({ limit: 8, offset: 0 }, fetch)),
		settle(smsProfile(fetch)),
		settle(emailProfile(fetch))
	]);
	return {
		month,
		config,
		usage,
		emailStats,
		users,
		audit,
		profile,
		emailProfile: emailProfileData
	};
};
