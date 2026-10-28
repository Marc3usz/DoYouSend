<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { sendBatch } from '$lib/api/batches';
	import { resolveSelection, type ResolvedRecipient } from '$lib/api/groups';
	import { previewMessage, type SendEstimate } from '$lib/api/messages';
	import { lookupRecipients, type RecipientLookup } from '$lib/api/recipients';
	import { recipientTypeLabel } from '../odbiorcy/messages';
	import type { PageProps } from './$types';
	import {
		costText,
		estimateWarnings,
		partsText,
		previewFailureText,
		recipientsText,
		smsText
	} from './estimate';
	import { canConfirm, confirmText, newIdempotencyKey, sendFailureText } from './send';

	// Wait for a pause in typing before asking the backend again.
	const PREVIEW_DELAY_MS = 300;

	let { data }: PageProps = $props();

	let subject = $state(untrack(() => data.initialDraft?.subject ?? ''));
	// Sent exactly as typed: the composer never trims or reformats the body.
	let body = $state(untrack(() => data.initialDraft?.body ?? ''));
	let selectedGroupIds = $state<string[]>(untrack(() => data.initialDraft?.groupIds ?? []));
	let selectedRecipients = $state<RecipientLookup[]>(
		untrack(() => data.initialDraft?.recipients ?? [])
	);
	const selectedRecipientIds = $derived(selectedRecipients.map((r) => r.id));

	let excludedRecipientIds = $state<string[]>(
		untrack(() => data.initialDraft?.excludedRecipientIds ?? [])
	);
	let excludedPeople = $state<{ id: string; name: string }[]>([]);
	let resolvedRecipients = $state<ResolvedRecipient[]>([]);
	let latestResolve = 0;

	function excludeRecipient(r: ResolvedRecipient) {
		const id = r.recipient.id;
		if (!excludedRecipientIds.includes(id)) {
			excludedRecipientIds = [...excludedRecipientIds, id];
			const name = `${r.recipient.lastName} ${r.recipient.firstName}`.trim() || id;
			excludedPeople = [...excludedPeople.filter((p) => p.id !== id), { id, name }];
		}
	}

	function unexcludeRecipient(id: string) {
		excludedRecipientIds = excludedRecipientIds.filter((x) => x !== id);
		excludedPeople = excludedPeople.filter((p) => p.id !== id);
	}

	const incompleteRecipients = $derived(resolvedRecipients.filter((r) => r.channels.length < 2));

	$effect(() => {
		if (resolvedRecipients.length > 0 && excludedRecipientIds.length > 0) {
			for (const id of excludedRecipientIds) {
				if (!excludedPeople.some((p) => p.id === id)) {
					const found = resolvedRecipients.find((r) => r.recipient.id === id);
					if (found) {
						const name = `${found.recipient.lastName} ${found.recipient.firstName}`.trim() || id;
						excludedPeople = [...excludedPeople, { id, name }];
					}
				}
			}
		}
	});

	let personQuery = $state('');
	let personSearching = $state(false);
	let personCandidates = $state<RecipientLookup[]>([]);
	let personSearchError = $state<string | null>(null);
	let latestPersonSearch = 0;

	let estimate = $state<SendEstimate | null>(null);
	let failure = $state<string | null>(null);
	let counting = $state(false);
	let latestRequest = 0;

	// The final confirmation (POST /batches). While it is open the draft is frozen,
	// so the sender confirms exactly what the summary shows.
	let confirming = $state(false);
	let sending = $state(false);
	let sendFailure = $state<string | null>(null);
	// A new key for every opening of the confirmation; a retry after a network error
	// reuses it, so the backend returns the same batch instead of sending twice.
	let idempotencyKey = '';

	const ready = $derived(canConfirm(subject, estimate, counting) && failure === null);

	function openConfirmation() {
		idempotencyKey = newIdempotencyKey();
		sendFailure = null;
		confirming = true;
	}

	function closeConfirmation() {
		confirming = false;
		sendFailure = null;
	}

	function onSearchPersonInput(e: Event) {
		const q = (e.target as HTMLInputElement).value;
		personQuery = q;
		if (q.trim().length < 2) {
			personCandidates = [];
			personSearching = false;
			return;
		}
		const req = ++latestPersonSearch;
		personSearching = true;
		personSearchError = null;
		setTimeout(async () => {
			if (req !== latestPersonSearch) return;
			try {
				const res = await lookupRecipients({ q: q.trim(), limit: 10 });
				if (req === latestPersonSearch) {
					const selSet = new Set(selectedRecipientIds);
					personCandidates = res.items.filter((r) => !selSet.has(r.id));
				}
			} catch {
				if (req === latestPersonSearch) {
					personSearchError = 'Nie udało się wyszukać osób.';
				}
			} finally {
				if (req === latestPersonSearch) {
					personSearching = false;
				}
			}
		}, 200);
	}

	function addPerson(r: RecipientLookup) {
		if (!selectedRecipients.some((x) => x.id === r.id)) {
			selectedRecipients = [...selectedRecipients, r];
		}
		personCandidates = personCandidates.filter((x) => x.id !== r.id);
	}

	function removePerson(id: string) {
		selectedRecipients = selectedRecipients.filter((x) => x.id !== id);
	}

	async function send() {
		sending = true;
		sendFailure = null;
		try {
			const batch = await sendBatch(
				{
					subject,
					body,
					selection: {
						groupIds: [...selectedGroupIds],
						recipientIds: [...selectedRecipientIds],
						excludedRecipientIds: [...excludedRecipientIds]
					}
				},
				idempotencyKey
			);
			await goto(`/historia/${batch.id}`);
		} catch (err) {
			sendFailure = sendFailureText(err);
		} finally {
			sending = false;
		}
	}

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
			selection: {
				groupIds: [...selectedGroupIds],
				recipientIds: [...selectedRecipientIds],
				excludedRecipientIds: [...excludedRecipientIds]
			}
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

	$effect(() => {
		const sel = {
			groupIds: [...selectedGroupIds],
			recipientIds: [...selectedRecipientIds],
			excludedRecipientIds: [...excludedRecipientIds]
		};
		if (sel.groupIds.length === 0 && sel.recipientIds.length === 0) {
			resolvedRecipients = [];
			return;
		}
		const req = ++latestResolve;
		const timer = setTimeout(async () => {
			try {
				const res = await resolveSelection(sel);
				if (req === latestResolve) {
					resolvedRecipients = res.recipients;
				}
			} catch {
				if (req === latestResolve) {
					resolvedRecipients = [];
				}
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
		<fieldset class="frozen" disabled={confirming}>
			<label for="subject">Temat e-maila</label>
			<input id="subject" name="subject" type="text" bind:value={subject} autocomplete="off" />

			<label for="body">Treść wiadomości</label>
			<textarea id="body" name="body" rows="8" bind:value={body} aria-describedby="body-hint"
			></textarea>
			<p id="body-hint" class="hint">
				Możesz wstawić <code>{'{{imie}}'}</code> i <code>{'{{nazwisko}}'}</code> — każdy odbiorca dostanie
				swoje dane, tak samo w e-mailu i w SMS-ie.
			</p>

			{#if body.trim() !== ''}
				<div class="message-preview-card">
					<div class="preview-header">
						<span class="preview-title">Podgląd wiadomości</span>
						{#if estimate?.sampleRecipientName}
							<span class="preview-target"
								>Dla: <strong>{estimate.sampleRecipientName}</strong></span
							>
						{/if}
					</div>
					{#if subject.trim()}
						<p class="preview-subject"><strong>Temat e-maila:</strong> {subject}</p>
					{/if}
					<div class="preview-body">{estimate?.sampleBody || body}</div>
					<p class="preview-note">
						E-mail i SMS mają tę samą treść; <code>{'{{imie}}'}</code> i
						<code>{'{{nazwisko}}'}</code> każdy odbiorca dostaje ze swoimi danymi.
					</p>
				</div>
			{/if}

			<fieldset>
				<legend>Odbiorcy</legend>

				<h4>Grupy odbiorców</h4>
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
										{recipientsText(group.memberCount)}{group.kind === 'system'
											? ' · systemowa'
											: ''}
									</span>
								</label>
							</li>
						{/each}
					</ul>
					<p class="hint">Osoba w kilku zaznaczonych grupach dostanie wiadomość tylko raz.</p>
				{/if}

				<h4 class="mt-section">Pojedynczy odbiorcy</h4>
				{#if selectedRecipients.length > 0}
					<div class="selected-chips">
						<p class="hint">Wybrani odbiorcy ({selectedRecipients.length}):</p>
						<ul class="chips">
							{#each selectedRecipients as r (r.id)}
								<li class="chip">
									<span>{r.lastName} {r.firstName}</span>
									<button
										type="button"
										class="chip-remove"
										onclick={() => removePerson(r.id)}
										aria-label={`Usuń ${r.firstName} ${r.lastName}`}>×</button
									>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				<div class="person-search">
					<label for="person-search-input">Dodaj pojedynczą osobę</label>
					<input
						id="person-search-input"
						type="search"
						placeholder="Wpisz nazwisko lub imię…"
						value={personQuery}
						oninput={onSearchPersonInput}
						autocomplete="off"
					/>
					{#if personSearching}
						<p class="hint">Szukam…</p>
					{:else if personSearchError}
						<p class="error" role="alert">{personSearchError}</p>
					{:else if personQuery.trim().length >= 2 && personCandidates.length === 0}
						<p class="hint">Brak pasujących osób.</p>
					{:else if personCandidates.length > 0}
						<ul class="candidates">
							{#each personCandidates as c (c.id)}
								<li>
									<span>
										<strong>{c.lastName} {c.firstName}</strong> ({recipientTypeLabel(c.type)})
									</span>
									<button type="button" class="btn-add-candidate" onclick={() => addPerson(c)}>
										+ Dodaj
									</button>
								</li>
							{/each}
						</ul>
					{/if}
				</div>

				{#if incompleteRecipients.length > 0}
					<div class="incomplete-section">
						<h4 class="incomplete-heading">
							Osoby z niepełnymi danymi kontaktowymi ({incompleteRecipients.length})
						</h4>
						<p class="hint">
							Te osoby nie mają kompletu danych kontaktowych. Zgodnie z wymaganiami możesz wykluczyć
							je z tej wysyłki:
						</p>
						<ul class="incomplete-list">
							{#each incompleteRecipients as inc (inc.recipient.id)}
								<li>
									<div class="inc-details">
										<strong>{inc.recipient.lastName} {inc.recipient.firstName}</strong>
										<span class="inc-badge">
											{#if inc.channels.length === 0}
												brak e-maila i telefonu
											{:else if !inc.channels.includes('email')}
												brak e-maila (tylko SMS)
											{:else}
												brak telefonu (tylko e-mail)
											{/if}
										</span>
									</div>
									<button type="button" class="btn-exclude" onclick={() => excludeRecipient(inc)}>
										Wyklucz z wysyłki
									</button>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if excludedRecipientIds.length > 0}
					<div class="excluded-section">
						<h4>Wykluczeni z tej wysyłki ({excludedRecipientIds.length})</h4>
						<ul class="chips">
							{#each excludedRecipientIds as id (id)}
								{@const person = excludedPeople.find((p) => p.id === id)}
								{@const resolved = resolvedRecipients.find((r) => r.recipient.id === id)}
								{@const displayName =
									person?.name ??
									(resolved
										? `${resolved.recipient.lastName} ${resolved.recipient.firstName}`.trim()
										: null) ??
									`Osoba wykluczona (${id.slice(0, 8)}…)`}
								<li class="chip chip-excluded">
									<span>{displayName}</span>
									<button
										type="button"
										class="chip-remove"
										onclick={() => unexcludeRecipient(id)}
										title="Przywróć odbiorcę do wysyłki"
										aria-label={`Przywróć ${displayName}`}>↺</button
									>
								</li>
							{/each}
						</ul>
					</div>
				{/if}
			</fieldset>
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

		{#if confirming && estimate}
			<section class="confirm" aria-labelledby="confirm-heading">
				<h3 id="confirm-heading">Zatwierdź wysyłkę</h3>
				<p>{confirmText(estimate)}</p>
				<p>Szacowany koszt SMS: <strong>{costText(estimate.costMilli)}</strong></p>
				{#if estimate.sampleBody}
					<div class="confirm-preview">
						<small
							><strong
								>Podgląd treści ({estimate.sampleRecipientName || 'przykładowy odbiorca'}):</strong
							></small
						>
						<div class="confirm-body">{estimate.sampleBody}</div>
					</div>
				{/if}
				<p class="hint">
					Wysyłki nie da się cofnąć. Każdy odbiorca dostanie tę samą treść e-mailem i SMS-em.
				</p>
				{#if sendFailure}
					<p class="error" role="alert">{sendFailure}</p>
				{/if}
				<div class="actions">
					<button type="button" class="primary" onclick={send} disabled={sending}>
						{sending ? 'Wysyłam…' : 'Wyślij teraz'}
					</button>
					<button type="button" onclick={closeConfirmation} disabled={sending}>
						Wróć do edycji
					</button>
				</div>
			</section>
		{:else}
			<button type="button" onclick={openConfirmation} disabled={!ready}>
				Dalej: zatwierdź wysyłkę
			</button>
			{#if !ready && estimate && hasRecipients && subject.trim() === ''}
				<p class="hint">Uzupełnij temat e-maila, żeby zatwierdzić wysyłkę.</p>
			{/if}
		{/if}
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
	.frozen {
		border: 0;
		margin: 0;
		padding: 0;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.confirm {
		margin-top: 1rem;
		padding-top: 0.75rem;
		border-top: 1px solid #ddd;
	}
	.confirm h3 {
		margin: 0 0 0.5rem;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
	.primary {
		background: #1d4ed8;
		color: #fff;
		border: 1px solid #1d4ed8;
		border-radius: 4px;
	}
	.primary:disabled {
		opacity: 0.6;
	}
	.mt-section {
		margin-top: 1.25rem;
		margin-bottom: 0.4rem;
	}
	.selected-chips {
		margin-bottom: 0.75rem;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		list-style: none;
		padding: 0;
		margin: 0.25rem 0 0;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		background: #e2e8f0;
		border: 1px solid #cbd5e1;
		border-radius: 16px;
		padding: 0.2rem 0.6rem;
		font-size: 0.9em;
	}
	.chip-remove {
		background: none;
		border: none;
		color: #64748b;
		cursor: pointer;
		font-weight: bold;
		padding: 0;
		margin: 0;
		line-height: 1;
		font-size: 1.1em;
	}
	.chip-remove:hover {
		color: #b00020;
	}
	.person-search {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		margin-top: 0.5rem;
	}
	.candidates {
		list-style: none;
		padding: 0;
		margin: 0.4rem 0 0;
		border: 1px solid #cbd5e1;
		border-radius: 4px;
		background: #fff;
		max-height: 12rem;
		overflow-y: auto;
	}
	.candidates li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.35rem 0.6rem;
		border-bottom: 1px solid #f1f5f9;
		gap: 0.5rem;
	}
	.candidates li:last-child {
		border-bottom: none;
	}
	.btn-add-candidate {
		background: #f1f5f9;
		border: 1px solid #cbd5e1;
		border-radius: 4px;
		padding: 0.2rem 0.5rem;
		font-size: 0.85em;
		cursor: pointer;
		margin: 0;
	}
	.btn-add-candidate:hover {
		background: #e2e8f0;
	}
	.message-preview-card {
		margin-top: 0.75rem;
		border: 1px solid #93c5fd;
		background: #eff6ff;
		border-radius: 6px;
		padding: 0.85rem 1rem;
	}
	.preview-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.4rem;
	}
	.preview-title {
		font-weight: 700;
		color: #1e40af;
		font-size: 0.95em;
	}
	.preview-target {
		font-size: 0.85em;
		color: #1e3a8a;
	}
	.preview-subject {
		margin: 0.2rem 0 0.4rem;
		font-size: 0.9em;
		color: #334155;
	}
	.preview-body {
		background: #fff;
		border: 1px solid #bfdbfe;
		border-radius: 4px;
		padding: 0.6rem 0.8rem;
		font-family: inherit;
		white-space: pre-wrap;
		font-size: 0.95em;
		color: #0f172a;
	}
	.preview-note {
		margin: 0.4rem 0 0;
		font-size: 0.8em;
		color: #64748b;
	}
	.incomplete-section {
		margin-top: 1.25rem;
		background: #fffbeb;
		border: 1px solid #fde68a;
		border-radius: 6px;
		padding: 0.75rem 1rem;
	}
	.incomplete-heading {
		margin: 0 0 0.25rem;
		color: #92400e;
		font-size: 0.95em;
	}
	.incomplete-list {
		list-style: none;
		padding: 0;
		margin: 0.5rem 0 0;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.incomplete-list li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.35rem 0.5rem;
		background: #fff;
		border: 1px solid #fef3c7;
		border-radius: 4px;
		gap: 0.5rem;
	}
	.inc-details {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}
	.inc-badge {
		font-size: 0.8em;
		color: #b45309;
	}
	.btn-exclude {
		background: #fee2e2;
		color: #991b1b;
		border: 1px solid #fca5a5;
		border-radius: 4px;
		padding: 0.2rem 0.6rem;
		font-size: 0.85em;
		cursor: pointer;
		margin: 0;
		white-space: nowrap;
	}
	.btn-exclude:hover {
		background: #fecaca;
	}
	.excluded-section {
		margin-top: 1rem;
	}
	.excluded-section h4 {
		margin: 0 0 0.3rem;
		font-size: 0.9em;
		color: #475569;
	}
	.chip-excluded {
		background: #f1f5f9;
		border-color: #cbd5e1;
		color: #64748b;
	}
	.chip-excluded span {
		text-decoration: line-through;
	}
	.confirm-preview {
		margin-top: 0.75rem;
		padding: 0.5rem;
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 4px;
		font-size: 0.9em;
	}
	.confirm-body {
		margin: 0.4rem 0 0;
		font-style: normal;
		white-space: pre-wrap;
		color: #1e293b;
		border-left: 3px solid #3b82f6;
		padding-left: 0.5rem;
	}
</style>
