<script lang="ts">
	import { getBatch, retryBatch, type BatchDetail } from '$lib/api/batches';
	import { errorBody } from '$lib/api/errors';
	import { batchTitle, channelModeLabels, channelModeOf, usesChannel } from '$lib/format/batches';
	import { costText, dateTimeText, plural, recipientsText, smsText } from '$lib/format/text';
	import type { PageProps } from './$types';
	import {
		batchStatusLabels,
		deliveryOf,
		deliveryText,
		excludedText,
		fromText,
		stillSending,
		timesText
	} from '../format';

	// How often a running batch is reloaded to show new statuses.
	const REFRESH_MS = 3000;

	let { data }: PageProps = $props();

	// Fresher copy of the batch fetched by the poll or returned by a retry. The
	// page load (with the excluded names) does not run again for that.
	let live = $state<BatchDetail | null>(null);
	const batch = $derived(live && live.id === data.batch.id ? live : data.batch);

	const mode = $derived(channelModeOf(batch.channels));
	const usesEmail = $derived(usesChannel(batch.channels, 'email'));
	const usesSms = $derived(usesChannel(batch.channels, 'sms'));
	const excludedCount = $derived(batch.excludedRecipientIds?.length ?? 0);
	const retriedCount = $derived(
		batch.recipients.reduce((sum, r) => sum + r.deliveries.filter((d) => d.attempts > 1).length, 0)
	);
	// Older backends do not split failures; a plain retry then covers them all.
	const retryable = $derived(batch.retryableCount ?? batch.failedCount);
	const uncertain = $derived(batch.uncertainCount ?? 0);

	let onlyFailed = $state(false);
	const shownRecipients = $derived(
		onlyFailed
			? batch.recipients.filter((r) => r.deliveries.some((d) => d.status === 'failed'))
			: batch.recipients
	);

	let retrying = $state(false);
	let retryError = $state<string | null>(null);
	let retryNotice = $state<string | null>(null);
	let expandedBodies = $state<Record<string, boolean>>({});

	function toggleBody(id: string) {
		expandedBodies[id] = !expandedBodies[id];
	}

	async function onRetry(includeUncertain: boolean) {
		if (
			includeUncertain &&
			!confirm(
				'Niepewne wiadomości mogły już dotrzeć do odbiorców — operator nie potwierdził ani wysłania, ani błędu. ' +
					'Ponowienie może sprawić, że część osób dostanie wiadomość dwa razy. Ponowić także je?'
			)
		) {
			return;
		}
		retrying = true;
		retryError = null;
		retryNotice = null;
		try {
			live = await retryBatch(batch.id, { includeUncertain });
			retryNotice = 'Ponowienie uruchomione. Statusy odświeżą się same.';
		} catch (err) {
			const body = errorBody(err);
			if (body?.code === 'no_failed_deliveries') {
				retryError = 'Brak nieudanych wiadomości do ponowienia.';
			} else if (body?.code === 'batch_running') {
				retryError = 'Wysyłka jest w trakcie — poczekaj, aż się zakończy.';
			} else if (body?.code === 'sender_not_allowed') {
				retryError =
					'Autor tej wysyłki nie może już wysyłać z jej adresu nadawcy (np. administrator cofnął dostęp do skrzynki). Użyj jej jako podstawy nowej wiadomości i wybierz innego nadawcę.';
			} else {
				retryError = 'Nie udało się uruchomić ponowienia. Spróbuj ponownie.';
			}
		} finally {
			retrying = false;
		}
	}

	$effect(() => {
		if (!stillSending(batch.status)) return;
		const id = batch.id;
		const timer = setInterval(async () => {
			try {
				live = await getBatch(id);
			} catch {
				// Keep the last statuses; the next tick tries again.
			}
		}, REFRESH_MS);
		return () => clearInterval(timer);
	});
</script>

<svelte:head>
	<title>{batchTitle(batch)} — Historia — DoYouSend</title>
</svelte:head>

<p><a href="/historia">← Historia wysyłek</a></p>
<h1>{batchTitle(batch)}</h1>

