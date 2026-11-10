// The sending-settings form (ADR-0018): Polish labels, turning the submitted
// form into a PATCH body, and backend errors into messages.
import { ApiError } from '$lib/api/client';
import { errorBody } from '$lib/api/errors';
import type { SettingItem, Settings } from '$lib/api/admin';

/** What the administrator types to confirm real sending. */
export const CONFIRM_PHRASE = 'WYSYŁAJ';

export const groupLabels: Record<SettingItem['group'], string> = {
	general: 'Tryb wysyłki',
	email: 'E-mail',
	sms: 'SMS',
	notion: 'Import odbiorców z Notion'
};

type Label = { label: string; hint?: string };

export const settingLabels: Record<string, Label> = {
	dry_run: {
		label: 'Tryb testowy (DRY_RUN)',
		hint: 'Włączony: nic nie wychodzi poza serwer, wysyłka jest tylko zapisywana.'
	},
	email_provider: {
		label: 'Operator e-mail',
		hint: 'mailpit = lokalna skrzynka testowa, sendgrid = prawdziwa wysyłka.'
	},
	email_from: {
		label: 'Nadawca e-mail',
		hint: 'Np. Szkoła Podstawowa nr 1 <sekretariat@szkola.pl>'
	},
	smtp_host: { label: 'Serwer SMTP (Mailpit)' },
	smtp_port: { label: 'Port SMTP' },
	smtp_username: { label: 'Login SMTP' },
	smtp_password: { label: 'Hasło SMTP' },
	sendgrid_api_key: {
		label: 'Klucz API SendGrid',
		hint: 'Uprawnienia: Mail Send, Sender Verification, odczyt Credits.'
	},
	sendgrid_sandbox: {
		label: 'Sandbox SendGrid',
		hint: 'SendGrid sprawdza wiadomość, ale jej nie doręcza.'
	},
	sendgrid_region: { label: 'Region danych SendGrid', hint: 'eu = serwery w UE (zalecane).' },
	sendgrid_webhook_public_key: {
		label: 'Klucz publiczny webhooka zdarzeń SendGrid',
		hint: 'Z ustawień Event Webhook (Signed Event Webhook). Pusty = statusy doręczeń wyłączone.'
	},
	sendgrid_sender_address: {
		label: 'Adres szkoły (ulica)',
		hint: 'Pokazywany w e-mailu weryfikacji nadawców.'
	},
	sendgrid_sender_city: { label: 'Miasto' },
	sendgrid_sender_zip: { label: 'Kod pocztowy' },
	sendgrid_sender_country: { label: 'Kraj' },
	sms_provider: {
		label: 'Operator SMS',
		hint: 'fake = SMS-y tylko w historii, smsapi = prawdziwa bramka.'
	},
	sms_api_key: { label: 'Token API SMSAPI' },
	sms_sender_name: {
		label: 'Nazwa nadawcy SMS',
		hint: 'Musi być zarejestrowana w SMSAPI (do 11 znaków).'
	},
	sms_test_mode: {
		label: 'Tryb testowy SMSAPI',
		hint: 'SMSAPI przyjmuje wiadomość, ale jej nie wysyła.'
	},
	sms_price_per_part_pln: {
		label: 'Cena części SMS (zł)',
		hint: 'Z kropką, np. 0.08. Dotyczy nowych wysyłek.'
	},
	smsapi_dlr_token: {
		label: 'Token raportów doręczeń SMSAPI',
		hint: 'Pusty = raporty doręczeń wyłączone.'
	},
	notion_token: {
		label: 'Token integracji Notion',
		hint: 'Internal Integration Secret (ntn_…) integracji z uprawnieniem tylko do odczytu.'
	},
	notion_database_id: {
		label: 'Baza Notion z odbiorcami',
		hint: 'Wklej link do bazy (⋯ → Copy link). Bazę trzeba udostępnić integracji (⋯ → Connections).'
	},
	notion_data_source_id: {
		label: 'Źródło danych Notion',
		hint: 'Tylko gdy baza ma kilka źródeł danych: ⋯ → Manage data sources → Copy ID.'
	},
	notion_property_map: {
		label: 'Mapowanie kolumn Notion',
		hint: 'Gdy kolumny nie nazywają się jak w szablonie (Imię, Nazwisko, E-mail, Telefon, Typ, Klasa), np. „Name=imię, Mail=e-mail”.'
	}
};

