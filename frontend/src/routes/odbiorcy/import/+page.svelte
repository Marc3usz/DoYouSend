<script lang="ts">
	import {
		checkImportFile,
		checkNotionImport,
		importFile,
		importFileError,
		importFromNotion,
		notionImportError,
		type ImportReport,
		type NotionImportReport
	} from '$lib/api/recipients';
	import ReportTables from './ReportTables.svelte';
	import {
		fileErrorText,
		notionErrorInSettings,
		notionErrorText,
		saveErrorText,
		savedSummary,
		type FileErrorText
	} from './messages';

	let file = $state<File | null>(null);
	let checking = $state(false);
	let report = $state<ImportReport | null>(null);
	let reportFileName = $state('');
	let failure = $state<FileErrorText | null>(null);
	let saving = $state(false);
	// True once report is the answer of a save, not of a check.
	let saved = $state(false);

	// The Notion import (ADR-0017) shares the report with the file import:
	// notionReport is set when the report on screen came from Notion.
	let notionReport = $state<NotionImportReport | null>(null);
	let notionBusy = $state<'check' | 'save' | null>(null);
	let failureInSettings = $state(false);
	// Bumped by every action, so a late answer to an older one is dropped.
	let seq = 0;

	const busy = $derived(checking || saving || notionBusy !== null);
	const pageUrls = $derived(new Map(notionReport?.pages.map((p) => [p.row, p.url]) ?? []));

	function clearResult() {
		seq++;
		report = null;
		notionReport = null;
		failure = null;
		failureInSettings = false;
		saved = false;
	}

	// A result on screen always belongs to the currently selected file, so
	// choosing another file (or cancelling the picker) clears it.
	function selectFile(next: File | null) {
		file = next;
		clearResult();
	}

	// The check is triggered by the user, so it runs in the browser (through
	// the /api dev proxy) rather than in a load function.
	async function check(event: SubmitEvent) {
		event.preventDefault();
		const checked = file;
		clearResult();
		const mine = seq;
		if (!checked) {
			failure = fileErrorText({ code: 'missing_file', message: '' });
			return;
		}
		checking = true;
		try {
			const result = await checkImportFile(checked);
			// Drop a late answer for a file that is no longer selected.
			if (file === checked && seq === mine) {
				report = result;
				reportFileName = checked.name;
			}
		} catch (err) {
			if (file === checked && seq === mine) {
				failure = fileErrorText(importFileError(err));
			}
		} finally {
			checking = false;
		}
	}

	// Saving sends the same file again: the backend checks it once more
	// against the stored recipients and stores the valid rows, all or none.
	async function save() {
		const toSave = file;
		if (!toSave || !report) return;
		failure = null;
		const mine = ++seq;
		saving = true;
		try {
			const result = await importFile(toSave);
			if (file === toSave && seq === mine) {
				report = result;
				saved = true;
			}
		} catch (err) {
			if (file === toSave && seq === mine) {
				failure = saveErrorText(importFileError(err));
			}
		} finally {
			saving = false;
		}
	}

	// The Notion database is set in the settings panel; the backend reads it
	// again on every call, so saving may differ from the check if someone
	// edited Notion in between (the report shows what was stored).
	async function runNotion(kind: 'check' | 'save') {
		if (kind === 'check') {
			clearResult();
		} else {
			failure = null;
			failureInSettings = false;
			seq++;
		}
		const mine = seq;
		notionBusy = kind;
		try {
			const result = kind === 'check' ? await checkNotionImport() : await importFromNotion();
			if (seq === mine) {
				report = result;
				notionReport = result;
				saved = kind === 'save';
			}
		} catch (err) {
			if (seq === mine) {
				const body = notionImportError(err);
				failure = notionErrorText(body);
				failureInSettings = notionErrorInSettings(body);
			}
		} finally {
			notionBusy = null;
		}
	}
</script>

