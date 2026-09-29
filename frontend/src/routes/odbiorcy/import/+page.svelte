<script lang="ts">
	import { checkImportFile, importFileError, type ImportReport } from '$lib/api/recipients';
	import {
		duplicateText,
		fieldErrorText,
		fileErrorText,
		recipientTypeLabel,
		type FileErrorText
	} from './messages';

	let file = $state<File | null>(null);
	let checking = $state(false);
	let report = $state<ImportReport | null>(null);
	let failure = $state<FileErrorText | null>(null);

	// The check is triggered by the user, so it runs in the browser (through
	// the /api dev proxy) rather than in a load function.
	async function check(event: SubmitEvent) {
		event.preventDefault();
		if (!file) {
			failure = fileErrorText({ code: 'missing_file', message: '' });
			return;
		}
		checking = true;
		report = null;
		failure = null;
		try {
			report = await checkImportFile(file);
		} catch (err) {
			failure = fileErrorText(importFileError(err));
		} finally {
			checking = false;
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
	To jest sprawdzenie pliku — nic nie zostaje zapisane. Zapisywanie odbiorców z pliku pojawi się w
	kolejnej wersji.
</p>

<form onsubmit={check}>
	<input
		type="file"
		name="file"
		accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		onchange={(e) => (file = e.currentTarget.files?.[0] ?? null)}
	/>
	<button type="submit" disabled={checking}>{checking ? 'Sprawdzam…' : 'Sprawdź plik'}</button>
</form>

{#if failure}
	<div class="error" role="alert">
		<p>{failure.text}</p>
		{#if failure.detail}
			<p class="detail">Szczegóły: <code>{failure.detail}</code></p>
		{/if}
	</div>
{/if}

{#if report}
	<p class="summary" aria-live="polite">
		Poprawne: <strong>{report.valid.length}</strong> · Błędne:
		<strong>{report.invalid.length}</strong> · Duplikaty:
		<strong>{report.duplicates.length}</strong>
	</p>

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
					<tr><td>{row.row}</td><td>{duplicateText(row)}</td></tr>
				{/each}
			</tbody>
		</table>
	{/if}

	{#if report.valid.length > 0}
		<h2>Poprawne wiersze</h2>
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
