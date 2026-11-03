<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { retryBatch } from '$lib/api/batches';
	import { errorBody } from '$lib/api/errors';
	import type { PageProps } from './$types';
	import { costText, plural, recipientsText, smsText } from '../../wiadomosci/estimate';
	import {
		batchStatusLabels,
		dateTimeText,
		deliveryOf,
		deliveryText,
		excludedText,
		stillSending
	} from '../format';

	// How often a running batch is reloaded to show new statuses.
	const REFRESH_MS = 3000;

	let { data }: PageProps = $props();
	const batch = $derived(data.batch);
	const retriedCount = $derived(
		batch.recipients.reduce((sum, r) => sum + r.deliveries.filter((d) => d.attempts > 1).length, 0)
	);

	let retrying = $state(false);
	let retryError = $state<string | null>(null);
	let expandedBodies = $state<Record<string, boolean>>({});

	function toggleBody(id: string) {
		expandedBodies[id] = !expandedBodies[id];
	}

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
	{#if (batch.excludedRecipientIds?.length ?? 0) > 0}
		<dt>Wykluczeni z wysyłki</dt>
		<dd><span class="warn">{excludedText(batch.excludedRecipientIds!.length)}</span></dd>
	{/if}
	<dt>SMS</dt>
	<dd>{smsText(batch.totalSmsParts)}, {costText(batch.costMilli)}</dd>
	{#if retriedCount > 0}
		<dt>Ponowienia</dt>
		<dd>
			{retriedCount}
			{plural(
				retriedCount,
				'wiadomość ponawiana',
				'wiadomości ponawiane',
				'wiadomości ponawianych'
			)}
		</dd>
	{/if}
</dl>

<div class="batch-actions">
	<a href="/wiadomosci?kopia={batch.id}" class="button">Użyj jako podstawy nowej wiadomości</a>
	{#if batch.failedCount > 0 && !stillSending(batch.status)}
		<button type="button" class="primary" onclick={onRetry} disabled={retrying}>
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
						{#if r.renderedBody && r.renderedBody !== batch.body}
							<br />
							<button type="button" class="btn-preview" onclick={() => toggleBody(r.recipientId)}>
								{expandedBodies[r.recipientId] ? 'Ukryj treść' : 'Podgląd spersonalizowanej treści'}
							</button>
						{/if}
					</td>
					<td class="delivery {email?.status ?? 'none'}">{deliveryText(email)}</td>
					<td class="delivery {sms?.status ?? 'none'}">{deliveryText(sms)}</td>
				</tr>
				{#if expandedBodies[r.recipientId] && r.renderedBody}
					<tr class="preview-row">
						<td colspan="3">
							<div class="preview-card">
								<strong>Spersonalizowana treść ({r.firstName} {r.lastName}):</strong>
								<pre class="preview-content">{r.renderedBody}</pre>
							</div>
						</td>
					</tr>
				{/if}
			{/each}
		</tbody>
	</table>
{/if}

{#if data.excludedRecipients && data.excludedRecipients.length > 0}
	<section class="excluded-section" aria-labelledby="excluded-heading">
		<h2 id="excluded-heading">Wykluczeni z tej wysyłki ({data.excludedRecipients.length})</h2>
		<p class="hint">
			Osoby należące do wybranych grup, które zostały celowo wykluczone z tej wysyłki przed jej
			zatwierdzeniem:
		</p>
		<ul class="excluded-list">
			{#each data.excludedRecipients as person (person.id)}
				<li>{person.name}</li>
			{/each}
		</ul>
	</section>
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
		background: var(--color-surface-muted);
		border: 1px solid var(--color-border);
		border-radius: 4px;
		padding: 0.75rem;
		max-width: 42rem;
	}
	.status.done_with_errors,
	.bad,
	.delivery.failed {
		color: var(--color-danger);
	}
	.status.running,
	.warn,
	.delivery.pending,
	.delivery.sending {
		color: var(--color-warning);
	}
	.delivery.delivered {
		color: var(--color-success);
	}
	.delivery.none {
		color: var(--color-text-muted);
	}
	.batch-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
		margin: 1.25rem 0;
	}
	.btn-preview {
		background: none;
		border: none;
		padding: 0;
		color: var(--color-primary);
		text-decoration: underline;
		cursor: pointer;
		font-size: 0.8125rem;
		margin-top: 0.25rem;
	}
	.btn-preview:hover {
		color: var(--color-primary-hover, var(--color-primary));
	}
	.preview-row td {
		background: var(--color-surface-muted);
		padding: 0.75rem;
		border-bottom: 1px solid var(--color-border);
	}
	.preview-card {
		padding: 0.5rem;
	}
	.preview-content {
		white-space: pre-wrap;
		font: inherit;
		margin: 0.5rem 0 0;
		padding: 0.5rem;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-sm, 4px);
	}
	.excluded-section {
		margin-top: 2rem;
		padding-top: 1rem;
		border-top: 1px solid var(--color-border);
	}
	.excluded-list {
		margin: 0.5rem 0 0 1.5rem;
		padding: 0;
	}
	.hint {
		color: var(--color-text-muted);
		font-size: 0.875rem;
		margin: 0.25rem 0;
	}
</style>
