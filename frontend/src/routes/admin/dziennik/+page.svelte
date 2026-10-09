<script lang="ts">
	import type { PageProps } from './$types';
	import { actionText, actorText, dateTimeText, detailsText } from '../format';

	let { data }: PageProps = $props();

	const pages = $derived(data.audit ? Math.max(1, Math.ceil(data.audit.total / data.pageSize)) : 1);
</script>

<svelte:head>
	<title>Dziennik zdarzeń — DoYouSend</title>
</svelte:head>

<p><a href="/admin">← Panel administratora</a></p>
<h1>Dziennik zdarzeń</h1>
<p class="lead">
	Kto się logował, kto zakładał i zmieniał konta, a w kolejnych wersjach także kto przygotował i
	zatwierdził każdą wysyłkę. Wpisów nie da się zmienić ani usunąć z panelu.
</p>

{#if data.failed}
	<p class="error" role="alert">Nie udało się wczytać dziennika. Odśwież stronę.</p>
{:else if data.audit && data.audit.items.length === 0}
	<p>Brak zdarzeń.</p>
{:else if data.audit}
	<table>
		<thead><tr><th>Kiedy</th><th>Kto</th><th>Zdarzenie</th><th>Szczegóły</th></tr></thead>
		<tbody>
			{#each data.audit.items as entry (entry.id)}
				<tr>
					<td>{dateTimeText(entry.createdAt)}</td>
					<td>{actorText(entry)}</td>
					<td>{actionText(entry.action)}</td>
					<td>{detailsText(entry)}</td>
				</tr>
			{/each}
		</tbody>
	</table>

	{#if pages > 1}
		<nav class="pager" aria-label="Strony dziennika">
			{#if data.page > 1}<a href="?strona={data.page - 1}">← Nowsze</a>{/if}
			<span>Strona {data.page} z {pages}</span>
			{#if data.page < pages}<a href="?strona={data.page + 1}">Starsze →</a>{/if}
		</nav>
	{/if}
{/if}

<style>
	.lead {
		color: #555;
		max-width: 42rem;
	}
	table {
		border-collapse: collapse;
		width: 100%;
	}
	th,
	td {
		border-bottom: 1px solid #ddd;
		padding: 0.35rem 0.5rem;
		text-align: left;
		vertical-align: top;
	}
	.pager {
		display: flex;
		gap: 1rem;
		margin-top: 1rem;
	}
	.error {
		color: #b00020;
	}
</style>
