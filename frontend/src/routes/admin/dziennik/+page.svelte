<script lang="ts">
	import type { PageProps } from './$types';
	import {
		actionText,
		actorText,
		auditActionOptions,
		dateTimeText,
		detailsText,
		entityHref
	} from '../format';

	let { data }: PageProps = $props();

	const filtered = $derived(Object.values(data.filter).some((v) => v !== ''));
	// Keeps the filter when moving between pages.
	function pageHref(p: number): string {
		const q = new URLSearchParams();
		if (data.filter.action) q.set('zdarzenie', data.filter.action);
		if (data.filter.userId) q.set('kto', data.filter.userId);
		if (data.filter.from) q.set('od', data.filter.from);
		if (data.filter.to) q.set('do', data.filter.to);
		if (p > 1) q.set('strona', String(p));
		return q.size > 0 ? `?${q}` : '/admin/dziennik';
	}

	const pages = $derived(data.audit ? Math.max(1, Math.ceil(data.audit.total / data.pageSize)) : 1);
</script>

<svelte:head>
	<title>Dziennik zdarzeń — DoYouSend</title>
</svelte:head>

<p><a href="/admin">← Panel administratora</a></p>
<h1>Dziennik zdarzeń</h1>
<p class="lead">
	Kto się logował, kto zakładał i zmieniał konta oraz kto zatwierdził i uruchomił każdą wysyłkę.
	Wpisów nie da się zmienić ani usunąć z panelu.
</p>

<form method="GET" class="filters" aria-label="Filtry dziennika">
	<label>
		Zdarzenie
		<select name="zdarzenie" value={data.filter.action}>
			<option value="">wszystkie</option>
			{#each auditActionOptions as o (o.value)}
				<option value={o.value}>{o.label}</option>
			{/each}
		</select>
	</label>
	{#if data.users}
		<label>
			Kto
			<select name="kto" value={data.filter.userId}>
				<option value="">wszyscy</option>
				{#each data.users as u (u.id)}
					<option value={u.id}>{u.fullName}</option>
				{/each}
			</select>
		</label>
	{/if}
	<label>
		Od
		<input type="date" name="od" value={data.filter.from} />
	</label>
	<label>
		Do
		<input type="date" name="do" value={data.filter.to} />
	</label>
	<button type="submit">Filtruj</button>
	{#if filtered}<a href="/admin/dziennik">Wyczyść</a>{/if}
</form>

{#if data.failed}
	<p class="error" role="alert">
		Nie udało się wczytać dziennika. {filtered
			? 'Sprawdź, czy data „Do” nie jest wcześniejsza niż „Od”.'
			: 'Odśwież stronę.'}
	</p>
{:else if data.audit && data.audit.items.length === 0}
	<p>{filtered ? 'Żadne zdarzenie nie pasuje do filtrów.' : 'Brak zdarzeń.'}</p>
{:else if data.audit}
	<table>
		<thead>
			<tr>
				<th scope="col">Kiedy</th>
				<th scope="col">Kto</th>
				<th scope="col">Zdarzenie</th>
				<th scope="col">Szczegóły</th>
			</tr>
		</thead>
		<tbody>
			{#each data.audit.items as entry (entry.id)}
				<tr>
					<td>{dateTimeText(entry.createdAt)}</td>
					<td>{actorText(entry)}</td>
					<td>
						{#if entityHref(entry)}
							<a href={entityHref(entry)}>{actionText(entry.action)}</a>
						{:else}
							{actionText(entry.action)}
						{/if}
					</td>
					<td>{detailsText(entry)}</td>
				</tr>
			{/each}
		</tbody>
	</table>

	{#if pages > 1}
		<nav class="pager" aria-label="Strony dziennika">
			{#if data.page > 1}<a href={pageHref(data.page - 1)}>← Nowsze</a>{/if}
			<span>Strona {data.page} z {pages}</span>
			{#if data.page < pages}<a href={pageHref(data.page + 1)}>Starsze →</a>{/if}
		</nav>
	{/if}
{/if}

<style>
	.filters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		align-items: end;
		margin: 1rem 0;
	}
	.filters label {
		display: flex;
		flex-direction: column;
		font-size: 0.9rem;
	}
	.pager {
		display: flex;
		gap: 1rem;
		margin-top: 1rem;
	}
</style>