export const sourceLabels: Record<SettingItem['source'], string> = {
	panel: 'z panelu',
	env: 'z pliku .env',
	default: 'domyślne'
};

export function labelOf(key: string): Label {
	return settingLabels[key] ?? { label: key };
}

function text(data: FormData, name: string): string | null {
	const v = data.get(name);
	return typeof v === 'string' ? v : null;
}

/**
 * The PATCH values for a submitted form: only what the administrator changed.
 * An unchanged field is left out, so a value from .env is not copied into the
 * panel. A secret left empty stays as it is; "reset:<key>" drops a panel value.
 */
export function collectChanges(data: FormData, current: Settings): Record<string, string | null> {
	const out: Record<string, string | null> = {};
	for (const item of current.items) {
		if (text(data, `reset:${item.key}`) === 'on') {
			if (item.source === 'panel' || item.unreadable) out[item.key] = null;
			continue;
		}
		const v = text(data, item.key);
		if (v === null) continue;
		if (item.kind === 'secret') {
			if (v.trim() !== '') out[item.key] = v.trim();
			continue;
		}
		if (v.trim() !== (item.value ?? '')) out[item.key] = v.trim();
	}
	return out;
}

/** The non-secret values of a rejected form, to show them again. */
export function keptValues(data: FormData, current: Settings): Record<string, string> {
	const out: Record<string, string> = {};
	for (const item of current.items) {
		const v = text(data, item.key);
		if (item.kind !== 'secret' && v !== null) out[item.key] = v;
	}
	return out;
}

/** A Polish message for a rejected save, plus per-setting messages. */
export function settingsFailureOf(err: unknown): {
	status: number;
	failure: string;
	fields: Record<string, string>;
	needsConfirmation: boolean;
} {
	const body = errorBody(err);
	switch (body?.code) {
		case 'confirmation_required':
			return {
				status: 409,
				needsConfirmation: true,
				fields: {},
				failure: `Ta zmiana włącza prawdziwą wysyłkę e-maili lub SMS-ów. Wpisz ${CONFIRM_PHRASE} w polu potwierdzenia i zapisz ponownie.`
			};
		case 'dry_run_locked':
			return {
				status: 409,
				needsConfirmation: false,
				fields: {},
				failure:
					'Na tym serwerze tryb testowy jest zablokowany (DRY_RUN_LOCK). Wyłączyć go może tylko osoba z dostępem do serwera.'
			};
		case 'batches_running':
			return {
				status: 409,
				needsConfirmation: false,
				fields: {},
				failure:
					'Trwa wysyłka wiadomości. Tryb testowy można wyłączyć, a operatora zmienić dopiero po jej zakończeniu. Nic nie zostało zapisane, spróbuj ponownie za chwilę.'
			};
		case 'invalid_input': {
			const fields: Record<string, string> = {};
			let failure = 'Popraw zaznaczone pola.';
			for (const f of body.fields) {
				if (f.field === 'config') {
					failure = `Z tymi ustawieniami nie da się uruchomić operatora: ${f.message}`;
				} else {
					fields[f.field] = fieldText(f.field);
				}
			}
			return { status: 400, needsConfirmation: false, fields, failure };
		}
	}
	const status = err instanceof ApiError && err.status < 500 ? err.status : 500;
	return {
		status,
		needsConfirmation: false,
		fields: {},
		failure: 'Nie udało się zapisać ustawień. Spróbuj ponownie za chwilę.'
	};
}

function fieldText(key: string): string {
	switch (key) {
		case 'sms_price_per_part_pln':
			return 'Podaj cenę z kropką, np. 0.08 (najwyżej 3 miejsca po przecinku).';
		case 'smtp_port':
			return 'Podaj numer portu od 1 do 65535.';
		case 'email_from':
			return 'Podaj adres, np. Szkoła <sekretariat@szkola.pl>.';
		case 'notion_database_id':
		case 'notion_data_source_id':
			return 'Wklej link skopiowany z Notion albo identyfikator (32 znaki).';
		case 'notion_property_map':
			return 'Podaj pary „kolumna w Notion=kolumna importu” oddzielone przecinkami, np. Name=imię, Mail=e-mail.';
		default:
			return 'Niepoprawna wartość.';
	}
}
