import { fail } from '@sveltejs/kit';
import { listUsers } from '$lib/api/admin';
import {
	addSenderAddress,
	addSenderDomain,
	listSenderAddresses,
	listSenderDomains,
	removeSenderAddress,
	removeSenderDomain,
	updateSenderAddress,
	validateSenderDomain
} from '$lib/api/senders';
import type { Actions, PageServerLoad } from './$types';
import { addressFailureOf, domainFailureOf, domainStatusLabels, readUserIds } from './senders';

export const load: PageServerLoad = async ({ fetch }) => {
	// Each section loads on its own, so one failing endpoint leaves the rest usable.
	const [domains, addresses, users] = await Promise.all([
		listSenderDomains(fetch).catch(() => null),
		listSenderAddresses(fetch).catch(() => null),
		listUsers(fetch).catch(() => null)
	]);
	return {
		domains,
		addresses: addresses?.items ?? null,
		// Only active accounts can be given an address.
		users: users?.filter((u) => !u.disabled) ?? null
	};
};

const text = (data: FormData, name: string) => {
	const v = data.get(name);
	return typeof v === 'string' ? v.trim() : '';
};

export const actions: Actions = {
	domainAdd: async ({ request, fetch }) => {
		const domain = text(await request.formData(), 'domain');
		try {
			const added = await addSenderDomain(domain, fetch);
			return {
				action: 'domainAdd' as const,
				done: `Dodano domenę ${added.domain}. Dodaj u dostawcy DNS rekordy z tabeli i naciśnij „Sprawdź DNS”.`
			};
		} catch (err) {
			const f = domainFailureOf(err, 'dodać domeny');
			return fail(f.status, { action: 'domainAdd' as const, domain, ...f });
		}
	},

	domainValidate: async ({ request, fetch }) => {
		const id = text(await request.formData(), 'id');
		try {
			const d = await validateSenderDomain(id, fetch);
			return {
				action: 'domain' as const,
				id,
				done:
					d.status === 'valid'
						? `Domena ${d.domain} jest uwierzytelniona. Jej użytkownicy wysyłają teraz ze swoich adresów.`
						: `Domena ${d.domain}: ${domainStatusLabels[d.status]}. Rekordy oznaczone ✗ nie są jeszcze widoczne w DNS — zmiany potrafią potrwać do kilku godzin.`
			};
		} catch (err) {
			const f = domainFailureOf(err, 'sprawdzić DNS');
			return fail(f.status, { action: 'domain' as const, id, failure: f.failure });
		}
	},

	domainRemove: async ({ request, fetch }) => {
		const id = text(await request.formData(), 'id');
		try {
			await removeSenderDomain(id, fetch);
			return { action: 'domainRemoved' as const, done: 'Usunięto domenę.' };
		} catch (err) {
			const f = domainFailureOf(err, 'usunąć domeny');
			return fail(f.status, { action: 'domain' as const, id, failure: f.failure });
		}
	},

	addressAdd: async ({ request, fetch }) => {
		const data = await request.formData();
		const values = { email: text(data, 'email'), name: text(data, 'name') };
		const userIds = readUserIds(data);
		try {
			const added = await addSenderAddress({ ...values, userIds }, fetch);
			return { action: 'addressAdd' as const, done: `Dodano adres ${added.email}.` };
		} catch (err) {
			const f = addressFailureOf(err, 'dodać adresu');
			return fail(f.status, { action: 'addressAdd' as const, values, userIds, ...f });
		}
	},

	addressUpdate: async ({ request, fetch }) => {
		const data = await request.formData();
		const id = text(data, 'id');
		const name = text(data, 'name');
		try {
			const saved = await updateSenderAddress(id, { name, userIds: readUserIds(data) }, fetch);
			return { action: 'address' as const, id, done: `Zapisano zmiany adresu ${saved.email}.` };
		} catch (err) {
			const f = addressFailureOf(err);
			return fail(f.status, {
				action: 'address' as const,
				id,
				failure: f.failure || Object.values(f.fields).join(' ')
			});
		}
	},

	addressRemove: async ({ request, fetch }) => {
		const id = text(await request.formData(), 'id');
		try {
			await removeSenderAddress(id, fetch);
			return { action: 'addressRemoved' as const, done: 'Usunięto adres.' };
		} catch (err) {
			const f = addressFailureOf(err, 'usunąć adresu');
			return fail(f.status, { action: 'address' as const, id, failure: f.failure });
		}
	}
};