<dl class="summary">
	<dt>Status</dt>
	<dd>
		<span class="status {batch.status}">{batchStatusLabels[batch.status]}</span>
		{#if stillSending(batch.status)}<span class="hint">(odświeża się samo)</span>{/if}
	</dd>
	<dt>Kanały</dt>
	<dd>
		{channelModeLabels[mode]}
		{#if mode !== 'both'}
			<span class="warn">— wysyłka jednym kanałem</span>
		{/if}
	</dd>
	{#if usesEmail}
		<dt>Nadawca e-maila</dt>
		<dd>{fromText(batch.from)}</dd>
	{/if}
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
	{#if excludedCount > 0}
		<dt>Wykluczeni z wysyłki</dt>
		<dd><span class="warn">{excludedText(excludedCount)}</span></dd>
	{/if}
	{#if usesSms}
		<dt>SMS</dt>
		<dd>{smsText(batch.totalSmsParts)}, {costText(batch.costMilli)}</dd>
	{/if}
	{#if (batch.retryCount ?? 0) > 0}
		<dt>Ponowienia</dt>
		<dd>
			{timesText(batch.retryCount ?? 0)}, ostatnio {dateTimeText(batch.lastRetryAt)}
		</dd>
	{/if}
	{#if retriedCount > 0}
		<dt>Wiadomości ponawiane</dt>
		<dd>
			{retriedCount}
			{plural(retriedCount, 'wiadomość wymagała', 'wiadomości wymagały', 'wiadomości wymagało')} więcej
			niż jednej próby
		</dd>
	{/if}
</dl>

<div class="batch-actions">
	<a href="/wiadomosci?kopia={batch.id}" class="button">Użyj jako podstawy nowej wiadomości</a>
	{#if !stillSending(batch.status)}
		{#if retryable > 0}
			<button type="button" class="primary" onclick={() => onRetry(false)} disabled={retrying}>
				{retrying ? 'Uruchamiam ponowienie…' : `Ponów nieudane (${retryable})`}
			</button>
		{/if}
		{#if uncertain > 0}
			<button type="button" onclick={() => onRetry(true)} disabled={retrying}>
				Ponów także niepewne ({uncertain})
			</button>
		{/if}
	{/if}
</div>
{#if uncertain > 0 && !stillSending(batch.status)}
	<p class="hint">
		Niepewne to wiadomości, o których operator nie dał jednoznacznej odpowiedzi — mogły dotrzeć.
		Ponawiaj je tylko wtedy, gdy wiesz, że odbiorcy ich nie dostali.
	</p>
{/if}
<div aria-live="polite">
	{#if retryNotice}
		<p class="notice">{retryNotice}</p>
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
	{#if batch.failedCount > 0}
		<label class="filter">
			<input type="checkbox" bind:checked={onlyFailed} />
			Pokaż tylko odbiorców z błędem
		</label>
	{/if}
	<table>
		<thead>
			<tr><th scope="col">Odbiorca</th><th scope="col">E-mail</th><th scope="col">SMS</th></tr>
		</thead>
		<tbody>
			{#each shownRecipients as r (r.recipientId)}
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
							<button
								type="button"
								class="link btn-preview"
								aria-expanded={expandedBodies[r.recipientId] ?? false}
								onclick={() => toggleBody(r.recipientId)}
							>
								{expandedBodies[r.recipientId] ? 'Ukryj treść' : 'Podgląd spersonalizowanej treści'}
							</button>
						{/if}
					</td>
					<td class="delivery {email?.status ?? 'none'}">{deliveryText(email, usesEmail)}</td>
					<td class="delivery {sms?.status ?? 'none'}">{deliveryText(sms, usesSms)}</td>
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
			{:else}
				<tr><td colspan="3">Nikt nie ma błędu.</td></tr>
			{/each}
		</tbody>
	</table>
{/if}

{#if data.excludedRecipients.length > 0}
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
		border-radius: var(--radius);
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
	.filter {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: normal;
		margin-bottom: 0.75rem;
	}
	.btn-preview {
		font-size: var(--text-sm);
		margin-top: 0.25rem;
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
		border-radius: var(--radius);
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
		margin: 0.25rem 0;
	}
</style>
