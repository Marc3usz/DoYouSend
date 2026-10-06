<script lang="ts">
	import {
		checkImportFile,
		importFile,
		importFileError,
		type ImportReport
	} from '$lib/api/recipients';
	import {
		duplicateText,
		fieldErrorText,
		fileErrorText,
		recipientTypeLabel,
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

	// A result on screen always belongs to the currently selected file, so
	// choosing another file (or cancelling the picker) clears it.
	function selectFile(next: File | null) {
		file = next;
		report = null;
		failure = null;
		saved = false;
	}

	// The check is triggered by the user, so it runs in the browser (through
	// the /api dev proxy) rather than in a load function.
	async function check(event: SubmitEvent) {
		event.preventDefault();
		const checked = file;
		report = null;
		failure = null;
		saved = false;
		if (!checked) {
			failure = fileErrorText({ code: 'missing_file', message: '' });
			return;
		}
		checking = true;
		try {
			const result = await checkImportFile(checked);
			// Drop a late answer for a file that is no longer selected.
			if (file === checked) {
				report = result;
				reportFileName = checked.name;
			}
		} catch (err) {
			if (file === checked) {
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
		saving = true;
		try {
			const result = await importFile(toSave);
			if (file === toSave) {
				report = result;
				saved = true;
			}
		} catch (err) {
			if (file === toSave) {
				failure = saveErrorText(importFileError(err));
			}
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Import odbiorców — DoYouSend</title>
</svelte:head>

<p><a href="/odbiorcy">← Odbiorcy</a></p>
<h1>Import odbiorców z pliku</h1>

<p>
	Wgraj plik CSV (UTF-8, separator przecinek albo średnik) lub XLSX. Pierwszy wiersz to nagłówek z
	kolumnami: <strong>imię, nazwisko, e-mail, telefon, typ</strong> (rodzic albo uczeń). Każda osoba musi
	mieć e-mail lub telefon.
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
	<button type="submit" disabled={checking || saving}
		>{checking ? 'Sprawdzam…' : 'Sprawdź plik'}</button
	>
</form>

{#if failure}
	<div class="error" role="alert">
		<p>{failure.text}</p>
		{#if failure.detail}
			<p class="detail">Szczegóły: <code>{failure.detail}</code></p>
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
		Plik <strong>{reportFileName}</strong> — poprawne: <strong>{report.valid.length}</strong>,
		błędne: <strong>{report.invalid.length}</strong>, duplikaty:
		<strong>{report.duplicates.length}</strong>
	</p>
	{#if report.valid.length > 0}
		<p>
			<button type="button" onclick={save} disabled={saving}>
				{saving ? 'Zapisuję…' : `Zapisz poprawne wiersze (${report.valid.length})`}
			</button>
		</p>
	{/if}
{/if}

{#if report}
	{#if report.invalid.length > 0}
		<h2>Błędne wiersze</h2>
		<table>
			<thead><tr><th>Wiersz</th><th>Problem</th></tr></thead>
			<tbody>
				{#each report.invalid as row (row.row)}
					<tr>
						<td>{row.row}</td>
						<td>
							<ul>
								{#each row.errors as error (error.field)}
									<li>{fieldErrorText(error)}</li>
								{/each}
							</ul>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}

	{#if report.duplicates.length > 0}
		<h2>Duplikaty</h2>
		<table>
			<thead><tr><th>Wiersz</th><th>Problem</th></tr></thead>
			<tbody>
				{#each report.duplicates as row (row.row)}
					<tr>
						<td>{row.row}</td>
						<td>
							{duplicateText(row)}
							{#if row.existingRecipientId}
								— <a href="/odbiorcy/{row.existingRecipientId}">zobacz</a>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}

	{#if report.valid.length > 0}
		<h2>{saved ? 'Zapisane wiersze' : 'Poprawne wiersze'}</h2>
		<table>
			<thead>
				<tr
					><th>Wiersz</th><th>Imię</th><th>Nazwisko</th><th>E-mail</th><th>Telefon</th><th>Typ</th
					></tr
				>
			</thead>
			<tbody>
				{#each report.valid as { row, recipient } (row)}
					<tr>
						<td>{row}</td>
						<td>{recipient.firstName}</td>
						<td>{recipient.lastName}</td>
						<td>{recipient.email ?? '—'}</td>
						<td>{recipient.phone ?? '—'}</td>
						<td>{recipientTypeLabel(recipient.type)}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}

	{#if report.valid.length + report.invalid.length + report.duplicates.length === 0}
		<p>Plik ma poprawny nagłówek, ale nie zawiera żadnych osób.</p>
	{/if}
{/if}

<style>
	.note {
		color: #555;
	}
	form {
		display: flex;
		gap: 0.75rem;
		align-items: center;
		flex-wrap: wrap;
		margin: 1rem 0;
	}
	.notice {
		border: 1px solid #2a7;
		background: #f2fbf6;
		padding: 0.5rem 1rem;
	}
	.error {
		border: 1px solid #c00;
		background: #fff4f4;
		padding: 0.5rem 1rem;
	}
	.detail {
		font-size: 0.85rem;
		color: #555;
	}
	table {
		border-collapse: collapse;
		width: 100%;
		margin-bottom: 1.5rem;
	}
	th,
	td {
		border-bottom: 1px solid #ddd;
		padding: 0.35rem 0.5rem;
		text-align: left;
		vertical-align: top;
	}
	td ul {
		margin: 0;
		padding-left: 1.1rem;
	}
</style>