<svelte:head>
	<title>Import odbiorców — DoYouSend</title>
</svelte:head>

<p><a href="/odbiorcy">← Odbiorcy</a></p>
<h1>Import odbiorców</h1>

<h2>Z pliku</h2>

<p>
	Wgraj plik CSV (UTF-8, separator przecinek albo średnik) lub XLSX. Pierwszy wiersz to nagłówek z
	kolumnami: <strong>imię, nazwisko, e-mail, telefon, typ</strong> (rodzic albo uczeń) i opcjonalnie
	<strong>klasa</strong> (uczeń: jego klasa, rodzic: klasy dzieci po przecinku, np. „1B, 3A”). Każda osoba
	musi mieć e-mail lub telefon.
</p>
<p class="note">
	Najpierw sprawdź plik — sprawdzenie niczego nie zapisuje. Potem zapisz poprawne wiersze: błędne i
	duplikaty (także osoby, które już są w bazie) zostaną pominięte.
</p>

<form onsubmit={check}>
	<input
		type="file"
		name="file"
		accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		onchange={(e) => selectFile(e.currentTarget.files?.[0] ?? null)}
	/>
	<button type="submit" disabled={busy}>{checking ? 'Sprawdzam…' : 'Sprawdź plik'}</button>
</form>

<h2>Z bazy Notion</h2>
<p class="note">
	Wczytuje bazę Notion ustawioną w <a href="/admin/ustawienia">Ustawieniach</a> (token integracji i link
	do bazy). Kolumny jak w pliku; inne nazwy można zmapować w Ustawieniach. Wiersze przechodzą te same
	kontrole co plik, a przy każdym jest link do strony w Notion.
</p>
<p>
	<button type="button" onclick={() => runNotion('check')} disabled={busy}
		>{notionBusy === 'check' ? 'Wczytuję z Notion…' : 'Sprawdź bazę Notion'}</button
	>
</p>

{#if failure}
	<div class="error" role="alert">
		<p>{failure.text}</p>
		{#if failure.detail}
			<p class="detail">Szczegóły: <code>{failure.detail}</code></p>
		{/if}
		{#if failureInSettings}
			<p><a href="/admin/ustawienia">Przejdź do Ustawień</a></p>
		{/if}
	</div>
{/if}

{#if report && saved}
	<div class="notice" role="status">
		<p>{savedSummary(report)}</p>
		<p><a href="/odbiorcy">Przejdź do listy odbiorców</a></p>
	</div>
{:else if report}
	<p class="summary" aria-live="polite">
		{#if notionReport}
			Baza Notion ({notionReport.pageCount} stron)
		{:else}
			Plik <strong>{reportFileName}</strong>
		{/if}
		— poprawne: <strong>{report.valid.length}</strong>, błędne:
		<strong>{report.invalid.length}</strong>, duplikaty:
		<strong>{report.duplicates.length}</strong>
	</p>
	{#if report.valid.length > 0}
		<p>
			<button
				type="button"
				class="primary"
				onclick={() => (notionReport ? runNotion('save') : save())}
				disabled={busy}
			>
				{saving || notionBusy === 'save'
					? 'Zapisuję…'
					: `Zapisz poprawne wiersze (${report.valid.length})`}
			</button>
		</p>
	{/if}
{/if}

{#if report}
	<ReportTables
		{report}
		{saved}
		pageUrl={notionReport ? (row) => pageUrls.get(row) : undefined}
		emptyText={notionReport
			? 'Baza Notion ma poprawne kolumny, ale nie zawiera żadnych osób.'
			: 'Plik ma poprawny nagłówek, ale nie zawiera żadnych osób.'}
	/>
{/if}

<style>
	.note {
		color: var(--color-text-muted);
	}
	form {
		display: flex;
		gap: 0.75rem;
		align-items: center;
		flex-wrap: wrap;
		margin: 1rem 0;
	}
	.detail {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
</style>
