<script lang="ts">
	import { page as pageState } from '$app/state';
	import type { PageProps } from './$types';
	import { listHref, pageCount } from './list';
	import { issueHint, issueText, recipientTypeLabel } from './messages';

	let { data }: PageProps = $props();

	const lastPage = $derived(data.page ? pageCount(data.page.total) : 1);
	const filtered = $derived(
		data.params.q !== '' || data.params.type !== '' || data.params.issue !== ''
	);
	const deleted = $derived(pageState.url.searchParams.has('usunieto'));
</script>

<svelte:head>
	<title>Odbiorcy — DoYouSend</title>
</svelte:head>

<h1>Odbiorcy</h1>

<p class="actions">
	<a href="/odbiorcy/nowy">Dodaj odbiorcę</a>
	<a href="/odbiorcy/import">Import z pliku</a>
</p>

{#if deleted}
	<p class="notice" role="status">Odbiorca został usunięty.</p>
{/if}

<!-- A plain GET form: the filters land in the URL, so a view can be linked. -->
<form method="GET" class="filters">
	<label>
		Szukaj
		<input
			type="search"
			name="q"
			value={data.params.q}
			maxlength="100"
			placeholder="imię, nazwisko, e-mail lub telefon"
		/>
	</label>
	<label>
		Typ
		<select name="type" value={data.params.type}>
			<option value="">wszyscy</option>
			<option value="parent">rodzice</option>
			<option value="student">uczniowie</option>
		</select>
	</label>
	<label>
		Problem z danymi
		<select name="issue" value={data.params.issue}>
			<option value="">dowolny</option>
			<option value="email">bez działającego e-maila</option>
			<option value="sms">bez działającego telefonu</option>
		</select>
	</label>
	<button type="submit">Filtruj</button>
	{#if filtered}
		<a href="/odbiorcy">Wyczyść</a>
	{/if}
</form>

{#if data.failure}
	<div class="error" role="alert"><p>{data.failure}</p></div>
{:else if data.page}
	<p class="summary">
		{#if filtered}Pasujących odbiorców{:else}Odbiorców w bazie{/if}:
		<strong>{data.page.total}</strong>
	</p>

	{#if data.page.items.length === 0}
		<p>
			{#if filtered}
				Nikt nie pasuje do tych filtrów.
			{:else if data.page.total === 0}
				Baza jest pusta. <a href="/odbiorcy/nowy">Dodaj pierwszego odbiorcę</a> albo
				<a href="/odbiorcy/import">sprawdź plik do importu</a>.
			{:else}
				Ta strona listy jest pusta. <a href={listHref({ ...data.params, page: 1 })}
					>Wróć do początku</a
				>.
			{/if}
		</p>
	{:else}
		<table>
			<thead>
				<tr><th>Nazwisko i imię</th><th>Typ</th><th>E-mail</th><th>Telefon</th><th>Uwagi</th></tr>
			</thead>
			<tbody>
				{#each data.page.items as r (r.id)}
					<tr>
						<td><a href="/odbiorcy/{r.id}">{r.lastName} {r.firstName}</a></td>
						<td>{recipientTypeLabel(r.type)}</td>
						<td>{r.email ?? '—'}</td>
						<td>{r.phone ?? '—'}</td>
						<td>
							{#each r.issues as issue (issue.channel)}
								<span class="issue" title={issueHint(issue)}>{issueText(issue)}</span>
							{/each}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>

		{#if lastPage > 1}
			<nav class="pager" aria-label="Strony listy">
				{#if data.params.page > 1}
					<a href={listHref({ ...data.params, page: data.params.page - 1 })}>← Poprzednia</a>
				{/if}
				<span>Strona {data.params.page} z {lastPage}</span>
				{#if data.params.page < lastPage}
					<a href={listHref({ ...data.params, page: data.params.page + 1 })}>Następna →</a>
				{/if}
			</nav>
		{/if}
	{/if}
{/if}

<style>
	.actions {
		display: flex;
		gap: 1rem;
	}
	.filters {
		display: flex;
		gap: 0.75rem;
		align-items: end;
		flex-wrap: wrap;
		margin: 1rem 0;
	}
	.filters label {
		display: flex;
		flex-direction: column;
		font-size: 0.9rem;
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
	table {
		border-collapse: collapse;
		width: 100%;
		margin-bottom: 1rem;
	}
	th,
	td {
		border-bottom: 1px solid #ddd;
		padding: 0.35rem 0.5rem;
		text-align: left;
		vertical-align: top;
	}
	.issue {
		display: inline-block;
		margin-right: 0.35rem;
		padding: 0 0.4rem;
		border-radius: 0.25rem;
		background: #fff1d6;
		color: #6b4500;
		font-size: 0.85rem;
	}
	.pager {
		display: flex;
		gap: 1rem;
		align-items: center;
	}
</style>
