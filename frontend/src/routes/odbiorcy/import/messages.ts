// Polish texts for the import report. The backend sends stable keys
// (field, code) plus an English diagnostic message; the UI never shows
// the English message on its own.
import type {
	ImportDuplicate,
	ImportFieldError,
	ImportFileError,
	ImportReport,
	NotionImportError,
	RecipientType
} from '$lib/api/recipients';

const fieldErrorTexts: Record<ImportFieldError['field'], string> = {
	first_name: 'Brak imienia',
	last_name: 'Brak nazwiska',
	type: 'Nieznany typ — wpisz „rodzic” albo „uczeń”',
	email: 'Niepoprawny adres e-mail',
	phone: 'Niepoprawny numer telefonu, oczekiwany np. +48 500 100 101',
	classes: 'Niepoprawna klasa — wpisz np. „3A” albo „1B, 3A”; uczeń może mieć tylko jedną klasę',
	contact: 'Brak e-maila i telefonu — potrzebny jest co najmniej jeden',
	encoding: 'Wiersz nie jest zapisany w UTF-8 — zapisz plik jako „CSV UTF-8”',
	row: 'Nie udało się odczytać wiersza'
};

export function fieldErrorText(error: ImportFieldError): string {
	return fieldErrorTexts[error.field];
}

const fileErrorTexts: Record<ImportFileError['code'], string> = {
	invalid_request: 'Nie udało się wysłać pliku. Spróbuj ponownie.',
	missing_file: 'Wybierz plik do sprawdzenia.',
	empty_file: 'Plik jest pusty — brakuje wiersza nagłówka.',
	missing_columns:
		'W nagłówku brakuje wymaganych kolumn: imię, nazwisko, e-mail, telefon i typ (kolumny e-mail i telefon mogą mieć puste komórki).',
	invalid_header: 'Nie udało się odczytać wiersza nagłówka.',
	unsupported_format:
		'Nieobsługiwany format. Wgraj plik CSV (UTF-8) albo XLSX bez hasła; stary format .xls zapisz jako .xlsx.',
	file_too_large: 'Plik jest za duży — limit to 5 MB.',
	too_many_rows:
		'Plik ma za dużo wierszy — limit to 5000 osób. Usuń z arkusza wszystko poza listą.',
	unclosed_quote:
		'W pliku jest niezamknięty cudzysłów, przez który nie da się odczytać dalszych wierszy.'
};

export type FileErrorText = { text: string; detail?: string };

/** Text for a whole-file error; undefined means the check itself failed. */
export function fileErrorText(error: ImportFileError | undefined): FileErrorText {
	if (!error) {
		return { text: 'Nie udało się sprawdzić pliku. Spróbuj ponownie za chwilę.' };
	}
	return { text: fileErrorTexts[error.code], detail: error.message };
}

/** Text for a failed save; undefined means the save itself failed (nothing was stored). */
export function saveErrorText(error: ImportFileError | undefined): FileErrorText {
	if (!error) {
		return {
			text: 'Nie udało się zapisać odbiorców. Nic nie zostało zapisane — spróbuj ponownie za chwilę.'
		};
	}
	return fileErrorText(error);
}

/** The summary after a save; valid rows are the ones actually stored. */
export function savedSummary(report: ImportReport): string {
	const skipped = report.invalid.length + report.duplicates.length;
	const saved = `Zapisano odbiorców: ${report.valid.length}.`;
	return skipped > 0
		? `${saved} Pominięto wierszy: ${skipped} (błędne i duplikaty, lista niżej).`
		: saved;
}

export function duplicateText(duplicate: ImportDuplicate): string {
	const what = duplicate.field === 'email' ? 'Ten sam e-mail' : 'Ten sam numer telefonu';
	if (duplicate.duplicateOfRow !== undefined) {
		return `${what} co w wierszu ${duplicate.duplicateOfRow}`;
	}
	return `${what} ma już odbiorca zapisany w bazie`;
}

export { recipientTypeLabel } from '$lib/format/recipients';

const notionErrorTexts: Record<NotionImportError['code'], string> = {
	notion_not_configured:
		'Import z Notion nie jest skonfigurowany. Administrator ustawia token integracji i link do bazy w Ustawieniach.',
	notion_invalid_config:
		'Ustawienia Notion są niepoprawne — sprawdź link do bazy i mapowanie kolumn w Ustawieniach.',
	notion_ambiguous_source:
		'Baza Notion ma kilka źródeł danych. Wskaż jedno w Ustawieniach (pole „Źródło danych Notion”).',
	notion_no_source:
		'Integracja nie widzi żadnego źródła danych w tej bazie. Sprawdź, czy baza jest udostępniona integracji.',
	notion_unauthorized: 'Notion odrzucił token integracji. Wpisz aktualny token w Ustawieniach.',
	notion_not_found:
		'Notion nie znalazł bazy. Udostępnij ją integracji (⋯ → Connections) albo popraw link w Ustawieniach.',
	notion_unavailable: 'Notion nie odpowiada albo ogranicza ruch. Spróbuj ponownie za chwilę.',
	empty_file: 'Baza Notion nie ma żadnego wiersza.',
	missing_columns:
		'W bazie Notion brakuje wymaganych kolumn: imię, nazwisko, e-mail, telefon i typ. Zmień nazwy kolumn albo ustaw mapowanie w Ustawieniach.',
	too_many_rows: 'Baza Notion ma za dużo wierszy — limit to 5000 osób.',
	internal:
		'Nie udało się zapisać odbiorców. Nic nie zostało zapisane — spróbuj ponownie za chwilę.'
};

/** Text for a failed Notion check or import; undefined means the request itself failed. */
export function notionErrorText(error: NotionImportError | undefined): FileErrorText {
	if (!error) {
		return { text: 'Nie udało się połączyć z serwerem. Spróbuj ponownie za chwilę.' };
	}
	return { text: notionErrorTexts[error.code], detail: error.message };
}

/** Whether the administrator can fix the error in the settings panel. */
export function notionErrorInSettings(error: NotionImportError | undefined): boolean {
	return (
		error !== undefined &&
		[
			'notion_not_configured',
			'notion_invalid_config',
			'notion_ambiguous_source',
			'notion_unauthorized',
			'notion_not_found',
			'missing_columns'
		].includes(error.code)
	);
}
