<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { sendBatch } from '$lib/api/batches';
	import { resolveSelection, type ResolvedRecipient } from '$lib/api/groups';
	import { previewMessage, type SendEstimate } from '$lib/api/messages';
	import { fixRecipientContact, type RecipientLookup } from '$lib/api/recipients';
	import RecipientPicker from '$lib/components/RecipientPicker.svelte';
	import { contactFixErrorText, missingChannels, validateContactFixInput } from './fix';
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

	let showCopyBanner = $state(true);

	let subject = $state(untrack(() => data.initialDraft?.subject ?? ''));
	// Sent exactly as typed: the composer never trims or reformats the body.
	let body = $state(untrack(() => data.initialDraft?.body ?? ''));
	let selectedGroupIds = $state<string[]>(untrack(() => data.initialDraft?.groupIds ?? []));
	let selectedRecipients = $state<RecipientLookup[]>(
		untrack(() => data.initialDraft?.recipients ?? [])
	);
	const selectedRecipientIds = $derived(selectedRecipients.map((r) => r.id));
	const selectedIdSet = $derived(new Set(selectedRecipientIds));

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

	let refreshTrigger = $state(0);
	let channelMode = $state<'both' | 'email' | 'sms'>('both');
	const targetChannels = $derived<('email' | 'sms')[]>(
		channelMode === 'email' ? ['email'] : channelMode === 'sms' ? ['sms'] : ['email', 'sms']
	);

	function activeNeeds(inc: ResolvedRecipient) {
		const full = missingChannels(inc.channels);
		return {
			needsEmail: channelMode !== 'sms' && full.needsEmail,
			needsPhone: channelMode !== 'email' && full.needsPhone
		};
	}

	let fixingRecipientId = $state<string | null>(null);
	let fixEmail = $state('');
	let fixPhone = $state('');
	let fixError = $state<string | null>(null);
	let isFixingSaving = $state(false);
	let fixSuccessMessage = $state<string | null>(null);

	function startFixing(inc: ResolvedRecipient) {
		fixingRecipientId = inc.recipient.id;
		fixEmail = '';
		fixPhone = '';
		fixError = null;
		fixSuccessMessage = null;
	}

	function cancelFixing() {
		fixingRecipientId = null;
		fixEmail = '';
		fixPhone = '';
		fixError = null;
	}

	async function submitFix(inc: ResolvedRecipient) {
		const needs = activeNeeds(inc);
		const valErr = validateContactFixInput({ email: fixEmail, phone: fixPhone }, needs);
		if (valErr) {
			fixError = valErr;
			return;
		}

		const patch: { email?: string; phone?: string } = {};
		if (needs.needsEmail && fixEmail.trim()) patch.email = fixEmail.trim();
		if (needs.needsPhone && fixPhone.trim()) patch.phone = fixPhone.trim();

		isFixingSaving = true;
		fixError = null;
		try {
			await fixRecipientContact(inc.recipient.id, patch);
			const name = `${inc.recipient.firstName} ${inc.recipient.lastName}`.trim();
			fixSuccessMessage = `Uzupełniono dane kontaktowe dla: ${name}.`;
			fixingRecipientId = null;
			fixEmail = '';
			fixPhone = '';
			refreshTrigger++;
		} catch (err) {
			fixError = contactFixErrorText(err);
		} finally {
			isFixingSaving = false;
		}
	}

	const incompleteRecipients = $derived(
		resolvedRecipients.filter((r) => {
			if (channelMode === 'email') return !r.channels.includes('email');
			if (channelMode === 'sms') return !r.channels.includes('sms');
			return r.channels.length < 2;
		})
	);

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

	const ready = $derived(
		canConfirm(subject, estimate, counting, targetChannels) && failure === null
	);

	function openConfirmation() {
		idempotencyKey = newIdempotencyKey();
		sendFailure = null;
		confirming = true;
	}

	function closeConfirmation() {
		confirming = false;
		sendFailure = null;
	}

	function togglePeople(people: RecipientLookup[], selected: boolean) {
		if (selected) {
			const known = new Set(selectedRecipients.map((r) => r.id));
			selectedRecipients = [...selectedRecipients, ...people.filter((r) => !known.has(r.id))];
		} else {
			const drop = new Set(people.map((r) => r.id));
			selectedRecipients = selectedRecipients.filter((r) => !drop.has(r.id));
		}
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
					channels: targetChannels,
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
		if (refreshTrigger < 0) return;
		// The subject does not change the estimate, so typing it triggers no request.
		const draft = {
			subject: untrack(() => subject),
			body,
			channels: targetChannels,
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
		if (refreshTrigger < 0) return;
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

{#if data.copiedBatchSubject && showCopyBanner}
	<aside class="copy-banner" role="status" aria-label="Informacja o załadowaniu kopii">
		<span class="copy-banner-text">
			Załadowano treść i grupy na podstawie wcześniejszej wysyłki: <strong
				>{data.copiedBatchSubject}</strong
			>.
		</span>
		<button
			type="button"
			class="copy-banner-dismiss"
			onclick={() => (showCopyBanner = false)}
			aria-label="Zamknij powiadomienie"
		>
			✕
		</button>
	</aside>
{/if}

<div class="composer">
	<form class="draft" onsubmit={(e) => e.preventDefault()}>
		<fieldset class="frozen" disabled={confirming}>
			<div class="channel-mode-container">
				<span class="channel-mode-title">Kanały wysyłki</span>
				<div class="channel-mode-group" role="radiogroup" aria-label="Kanały wysyłki">
					<label class="channel-radio-label" class:selected={channelMode === 'both'}>
						<input type="radio" name="channelMode" value="both" bind:group={channelMode} />
						<span class="channel-radio-content">
							<strong class="channel-name">E-mail i SMS</strong>
							<span class="channel-desc">Dwa kanały jednocześnie (zalecane)</span>
						</span>
					</label>
					<label class="channel-radio-label" class:selected={channelMode === 'email'}>
						<input type="radio" name="channelMode" value="email" bind:group={channelMode} />
						<span class="channel-radio-content">
							<strong class="channel-name">Tylko E-mail</strong>
							<span class="channel-desc">Wysyłka wyłącznie e-mailem (brak kosztu SMS)</span>
						</span>
					</label>
					<label class="channel-radio-label" class:selected={channelMode === 'sms'}>
						<input type="radio" name="channelMode" value="sms" bind:group={channelMode} />
						<span class="channel-radio-content">
							<strong class="channel-name">Tylko SMS</strong>
							<span class="channel-desc">Szybkie powiadomienie na telefon</span>
						</span>
					</label>
				</div>
			</div>

			{#if channelMode !== 'sms'}
				<label for="subject">Temat e-maila</label>
				<input id="subject" name="subject" type="text" bind:value={subject} autocomplete="off" />
			{:else}
				<label for="subject" class="optional-label">
					Temat e-maila <span class="hint-tag">(niewymagany przy wysyłce tylko SMS)</span>
				</label>
				<input
					id="subject"
					name="subject"
					type="text"
					bind:value={subject}
					placeholder="Opcjonalny (wiadomość zostanie wysłana wyłącznie przez SMS)"
					autocomplete="off"
				/>
			{/if}

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
					{#if channelMode !== 'sms' && subject.trim()}
						<p class="preview-subject"><strong>Temat e-maila:</strong> {subject}</p>
					{/if}
					<div class="preview-body">{estimate?.sampleBody || body}</div>
					<p class="preview-note">
						{#if channelMode === 'email'}
							Wiadomość zostanie wysłana wyłącznie na adresy e-mail odbiorców.
						{:else if channelMode === 'sms'}
							Wiadomość zostanie wysłana wyłącznie przez SMS na numery telefonów.
						{:else}
							E-mail i SMS mają tę samą treść; <code>{'{{imie}}'}</code> i
							<code>{'{{nazwisko}}'}</code> każdy odbiorca dostaje ze swoimi danymi.
						{/if}
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

				<details class="person-picker" open={selectedRecipients.length > 0}>
					<summary>Wybierz osoby z listy odbiorców</summary>
					<RecipientPicker
						initial={data.people}
						pageSize={10}
						selectedIds={selectedIdSet}
						ontoggle={togglePeople}
						caption="Odbiorcy do wyboru"
					/>
				</details>

				{#if fixSuccessMessage}
					<p class="notice" role="status">
						{fixSuccessMessage}
					</p>
				{/if}

				{#if incompleteRecipients.length > 0}
					<div class="incomplete-section">
						<h4 class="incomplete-heading">
							{#if channelMode === 'email'}
								Osoby bez adresu e-mail ({incompleteRecipients.length})
							{:else if channelMode === 'sms'}
								Osoby bez numeru telefonu ({incompleteRecipients.length})
							{:else}
								Osoby z niepełnymi danymi kontaktowymi ({incompleteRecipients.length})
							{/if}
						</h4>
						<p class="hint">
							{#if channelMode === 'email'}
								Te osoby nie mają adresu e-mail wymaganego do tej wysyłki. Możesz uzupełnić
								brakujący adres lub wykluczyć odbiorcę z tej wysyłki:
							{:else if channelMode === 'sms'}
								Te osoby nie mają numeru telefonu wymaganego do tej wysyłki. Możesz uzupełnić
								brakujący numer lub wykluczyć odbiorcę z tej wysyłki:
							{:else}
								Te osoby nie mają kompletu danych kontaktowych. Możesz uzupełnić brakujące dane
								przed wysyłką lub wykluczyć odbiorcę z tej wiadomości:
							{/if}
						</p>
						<ul class="incomplete-list">
							{#each incompleteRecipients as inc (inc.recipient.id)}
								{@const needs = activeNeeds(inc)}
								<li>
									<div class="inc-row">
										<div class="inc-details">
											<strong>{inc.recipient.lastName} {inc.recipient.firstName}</strong>
											<span class="inc-badge">
												{#if inc.channels.length === 0}
													brak e-maila i telefonu
												{:else if needs.needsEmail}
													brak e-maila
												{:else if needs.needsPhone}
													brak telefonu
												{/if}
											</span>
										</div>
										<div class="inc-actions">
											<button type="button" class="button btn-fix" onclick={() => startFixing(inc)}>
												Popraw dane
											</button>
											<button
												type="button"
												class="btn-exclude"
												onclick={() => excludeRecipient(inc)}
											>
												Wyklucz z wysyłki
											</button>
										</div>
									</div>

									{#if fixingRecipientId === inc.recipient.id}
										<div class="fix-form">
											<div class="fix-heading">
												Uzupełnij brakujące dane: <strong
													>{inc.recipient.firstName} {inc.recipient.lastName}</strong
												>
											</div>
											{#if fixError}
												<div class="alert-warning" role="alert">{fixError}</div>
											{/if}
											<div class="fix-inputs">
												{#if needs.needsEmail}
													<label class="fix-label">
														<span>Adres e-mail:</span>
														<input
															type="email"
															placeholder="np. jan.kowalski@example.test"
															bind:value={fixEmail}
															disabled={isFixingSaving}
														/>
													</label>
												{/if}
												{#if needs.needsPhone}
													<label class="fix-label">
														<span>Numer telefonu:</span>
														<input
															type="tel"
															placeholder="np. +48 500 100 101"
															bind:value={fixPhone}
															disabled={isFixingSaving}
														/>
													</label>
												{/if}
											</div>
											<div class="fix-buttons">
												<button
													type="button"
													class="button primary"
													disabled={isFixingSaving}
													onclick={() => submitFix(inc)}
												>
													{isFixingSaving ? 'Zapisywanie...' : 'Zapisz'}
												</button>
												<button
													type="button"
													class="button"
													disabled={isFixingSaving}
													onclick={cancelFixing}
												>
													Anuluj
												</button>
											</div>
										</div>
									{/if}
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
					{#if channelMode === 'email'}
						Tryb: <strong>Tylko E-mail</strong> (wiadomość nie zużywa pakietu SMS).
					{:else if estimate.template.parts === 0}
						Wpisz treść, żeby zobaczyć liczbę SMS-ów.
					{:else}
						Sama treść: <strong>{smsText(estimate.template.parts)}</strong> na odbiorcę
						<span class="encoding">
							({estimate.template.encoding === 'UCS2' ? 'UCS-2' : 'GSM-7'}, długość {estimate
								.template.units})
						</span>
					{/if}
				</p>
				{#if channelMode !== 'email' && estimate.template.encoding === 'UCS2'}
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
							{#if channelMode !== 'sms'}
								<dt>E-maile</dt>
								<dd>{estimate.emailCount}</dd>
							{/if}
							{#if channelMode !== 'email'}
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
				{#if channelMode !== 'email'}
					<p>Szacowany koszt SMS: <strong>{costText(estimate.costMilli)}</strong></p>
				{/if}
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
					{#if channelMode === 'email'}
						Wysyłki nie da się cofnąć. Każdy odbiorca dostanie wiadomość e-mail.
					{:else if channelMode === 'sms'}
						Wysyłki nie da się cofnąć. Każdy odbiorca dostanie wiadomość SMS.
					{:else}
						Wysyłki nie da się cofnąć. Każdy odbiorca dostanie tę samą treść e-mailem i SMS-em.
					{/if}
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
			<button type="button" class="primary" onclick={openConfirmation} disabled={!ready}>
				Dalej: zatwierdź wysyłkę
			</button>
			{#if !ready && estimate && hasRecipients && channelMode !== 'sms' && subject.trim() === ''}
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
	fieldset {
		margin-top: 1rem;
		border: 1px solid var(--color-border);
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
		color: var(--color-text-muted);
		font-size: 0.9em;
	}
	.summary {
		position: sticky;
		top: 1rem;
		padding: 1rem 1.25rem;
		border: 1px solid var(--color-border);
		border-radius: 6px;
		background: var(--color-surface-muted);
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
		border-top: 1px solid var(--color-border);
		padding-top: 0.4rem;
	}
	.encoding {
		color: var(--color-text-muted);
	}
	.warnings {
		padding-left: 1.1rem;
		color: var(--color-warning);
	}
	button {
		margin-top: var(--space-4);
	}
	.frozen {
		background: none;
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
		border-top: 1px solid var(--color-border);
	}
	.confirm h3 {
		margin: 0 0 0.5rem;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
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
		background: var(--color-border);
		border: 1px solid var(--color-border);
		border-radius: 16px;
		padding: 0.2rem 0.6rem;
		font-size: 0.9em;
	}
	.chip-remove {
		min-height: 0;
		background: none;
		border: none;
		color: var(--color-text-muted);
		cursor: pointer;
		font-weight: bold;
		padding: 0;
		margin: 0;
		line-height: 1;
		font-size: 1.1em;
	}
	.chip-remove:hover {
		color: var(--color-danger);
	}
	.person-picker {
		margin-top: 0.5rem;
	}
	.person-picker summary {
		cursor: pointer;
		font-weight: 600;
	}
	.message-preview-card {
		margin-top: 0.75rem;
		border: 1px solid var(--color-primary);
		background: var(--color-primary-soft);
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
		color: var(--color-primary-hover);
		font-size: 0.95em;
	}
	.preview-target {
		font-size: 0.85em;
		color: var(--color-primary-hover);
	}
	.preview-subject {
		margin: 0.2rem 0 0.4rem;
		font-size: 0.9em;
		color: var(--color-text);
	}
	.preview-body {
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: 4px;
		padding: 0.6rem 0.8rem;
		font-family: inherit;
		white-space: pre-wrap;
		font-size: 0.95em;
		color: var(--color-text);
	}
	.preview-note {
		margin: 0.4rem 0 0;
		font-size: 0.8em;
		color: var(--color-text-muted);
	}
	.incomplete-section {
		margin-top: 1.25rem;
		background: var(--color-warning-soft);
		border: 1px solid var(--color-warning-border);
		border-radius: 6px;
		padding: 0.75rem 1rem;
	}
	.incomplete-heading {
		margin: 0 0 0.25rem;
		color: var(--color-warning);
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
		flex-direction: column;
		padding: 0.5rem;
		background: var(--color-surface);
		border: 1px solid var(--color-warning-border);
		border-radius: var(--radius);
		gap: 0.5rem;
	}
	.inc-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
	}
	.inc-details {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}
	.inc-badge {
		font-size: 0.8em;
		color: var(--color-warning);
	}
	.inc-actions {
		display: flex;
		gap: 0.4rem;
		align-items: center;
	}
	.btn-fix {
		min-height: 0;
		font-weight: 600;
		background: var(--color-primary-soft);
		color: var(--color-primary);
		border: 1px solid var(--color-primary);
		border-radius: 4px;
		padding: 0.2rem 0.6rem;
		font-size: 0.85em;
		cursor: pointer;
		margin: 0;
		white-space: nowrap;
	}
	.btn-fix:hover:not(:disabled) {
		background: var(--color-primary);
		color: var(--color-on-primary);
	}
	.btn-exclude {
		min-height: 0;
		font-weight: 600;
		background: var(--color-danger-soft);
		color: var(--color-danger-hover);
		border: 1px solid var(--color-danger-border);
		border-radius: 4px;
		padding: 0.2rem 0.6rem;
		font-size: 0.85em;
		cursor: pointer;
		margin: 0;
		white-space: nowrap;
	}
	.btn-exclude:hover {
		background: var(--color-danger-soft);
	}
	.fix-form {
		padding: 0.6rem 0.75rem;
		background: var(--color-surface-muted);
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.fix-heading {
		font-size: 0.9em;
		color: var(--color-text);
	}
	.fix-inputs {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
	}
	.fix-label {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		font-size: 0.85em;
		color: var(--color-text-muted);
		flex: 1 1 12rem;
	}
	.fix-buttons {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.25rem;
	}
	.fix-buttons .button {
		min-height: 0;
		padding: 0.25rem 0.75rem;
		font-size: 0.85em;
	}
	.excluded-section {
		margin-top: 1rem;
	}
	.excluded-section h4 {
		margin: 0 0 0.3rem;
		font-size: 0.9em;
		color: var(--color-text-muted);
	}
	.chip-excluded {
		background: var(--color-surface-muted);
		border-color: var(--color-border);
		color: var(--color-text-muted);
	}
	.chip-excluded span {
		text-decoration: line-through;
	}
	.confirm-preview {
		margin-top: 0.75rem;
		padding: 0.5rem;
		background: var(--color-surface-muted);
		border: 1px solid var(--color-border);
		border-radius: 4px;
		font-size: 0.9em;
	}
	.confirm-body {
		margin: 0.4rem 0 0;
		font-style: normal;
		white-space: pre-wrap;
		color: var(--color-text);
		border-left: 3px solid var(--color-primary);
		padding-left: 0.5rem;
	}
	.copy-banner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		background: var(--color-surface-muted);
		border: 1px solid var(--color-border);
		border-left: 4px solid var(--color-primary);
		border-radius: var(--radius-sm, 4px);
		padding: 0.75rem 1rem;
		margin: 1rem 0;
	}
	.copy-banner-text {
		color: var(--color-text);
		font-size: 0.9375rem;
	}
	.copy-banner-dismiss {
		background: transparent;
		border: none;
		color: var(--color-text-muted);
		font-size: 1rem;
		cursor: pointer;
		padding: 0.25rem 0.5rem;
		line-height: 1;
	}
	.copy-banner-dismiss:hover {
		color: var(--color-text);
	}
	.channel-mode-container {
		margin-bottom: 1.5rem;
		padding: 1rem;
		background: var(--color-surface-muted, #f8fafc);
		border: 1px solid var(--color-border, #e2e8f0);
		border-radius: 8px;
	}
	.channel-mode-title {
		display: block;
		font-weight: 600;
		font-size: 0.95rem;
		margin-bottom: 0.75rem;
		color: var(--color-text, #1e293b);
	}
	.channel-mode-group {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 0.75rem;
	}
	.channel-radio-label {
		display: flex;
		align-items: flex-start;
		gap: 0.6rem;
		padding: 0.75rem 1rem;
		border: 1px solid var(--color-border, #cbd5e1);
		border-radius: 6px;
		background: var(--color-surface, #ffffff);
		cursor: pointer;
		transition: all 150ms ease;
	}
	.channel-radio-label:hover {
		border-color: var(--color-primary, #2563eb);
		background: var(--color-surface-hover, #f1f5f9);
	}
	.channel-radio-label.selected {
		border-color: var(--color-primary, #2563eb);
		background: var(--color-primary-soft, #eff6ff);
		box-shadow: 0 0 0 1px var(--color-primary, #2563eb);
	}
	.channel-radio-content {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}
	.channel-name {
		font-size: 0.95rem;
		color: var(--color-text, #0f172a);
	}
	.channel-desc {
		font-size: 0.8rem;
		color: var(--color-text-muted, #64748b);
	}
	.optional-label .hint-tag {
		font-weight: normal;
		font-size: 0.85em;
		color: var(--color-text-muted, #64748b);
	}
</style>
