<script lang="ts">
	import type { ImportReport } from '$lib/api/recipients';
	import { duplicateText, fieldErrorText, recipientTypeLabel } from './messages';

	let {
		report,
		saved,
		pageUrl,
		emptyText
	}: {
		report: ImportReport;
		/** True when report is the answer of a save, not of a check. */
		saved: boolean;
		/** The source page of a row (Notion import), shown as a link next to its number. */
		pageUrl?: (row: number) => string | undefined;
		emptyText: string;
	} = $props();
</script>

{#snippet rowCell(row: number)}
	<td>
		{row}
		{#if pageUrl?.(row)}
			<a class="source" href={pageUrl(row)} target="_blank" rel="noopener noreferrer"
				>otwórz w Notion</a
			>
		{/if}
	</td>
{/snippet}

{#if report.invalid.length > 0}
	<h2>Błędne wiersze</h2>
	<table>
		<thead><tr><th>Wiersz</th><th>Problem</th></tr></thead>
		<tbody>
			{#each report.invalid as row (row.row)}
				<tr>
					{@render rowCell(row.row)}
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
					{@render rowCell(row.row)}
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
				><th>Wiersz</th><th>Imię</th><th>Nazwisko</th><th>E-mail</th><th>Telefon</th><th>Typ</th><th
					>Klasa</th
				></tr
			>
		</thead>
		<tbody>
			{#each report.valid as { row, recipient } (row)}
				<tr>
					{@render rowCell(row)}
					<td>{recipient.firstName}</td>
					<td>{recipient.lastName}</td>
					<td>{recipient.email ?? '—'}</td>
					<td>{recipient.phone ?? '—'}</td>
					<td>{recipientTypeLabel(recipient.type)}</td>
					<td>{recipient.classes.length > 0 ? recipient.classes.join(', ') : '—'}</td>
				</tr>
			{/each}
		</tbody>
	</table>
{/if}

{#if report.valid.length + report.invalid.length + report.duplicates.length === 0}
	<p>{emptyText}</p>
{/if}

<style>
	td ul {
		margin: 0;
		padding-left: 1.1rem;
	}
	.source {
		margin-left: 0.4rem;
		font-size: 0.85rem;
	}
</style>
