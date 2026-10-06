<script lang="ts">
	import { untrack } from 'svelte';
	import { previewMessage, type SendEstimate } from '$lib/api/messages';
	import type { PageProps } from './$types';
	import {
		costText,
		estimateWarnings,
		partsText,
		previewFailureText,
		recipientsText,
		smsText
	} from './estimate';

	// Wait for a pause in typing before asking the backend again.
	const PREVIEW_DELAY_MS = 300;

	let { data }: PageProps = $props();

	let subject = $state('');
	// Sent exactly as typed: the composer never trims or reformats the body.
	let body = $state('');
	let selectedGroupIds = $state<string[]>([]);

	let estimate = $state<SendEstimate | null>(null);
	let failure = $state<string | null>(null);
	let counting = $state(false);
	let latestRequest = 0;

	const warnings = $derived(estimate ? estimateWarnings(estimate) : []);
	const hasRecipients = $derived((estimate?.recipientCount ?? 0) > 0);
	// Counts and cost are only meaningful once the body renders for the recipients.
	const renders = $derived(
		estimate !== null && estimate.template.parts > 0 && estimate.unknownPlaceholders.length === 0
	);

	$effect(() => {
		// The subject does not change the estimate, so typing it triggers no request.
		const draft = {
			subject: untrack(() => subject),
			body,
			selection: { groupIds: [...selectedGroupIds] }
		};
		const request = ++latestRequest;
		counting = true;
		const timer = setTimeout(async () => {
			try {
				const result = await previewMessage(draft);
				// Drop a late answer for a draft that has changed since.
				if (request === latestRequest) {
					estimate = result;
					failure = null;
				}
			} catch (err) {
				if (request === latestRequest) failure = previewFailureText(err);
			} finally {
				if (request === latestRequest) counting = false;
			}
		}, PREVIEW_DELAY_MS);
		return () => clearTimeout(timer);
	});
</script>

<svelte:head>
	<title>Nowa wiadomość — DoYouSend</title>
</svelte:head>

<h1>Nowa wiadomość</h1>
<p class="lead">
	Jedna treść trafia do odbiorców jednocześnie e-mailem i SMS-em — w obu kanałach dokładnie taka
	sama. Temat dotyczy tylko e-maila, więc wszystko, co ważne, wpisz w treść.
</p>

