// Polish texts and formatting for the administration panel. Every number
// comes from the backend; nothing here counts SMS parts or costs.
import type { AuditEntry } from '$lib/api/admin';
import type { User } from '$lib/api/auth';
import { roleLabels } from '$lib/access';
import { costText } from '$lib/format/text';

export { dateTimeText } from '$lib/format/text';

/** A cost in thousandths of a złoty (80 -> "0,08 zł"). */
export const plnText = costText;

const pointsFormat = new Intl.NumberFormat('pl-PL', {
	minimumFractionDigits: 2,
	maximumFractionDigits: 2,
	useGrouping: true
});

/** Formats a points balance (e.g. 125.5 -> "125,50 pkt"). */
export function pointsText(points: number): string {
	return `${pointsFormat.format(points)} pkt`;
}

/** Points balance below which the admin dashboard shows a low balance warning. */
export const LOW_POINTS_THRESHOLD = 50;

/** Email credit limit below which the admin dashboard shows a low credits warning. */
export const LOW_EMAIL_CREDITS_THRESHOLD = 20;

/** Formats email credits (e.g. 95 / 100 -> "95 / 100 kredytów" or 95 -> "95 kredytów"). */
export function creditsText(remain?: number, total?: number): string {
	if (remain === undefined) return '—';
	if (total !== undefined) {
		return `${remain} / ${total} kredytów`;
	}
	return `${remain} kredytów`;
}

const monthName = new Intl.DateTimeFormat('pl-PL', {
	month: 'long',
	year: 'numeric',
	timeZone: 'Europe/Warsaw'
});

/** The current month in Warsaw as an inclusive YYYY-MM-DD range and its name. */
export function currentMonth(now: Date): { from: string; to: string; label: string } {
	const { from, to, label } = monthOf(null, now);
	return { from, to, label };
}

export type Month = {
	/** YYYY-MM, as in ?miesiac=. */
	key: string;
	from: string;
	to: string;
	label: string;
	previous: string;
	/** null for the current month: there is nothing to show after it. */
	next: string | null;
	isCurrent: boolean;
};

function warsawYearMonth(now: Date): { year: number; month: number } {
	const parts = new Intl.DateTimeFormat('en-CA', {
		year: 'numeric',
		month: '2-digit',
		timeZone: 'Europe/Warsaw'
	}).formatToParts(now);
	return {
		year: Number(parts.find((p) => p.type === 'year')?.value),
		month: Number(parts.find((p) => p.type === 'month')?.value)
	};
}

const keyOf = (year: number, month: number) => `${year}-${String(month).padStart(2, '0')}`;

/**
 * The month to report: ?miesiac=YYYY-MM when it is a past or the current
 * month, otherwise the current month in Warsaw.
 */
export function monthOf(raw: string | null, now: Date): Month {
	const current = warsawYearMonth(now);
	let { year, month } = current;
	const m = raw?.match(/^(\d{4})-(\d{2})$/);
	if (m) {
		const y = Number(m[1]);
		const mo = Number(m[2]);
		const notFuture = y < current.year || (y === current.year && mo <= current.month);
		if (mo >= 1 && mo <= 12 && y >= 2000 && notFuture) {
			year = y;
			month = mo;
		}
	}
	const lastDay = new Date(Date.UTC(year, month, 0)).getUTCDate();
	const key = keyOf(year, month);
	const isCurrent = year === current.year && month === current.month;
	const prev = month === 1 ? keyOf(year - 1, 12) : keyOf(year, month - 1);
	const next = month === 12 ? keyOf(year + 1, 1) : keyOf(year, month + 1);
	return {
		key,
		from: `${key}-01`,
		to: `${key}-${String(lastDay).padStart(2, '0')}`,
		// Mid-month at noon UTC is the same month in Warsaw.
		label: monthName.format(new Date(Date.UTC(year, month - 1, 15, 12))),
		previous: prev,
		next: isCurrent ? null : next,
		isCurrent
	};
}

