// Polish texts and formatting for the administration panel. Every number
// comes from the backend; nothing here counts SMS parts or costs.
import type { AuditEntry } from '$lib/api/admin';
import type { User } from '$lib/api/auth';
import { roleLabels } from '$lib/access';

const pln = new Intl.NumberFormat('pl-PL', {
	style: 'currency',
	currency: 'PLN',
	minimumFractionDigits: 2,
	maximumFractionDigits: 3,
	useGrouping: true
});

/** A cost in thousandths of a złoty (80 -> "0,08 zł"). */
export function plnText(milli: number): string {
	return pln.format(milli / 1000);
}

const dateTime = new Intl.DateTimeFormat('pl-PL', {
	dateStyle: 'medium',
	timeStyle: 'short',
	timeZone: 'Europe/Warsaw'
});

export function dateTimeText(iso: string | null): string {
	return iso ? dateTime.format(new Date(iso)) : '—';
}

const monthName = new Intl.DateTimeFormat('pl-PL', {
	month: 'long',
	year: 'numeric',
	timeZone: 'Europe/Warsaw'
});

/** The current month in Warsaw as an inclusive YYYY-MM-DD range and its name. */
export function currentMonth(now: Date): { from: string; to: string; label: string } {
	const parts = new Intl.DateTimeFormat('en-CA', {
		year: 'numeric',
		month: '2-digit',
		timeZone: 'Europe/Warsaw'
	}).formatToParts(now);
	const year = Number(parts.find((p) => p.type === 'year')?.value);
	const month = Number(parts.find((p) => p.type === 'month')?.value);
	const lastDay = new Date(Date.UTC(year, month, 0)).getUTCDate();
	const mm = String(month).padStart(2, '0');
	return {
		from: `${year}-${mm}-01`,
		to: `${year}-${mm}-${String(lastDay).padStart(2, '0')}`,
		label: monthName.format(now)
	};
}

const actionTexts: Record<string, string> = {
	login: 'Zalogowanie',
	login_failed: 'Nieudane logowanie',
	logout: 'Wylogowanie',
	user_created: 'Utworzenie konta',
	user_updated: 'Zmiana konta',
	password_reset: 'Nadanie nowego hasła',
	password_changed: 'Zmiana własnego hasła'
};

/** A Polish name for an audit action; actions of other domains show as they are. */
export function actionText(action: string): string {
	return actionTexts[action] ?? action;
}

/** The details of an audit entry as one line, e.g. "rola: administrator". */
export function detailsText(entry: AuditEntry): string {
	const out: string[] = [];
	for (const [key, value] of Object.entries(entry.details)) {
		switch (key) {
			case 'role':
				out.push(`rola: ${value in roleLabels ? roleLabels[value as User['role']] : value}`);
				break;
			case 'disabled':
				out.push(value === 'true' ? 'zablokowane' : 'odblokowane');
				break;
			case 'fullName':
				out.push('zmiana imienia i nazwiska');
				break;
			case 'via':
				out.push(`przez ${value}`);
				break;
			default:
				out.push(`${key}: ${value}`);
		}
	}
	return out.join(', ');
}

/** Who did it: the user's name, or a note for entries without one. */
export function actorText(entry: AuditEntry): string {
	// A failed login names the account someone tried, not who tried it.
	if (entry.action === 'login_failed') {
		return entry.userName ? `${entry.userName} (próba logowania)` : 'nieznany adres e-mail';
	}
	if (entry.userName) return entry.userName;
	return entry.userId ? 'usunięty użytkownik' : 'system';
}

export type Check = { ok: boolean; text: string };

/** Plain-language status lines for the sending setup. */
export function setupChecks(c: {
	dryRun: boolean;
	email: { provider: string; credentialsSet: boolean; eventsWebhook: boolean };
	sms: { provider: string; credentialsSet: boolean; reportsWebhook: boolean };
}): Check[] {
	const localEmail = c.email.provider === 'mailpit';
	const fakeSms = c.sms.provider === 'fake';
	return [
		c.dryRun
			? { ok: true, text: 'Tryb próbny (DRY_RUN) — żadna wiadomość nie wychodzi poza serwer.' }
			: {
					ok: false,
					text: 'Tryb próbny wyłączony — wiadomości trafiają do prawdziwych odbiorców.'
				},
		localEmail
			? { ok: true, text: 'E-mail: lokalny Mailpit (testowy).' }
			: c.email.credentialsSet
				? { ok: true, text: `E-mail: ${c.email.provider}, klucz ustawiony.` }
				: { ok: false, text: `E-mail: ${c.email.provider}, ale brak klucza dostępu.` },
		fakeSms
			? { ok: true, text: 'SMS: bramka testowa (fake), nic nie jest wysyłane.' }
			: c.sms.credentialsSet
				? { ok: true, text: `SMS: ${c.sms.provider}, klucz ustawiony.` }
				: { ok: false, text: `SMS: ${c.sms.provider}, ale brak klucza dostępu.` },
		fakeSms || c.sms.reportsWebhook
			? {
					ok: true,
					text:
						'Raporty doręczeń SMS: ' + (fakeSms ? 'niepotrzebne w trybie testowym.' : 'włączone.')
				}
			: {
					ok: false,
					text: 'Raporty doręczeń SMS wyłączone (brak SMSAPI_DLR_TOKEN) — statusy zatrzymają się na „wysłano”.'
				},
		localEmail || c.email.eventsWebhook
			? {
					ok: true,
					text:
						'Zdarzenia e-mail: ' + (localEmail ? 'niepotrzebne w trybie testowym.' : 'włączone.')
				}
			: {
					ok: false,
					text: 'Zdarzenia e-mail wyłączone (brak SENDGRID_WEBHOOK_PUBLIC_KEY) — nie zobaczysz doręczeń ani odbić.'
				}
	];
}