<div class="composer">
	<form class="draft" onsubmit={(e) => e.preventDefault()}>
		<label for="subject">Temat e-maila</label>
		<input id="subject" name="subject" type="text" bind:value={subject} autocomplete="off" />

		<label for="body">Treść wiadomości</label>
		<textarea id="body" name="body" rows="8" bind:value={body} aria-describedby="body-hint"
		></textarea>
		<p id="body-hint" class="hint">
			Możesz wstawić <code>{'{{imie}}'}</code> i <code>{'{{nazwisko}}'}</code> — każdy odbiorca dostanie
			swoje dane, tak samo w e-mailu i w SMS-ie.
		</p>

		<fieldset>
			<legend>Odbiorcy</legend>
			{#if data.groupsFailed}
				<p class="error" role="alert">Nie udało się wczytać listy grup. Odśwież stronę.</p>
			{:else if data.groups.length === 0}
				<p class="hint">Nie ma jeszcze żadnych grup.</p>
			{:else}
				<ul class="groups">
					{#each data.groups as group (group.id)}
						<li>
							<label>
								<input type="checkbox" value={group.id} bind:group={selectedGroupIds} />
								<span class="group-name">{group.name}</span>
								<span class="group-meta">
									{recipientsText(group.memberCount)}{group.kind === 'system' ? ' · systemowa' : ''}
								</span>
							</label>
						</li>
					{/each}
				</ul>
				<p class="hint">Osoba w kilku zaznaczonych grupach dostanie wiadomość tylko raz.</p>
			{/if}
		</fieldset>
	</form>

	<aside class="summary" aria-labelledby="summary-heading" aria-busy={counting}>
		<h2 id="summary-heading">Podsumowanie</h2>

		{#if failure}
			<p class="error" role="alert">{failure}</p>
		{/if}

		{#if estimate}
			<div aria-live="polite">
				<p class="template">
					{#if estimate.template.parts === 0}
						Wpisz treść, żeby zobaczyć liczbę SMS-ów.
					{:else}
						Sama treść: <strong>{smsText(estimate.template.parts)}</strong> na odbiorcę
						<span class="encoding">
							({estimate.template.encoding === 'UCS2' ? 'UCS-2' : 'GSM-7'}, długość {estimate
								.template.units})
						</span>
					{/if}
				</p>
				{#if estimate.template.encoding === 'UCS2'}
					<p class="hint">
						Polskie litery lub emoji wymuszają kodowanie UCS-2: 70 znaków w jednym SMS-ie zamiast
						160. Treść zostanie wysłana bez zmian.
					</p>
				{/if}

				{#if hasRecipients}
					<dl>
						<dt>Odbiorcy</dt>
						<dd>{estimate.recipientCount}</dd>
						{#if renders}
							<dt>E-maile</dt>
							<dd>{estimate.emailCount}</dd>
							<dt>SMS-y do</dt>
							<dd>{recipientsText(estimate.smsCount)}</dd>
							{#if estimate.smsCount > 0}
								<dt>Części SMS na odbiorcę</dt>
								<dd>{partsText(estimate.minPartsPerRecipient, estimate.maxPartsPerRecipient)}</dd>
							{/if}
							<dt>Łącznie</dt>
							<dd>{smsText(estimate.totalSmsParts)}</dd>
							<dt class="cost">Szacowany koszt</dt>
							<dd class="cost">{costText(estimate.costMilli)}</dd>
						{/if}
					</dl>
				{:else}
					<p class="hint">Zaznacz co najmniej jedną grupę.</p>
				{/if}

				{#if warnings.length > 0}
					<ul class="warnings">
						{#each warnings as warning (warning)}
							<li>{warning}</li>
						{/each}
					</ul>
				{/if}
			</div>
		{:else if !failure}
			<p class="hint">Liczę…</p>
		{/if}

		<button type="button" disabled>Dalej: zatwierdź wysyłkę</button>
		<p class="hint">Zatwierdzanie i wysyłka pojawią się w kolejnej wersji.</p>
	</aside>
</div>

<style>
	.lead {
		max-width: 42rem;
	}
	.composer {
		display: grid;
		grid-template-columns: minmax(0, 3fr) minmax(16rem, 2fr);
		gap: 2rem;
		align-items: start;
	}
	@media (max-width: 48rem) {
		.composer {
			grid-template-columns: 1fr;
		}
	}
	.draft {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	label[for] {
		font-weight: 600;
		margin-top: 0.8rem;
	}
	input[type='text'],
	textarea {
		font: inherit;
		padding: 0.5rem;
		border: 1px solid #bbb;
		border-radius: 4px;
	}
	textarea {
		resize: vertical;
	}
	input[type='text']:focus-visible,
	textarea:focus-visible,
	input[type='checkbox']:focus-visible {
		outline: 2px solid #1d4ed8;
		outline-offset: 1px;
	}
	fieldset {
		margin-top: 1rem;
		border: 1px solid #ddd;
		border-radius: 4px;
	}
	legend {
		font-weight: 600;
	}
	.groups {
		list-style: none;
		padding: 0;
		margin: 0;
	}
	.groups label {
		display: flex;
		gap: 0.5rem;
		align-items: baseline;
		padding: 0.3rem 0;
		cursor: pointer;
	}
	.group-meta {
		color: #666;
		font-size: 0.9em;
	}
	.summary {
		position: sticky;
		top: 1rem;
		padding: 1rem 1.25rem;
		border: 1px solid #ddd;
		border-radius: 6px;
		background: #fafafa;
	}
	.summary[aria-busy='true'] dl {
		opacity: 0.6;
		transition: opacity 150ms ease-out;
	}
	.summary h2 {
		margin-top: 0;
	}
	dl {
		display: grid;
		grid-template-columns: auto auto;
		/* No column gap, so the line above the cost runs unbroken. */
		gap: 0.3rem 0;
	}
	dt {
		padding-right: 1rem;
	}
	dd {
		margin: 0;
		text-align: right;
		font-variant-numeric: tabular-nums;
	}
	.cost {
		font-weight: 700;
		border-top: 1px solid #ddd;
		padding-top: 0.4rem;
	}
	.encoding {
		color: #666;
	}
	.warnings {
		padding-left: 1.1rem;
		color: #8a4b00;
	}
	.hint {
		color: #555;
		font-size: 0.9em;
		margin: 0.2rem 0;
	}
	.error {
		color: #b00020;
	}
	button {
		margin-top: 1rem;
		font: inherit;
		padding: 0.5rem 1rem;
	}
</style>