const actionTexts: Record<string, string> = {
	login: 'Zalogowanie',
	login_failed: 'Nieudane logowanie',
	logout: 'Wylogowanie',
	user_created: 'Utworzenie konta',
	user_updated: 'Zmiana konta',
	password_reset: 'Nadanie nowego hasła',
	password_changed: 'Zmiana własnego hasła',
	batch_sent: 'Zatwierdzenie wysyłki',
	batch_retried: 'Ponowienie wysyłki',
	recipient_contact_fixed: 'Uzupełnienie kontaktu odbiorcy',
	recipients_imported: 'Import odbiorców',
	settings_changed: 'Zmiana ustawień wysyłki',
	settings_test_sent: 'Wiadomość testowa',
	sender_verification_requested: 'Zgłoszenie nadawcy e-mail do weryfikacji',
	sender_verification_resent: 'Ponowne wysłanie linku weryfikacji nadawcy',
	sender_identity_removed: 'Usunięcie nadawcy e-mail',
	sender_domain_added: 'Dodanie domeny nadawcy',
	sender_domain_validated: 'Sprawdzenie DNS domeny nadawcy',
	sender_domain_removed: 'Usunięcie domeny nadawcy',
	sender_address_added: 'Dodanie adresu nadawcy',
	sender_address_updated: 'Zmiana adresu nadawcy',
	sender_address_removed: 'Usunięcie adresu nadawcy'
};

/**
 * A Polish name for an audit action. An action this version does not know yet
 * shows as "inne zdarzenie (code)" rather than a bare English code.
 */
export function actionText(action: string): string {
	return actionTexts[action] ?? `inne zdarzenie (${action})`;
}

/** Where an audit entry's subject can be seen, if anywhere. */
export function entityHref(entry: Pick<AuditEntry, 'entity' | 'entityId'>): string | null {
	if (!entry.entityId) return null;
	switch (entry.entity) {
		case 'batch':
			return `/historia/${encodeURIComponent(entry.entityId)}`;
		case 'recipient':
			return `/odbiorcy/${encodeURIComponent(entry.entityId)}`;
		case 'user':
			return '/admin/uzytkownicy';
		default:
			return null;
	}
}

const settingKeyNames: Record<string, string> = {
	email_provider: 'operator e-mail',
	sms_provider: 'operator SMS'
};

const providerNames: Record<string, string> = {
	fake: 'testowy',
	mailpit: 'Mailpit',
	sendgrid: 'SendGrid',
	smsapi: 'SMSAPI'
};

const sourceNames: Record<string, string> = {
	notion: 'Notion',
	csv: 'plik CSV',
	xlsx: 'plik XLSX'
};

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
			case 'recipients':
				out.push(`odbiorcy: ${value}`);
				break;
			case 'groups':
				out.push(`grupy: ${value}`);
				break;
			case 'smsParts':
				out.push(`części SMS: ${value}`);
				break;
			case 'channels':
				out.push(
					`kanały: ${value
						.split(',')
						.map((c) => (c === 'sms' ? 'SMS' : c === 'email' ? 'e-mail' : c))
						.join(', ')}`
				);
				break;
			case 'source':
				out.push(`źródło: ${sourceNames[value] ?? value}`);
				break;
			case 'pages':
				out.push(`stron: ${value}`);
				break;
			case 'imported':
				out.push(`zapisano: ${value}`);
				break;
			case 'invalid':
				out.push(`błędnych: ${value}`);
				break;
			case 'duplicates':
				out.push(`duplikatów: ${value}`);
				break;
			case 'provider':
				out.push(`operator: ${providerNames[value] ?? value}`);
				break;
			case 'keys':
				out.push(`zmienione ustawienia: ${value.split(',').filter(Boolean).length}`);
				break;
			case 'dry_run':
				out.push(value === 'true' ? 'tryb testowy włączony' : 'tryb testowy wyłączony');
				break;
			case 'email_provider':
			case 'sms_provider':
				out.push(`${settingKeyNames[key]}: ${providerNames[value] ?? value}`);
				break;
			case 'domain':
				out.push(`domena: ${value}`);
				break;
			case 'email':
				out.push(`adres: ${value}`);
				break;
			case 'channel':
				out.push(`kanał: ${value === 'sms' ? 'SMS' : value === 'email' ? 'e-mail' : value}`);
				break;
			default:
				// Unknown keys stay out rather than show raw English codes.
				break;
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
			? { ok: true, text: 'Tryb testowy (DRY_RUN) — żadna wiadomość nie wychodzi poza serwer.' }
			: {
					ok: false,
					text: 'Tryb testowy wyłączony — wiadomości trafiają do prawdziwych odbiorców.'
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
