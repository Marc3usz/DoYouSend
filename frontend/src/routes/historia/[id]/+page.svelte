<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { retryBatch } from '$lib/api/batches';
	import { errorBody } from '$lib/api/errors';
	import type { PageProps } from './$types';
	import { costText, recipientsText, smsText } from '../../wiadomosci/estimate';
	import {
		batchStatusLabels,
		dateTimeText,
		deliveryOf,
		deliveryText,
		stillSending
	} from '../format';

	// How often a running batch is reloaded to show new statuses.
	const REFRESH_MS = 3000;

	let { data }: PageProps = $props();
	const batch = $derived(data.batch);

	let retrying = $state(false);
	let retryError = $state<string | null>(null);

	async function onRetry() {
		retrying = true;
		retryError = null;
		try {
			await retryBatch(batch.id);
			await invalidateAll();
		} catch (err) {
			const body = errorBody(err);
			if (body?.code === 'no_failed_deliveries') {
				retryError = 'Brak nieudanych dostaw do ponowienia.';
			} else if (body?.code === 'batch_running') {
				retryError = 'Wysyłka jest obecnie w trakcie.';
			} else {
				retryError = 'Nie udało się uruchomić ponowienia. Spróbuj ponownie.';
			}
		} finally {
			retrying = false;
		}
	}

	$effect(() => {
		if (!stillSending(batch.status)) return;
		const timer = setInterval(() => invalidateAll(), REFRESH_MS);
		return () => clearInterval(timer);
	});
</script>

<svelte:head>
	<title>{batch.subject} — Historia — DoYouSend</title>
</svelte:head>

<p><a href="/historia">← Historia wysyłek</a></p>
<h1>{batch.subject}</h1>

<dl class="summary">
	<dt>Status</dt>
	<dd>
		<span class="status {batch.status}">{batchStatusLabels[batch.status]}</span>
		{#if stillSending(batch.status)}<span class="hint">(odświeża się samo)</span>{/if}
	</dd>
	<dt>Zatwierdzona przez</dt>
	<dd>{batch.createdBy.fullName}, {dateTimeText(batch.createdAt)}</dd>
	<dt>Zakończona</dt>
	<dd>{dateTimeText(batch.finishedAt)}</dd>
	<dt>Grupy</dt>
	<dd>{batch.groups.map((g) => g.name).join(', ') || '—'}</dd>
	{#if batch.recipientIds.length > 0}
		<dt>Wybrani ręcznie</dt>
		<dd>{recipientsText(batch.recipientIds.length)}</dd>
	{/if}
	<dt>Odbiorcy</dt>
	<dd>
		{recipientsText(batch.recipientCount)}
		{#if batch.failedCount > 0}
			· <span class="bad">{batch.failedCount} z błędem</span>
		{/if}
		{#if batch.partialCount > 0}
			· <span class="warn">{batch.partialCount} jednym kanałem</span>
		{/if}
	</dd>
	<dt>SMS</dt>
	<dd>{smsText(batch.totalSmsParts)}, {costText(batch.costMilli)}</dd>
</dl>

<div class="batch-actions">
	<a href="/wiadomosci?kopia={batch.id}" class="btn-copy">Użyj jako podstawy nowej wiadomości</a>
	{#if batch.failedCount > 0 && !stillSending(batch.status)}
		<button type="button" class="btn-retry" onclick={onRetry} disabled={retrying}>
			{retrying ? 'Uruchamianie ponowienia…' : `Ponów nieudane wiadomości (${batch.failedCount})`}
		</button>
	{/if}
</div>
{#if retryError}
	<p class="error" role="alert">{retryError}</p>
{/if}

<h2>Treść</h2>
<pre class="body">{batch.body}</pre>

<h2>Odbiorcy</h2>
{#if batch.recipients.length === 0}
	<p>Brak odbiorców.</p>
{:else}
	<table>
		<thead>
			<tr><th>Odbiorca</th><th>E-mail</th><th>SMS</th></tr>
		</thead>
		<tbody>
			{#each batch.recipients as r (r.recipientId)}
				{@const email = deliveryOf(r, 'email')}
				{@const sms = deliveryOf(r, 'sms')}
				<tr>
					<td>
						{r.lastName}
						{r.firstName}
						{#if r.unreachable}
							<br /><span class="bad">brak e-maila i telefonu</span>
						{:else if r.renderedBody === null}
							<br /><span class="bad">brak danych do pola w treści</span>
						{:else if r.partial}
							<br /><span class="warn">tylko jeden kanał</span>
						{/if}
					</td>
					<td class="delivery {email?.status ?? 'none'}">{deliveryText(email)}</td>
					<td class="delivery {sms?.status ?? 'none'}">{deliveryText(sms)}</td>
				</tr>
			{/each}
		</tbody>
	</table>
{/if}

<style>
	.summary {
		display: grid;
		grid-template-columns: max-content auto;
		gap: 0.3rem 1rem;
	}
	dd {
		margin: 0;
	}
	.body {
		white-space: pre-wrap;
		font: inherit;
		background: #fafafa;
		border: 1px solid #ddd;
		border-radius: 4px;
		padding: 0.75rem;
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
	.status.done_with_errors,
	.bad,
	.delivery.failed {
		color: #b00020;
	}
	.status.running,
	.warn,
	.delivery.pending,
	.delivery.sending {
		color: #8a4b00;
	}
	.delivery.delivered {
		color: #1a7f37;
	}
	.delivery.none {
		color: #777;
	}
	.hint {
		color: #555;
		font-size: 0.9em;
	}
	.batch-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
		margin: 1.25rem 0;
	}
	.btn-copy {
		display: inline-block;
		text-decoration: none;
		color: #1d4ed8;
		border: 1px solid #1d4ed8;
		border-radius: 4px;
		padding: 0.4rem 0.75rem;
		font: inherit;
		background: #fff;
	}
	.btn-copy:hover {
		background: #eff6ff;
	}
	.btn-retry {
		background: #b00020;
		color: #fff;
		border: 1px solid #b00020;
		border-radius: 4px;
		padding: 0.4rem 0.75rem;
		font: inherit;
		cursor: pointer;
	}
	.btn-retry:hover:not(:disabled) {
		background: #8e0019;
	}
	.btn-retry:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.error {
		color: #b00020;
		margin: 0.5rem 0;
	}
</style>
