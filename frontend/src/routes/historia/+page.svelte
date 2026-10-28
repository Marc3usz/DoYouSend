<script lang="ts">
	import type { PageProps } from './$types';
	import { costText, recipientsText } from '../wiadomosci/estimate';
	import { batchStatusLabels, dateTimeText, filterStatuses } from './format';

	let { data }: PageProps = $props();

	const pages = $derived(
		data.batches ? Math.max(1, Math.ceil(data.batches.total / data.pageSize)) : 1
	);

	function pageHref(page: number): string {
		const q = new URLSearchParams();
		if (data.status) q.set('status', data.status);
		if (page > 1) q.set('strona', String(page));
		return q.size > 0 ? `?${q}` : '/historia';
	}
</script>

<svelte:head>
	<title>Historia wysyłek — DoYouSend</title>
</svelte:head>

<h1>Historia wysyłek</h1>
<p class="lead">
	Od najnowszej. Wysyłający widzi swoje wysyłki, administrator wszystkie. Szczegóły pokazują status
	e-maila i SMS-a dla każdego odbiorcy.
</p>

<nav class="filters" aria-label="Filtr statusu">
	<a href="/historia" aria-current={data.status ? undefined : 'page'}>Wszystkie</a>
	{#each filterStatuses as status (status)}
		<a href="?status={status}" aria-current={data.status === status ? 'page' : undefined}>
			{batchStatusLabels[status]}
		</a>
	{/each}
</nav>

{#if data.failed}
	<p class="error" role="alert">Nie udało się wczytać historii. Odśwież stronę.</p>
{:else if data.batches && data.batches.items.length === 0}
	<p>
		{data.status ? 'Brak wysyłek z tym statusem.' : 'Nie było jeszcze żadnej wysyłki.'}
		<a href="/wiadomosci">Nowa wiadomość</a>
	</p>
{:else if data.batches}
	<table>
		<thead>
			<tr>
				<th>Kiedy</th>
				<th>Temat</th>
				<th>Kto</th>
				<th>Grupy</th>
				<th>Odbiorcy</th>
				<th>Status</th>
				<th class="num">Koszt SMS</th>
			</tr>
		</thead>
		<tbody>
			{#each data.batches.items as batch (batch.id)}
				<tr>
					<td>{dateTimeText(batch.createdAt)}</td>
					<td><a href="/historia/{batch.id}">{batch.subject}</a></td>
					<td>{batch.createdBy.fullName}</td>
					<td>{batch.groups.map((g) => g.name).join(', ') || '—'}</td>
					<td>
						{recipientsText(batch.recipientCount)}
						{#if batch.failedCount > 0}
							<br /><span class="bad">{batch.failedCount} z błędem</span>
						{/if}
						{#if batch.partialCount > 0}
							<br /><span class="warn">{batch.partialCount} jednym kanałem</span>
						{/if}
					</td>
					<td><span class="status {batch.status}">{batchStatusLabels[batch.status]}</span></td>
					<td class="num">{costText(batch.costMilli)}</td>
				</tr>
			{/each}
		</tbody>
	</table>

	{#if pages > 1}
		<nav class="pager" aria-label="Strony historii">
			{#if data.page > 1}<a href={pageHref(data.page - 1)}>← Nowsze</a>{/if}
			<span>Strona {data.page} z {pages}</span>
			{#if data.page < pages}<a href={pageHref(data.page + 1)}>Starsze →</a>{/if}
		</nav>
	{/if}
{/if}

<style>
	.lead {
		color: var(--color-text-muted);
		max-width: 42rem;
	}
	.filters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 1rem;
		margin: 1rem 0;
	}
	.filters a[aria-current='page'] {
		font-weight: 700;
		text-decoration: none;
	}
	.num {
		text-align: right;
		font-variant-numeric: tabular-nums;
	}
	.status {
		white-space: nowrap;
	}
	.status.done_with_errors,
	.bad {
		color: var(--color-danger);
	}
	.status.running,
	.warn {
		color: var(--color-warning);
	}
	.pager {
		display: flex;
		gap: 1rem;
		margin-top: 1rem;
	}
</style>
