<script lang="ts">
	import { untrack } from 'svelte';
	import { enhance } from '$app/forms';
	import type { SettingItem } from '$lib/api/admin';
	import type { PageProps } from './$types';
	import { CONFIRM_PHRASE, groupLabels, labelOf, sourceLabels } from './settings';

	let { data, form }: PageProps = $props();

	const groups: SettingItem['group'][] = ['general', 'email', 'sms', 'notion'];
	const saveForm = $derived(form?.action === 'save' ? form : null);
	const testForm = $derived(form?.action === 'test' ? form : null);
	const fields: Record<string, string> = $derived(
		(saveForm && 'fields' in saveForm ? saveForm.fields : undefined) ?? {}
	);
	const kept: Record<string, string> = $derived(
		(saveForm && 'values' in saveForm ? saveForm.values : undefined) ?? {}
	);
	const needsConfirmation = $derived(
		!!saveForm && 'needsConfirmation' in saveForm && saveForm.needsConfirmation
	);
	const valueOf = (item: SettingItem) => kept[item.key] ?? item.value ?? '';
	const describedBy = (item: SettingItem) =>
		[
			fields[item.key] ? `${item.key}-error` : null,
			item.unreadable ? `${item.key}-unreadable` : null,
			`${item.key}-meta`
		]
			.filter(Boolean)
			.join(' ');

	let saving = $state(false);
	let testing = $state(false);
	let testChannel = $state<'email' | 'sms'>(
		untrack(() => (form?.action === 'test' && form.channel === 'sms' ? 'sms' : 'email'))
	);
</script>

<svelte:head>
	<title>Ustawienia wysyłki — DoYouSend</title>
</svelte:head>

<p><a href="/admin">← Panel administratora</a></p>
<h1>Ustawienia wysyłki</h1>
<p class="lead">
	Zmiany działają od razu, bez restartu serwera. Wartość z panelu ma pierwszeństwo przed plikiem
	<code>.env</code>; „Przywróć” usuwa ją z panelu. Klucze API są zapisywane zaszyfrowane i nie są
	nigdy wyświetlane.
</p>
<p>
	Adresy, z których wysyłane są e-maile (domeny szkoły i adresy udostępniane pracownikom), ustawisz
	w <a href="/admin/nadawcy">Nadawcach e-maili</a>.
</p>

{#if data.loadFailed || !data.settings}
	<p class="error" role="alert">Nie udało się wczytać ustawień. Odśwież stronę.</p>
{:else}
	{@const settings = data.settings}
	<p class={settings.sendsExternally ? 'alert-warning' : 'alert'} role="status">
		{#if settings.sendsExternally}
			<strong>Wysyłka działa naprawdę.</strong> Zatwierdzone komunikaty trafiają do rodziców i uczniów.
		{:else if settings.dryRun}
			<strong>Tryb testowy.</strong> Nic nie wychodzi poza serwer.
			{#if settings.dryRunLocked}Wyłączenie trybu testowego jest zablokowane na tym serwerze.{/if}
		{:else}
			<strong>Operatorzy lokalni.</strong> Wiadomości trafiają do Mailpita i historii, nie do odbiorców.
		{/if}
	</p>

	<form
		method="POST"
		action="?/save"
		use:enhance={() => {
			saving = true;
			return async ({ update }) => {
				await update({ reset: false });
				saving = false;
			};
		}}
		novalidate
	>
		<div aria-live="polite">
			{#if saveForm && 'done' in saveForm}
				<p class="notice">{saveForm.done}</p>
			{/if}
		</div>
		{#if saveForm && 'failure' in saveForm && saveForm.failure}
			<p class="error" role="alert">{saveForm.failure}</p>
		{/if}

		{#each groups as group (group)}
			<fieldset>
				<legend>{groupLabels[group]}</legend>
				{#each settings.items.filter((i) => i.group === group) as item (item.key)}
					{@const label = labelOf(item.key)}
					{@const locked = item.key === 'dry_run' && settings.dryRunLocked}
					<div class="field">
						<label for={item.key}>{label.label}</label>
						{#if item.kind === 'bool'}
							<select
								id={item.key}
								name={item.key}
								disabled={locked}
								aria-describedby={describedBy(item)}
							>
								<option value="true" selected={valueOf(item) === 'true'}>tak</option>
								<option value="false" selected={valueOf(item) === 'false'}>nie</option>
							</select>
						{:else if item.kind === 'enum'}
							<select id={item.key} name={item.key} aria-describedby={describedBy(item)}>
								{#each item.options ?? [] as option (option)}
									<option value={option} selected={valueOf(item) === option}>{option}</option>
								{/each}
							</select>
						{:else if item.kind === 'secret'}
							<input
								id={item.key}
								name={item.key}
								type="password"
								autocomplete="off"
								aria-describedby={describedBy(item)}
								placeholder={item.set
									? '•••••••• (ustawiony — wpisz, aby zmienić)'
									: 'nie ustawiony'}
							/>
						{:else}
							<input
								id={item.key}
								name={item.key}
								value={valueOf(item)}
								inputmode={item.kind === 'int' || item.kind === 'port'
									? 'numeric'
									: item.kind === 'price'
										? 'decimal'
										: item.kind === 'address'
											? 'email'
											: undefined}
								autocomplete="off"
								aria-invalid={fields[item.key] ? 'true' : undefined}
								aria-describedby={describedBy(item)}
							/>
						{/if}
						{#if fields[item.key]}<span class="field-error" id="{item.key}-error"
								>{fields[item.key]}</span
							>{/if}
						{#if item.unreadable}
							<span class="field-error" id="{item.key}-unreadable"
								>Zapisanej wartości nie da się odczytać (zmienił się klucz szyfrujący) — wpisz ją
								ponownie.</span
							>
						{/if}
						<span class="meta" id="{item.key}-meta">
							{#if label.hint}{label.hint}{/if}
							<span class="source">{sourceLabels[item.source]} · {item.envVar}</span>
							{#if item.source === 'panel' || item.unreadable}
								<label class="reset"
									><input type="checkbox" name="reset:{item.key}" /> Przywróć z .env / domyślne</label
								>
							{/if}
						</span>
					</div>
				{/each}
			</fieldset>
		{/each}

		<fieldset class:highlight={needsConfirmation}>
			<legend>Potwierdzenie prawdziwej wysyłki</legend>
			<label for="confirm">
				Gdy zmiana włącza prawdziwą wysyłkę (tryb testowy wyłączony i prawdziwy operator), wpisz
				<strong>{CONFIRM_PHRASE}</strong>. Wymaga to zgody opiekuna projektu. Wsady wysyłane w tej
				chwili dokończą wysyłkę z nowymi ustawieniami.
			</label>
			<input
				id="confirm"
				name="confirm"
				autocomplete="off"
				aria-describedby={needsConfirmation ? 'confirm-error' : undefined}
			/>
			{#if needsConfirmation}
				<span class="field-error" id="confirm-error"
					>Klucze API wpisane przed chwilą trzeba wpisać ponownie.</span
				>
			{/if}
		</fieldset>

		<button type="submit" class="primary" disabled={saving}>
			{saving ? 'Zapisuję…' : 'Zapisz ustawienia'}
		</button>
	</form>

	<section class="card test" aria-labelledby="test-heading">
		<h2 id="test-heading">Wyślij wiadomość testową</h2>
		<p class="hint">
			Sprawdza zapisane ustawienia jedną wiadomością. Wiadomość nie trafia do historii wysyłek. Przy
			włączonym trybie testowym nic nie wychodzi na zewnątrz.
		</p>
		<form
			method="POST"
			action="?/test"
			use:enhance={() => {
				testing = true;
				return async ({ update }) => {
					await update({ reset: false });
					testing = false;
				};
			}}
			novalidate
		>
			<fieldset class="channels">
				<legend>Kanał</legend>
				<label class="radio"
					><input type="radio" name="channel" value="email" bind:group={testChannel} /> E-mail</label
				>
				<label class="radio"
					><input type="radio" name="channel" value="sms" bind:group={testChannel} /> SMS</label
				>
			</fieldset>
			<div class="field">
				<label for="test-to">
					{testChannel === 'sms' ? 'Numer telefonu' : 'Adres e-mail (opcjonalnie)'}
				</label>
				<input
					id="test-to"
					name="to"
					type={testChannel === 'sms' ? 'tel' : 'email'}
					value={testForm?.to ?? ''}
					autocomplete="off"
					placeholder={testChannel === 'sms' ? '+48 500 100 101' : 'pusty = Twój adres logowania'}
					aria-invalid={testForm && 'failure' in testForm ? 'true' : undefined}
					aria-describedby={testForm && 'failure' in testForm ? 'test-error' : undefined}
				/>
			</div>
			<div aria-live="polite">
				{#if testForm && 'done' in testForm}
					<p class={testForm.dryRun ? 'alert' : 'notice'}>{testForm.done}</p>
				{/if}
			</div>
			{#if testForm && 'failure' in testForm}
				<p class="error" role="alert" id="test-error">{testForm.failure}</p>
			{/if}
			<button type="submit" disabled={testing}>
				{testing ? 'Wysyłam…' : 'Wyślij wiadomość testową'}
			</button>
		</form>
	</section>
{/if}

<style>
	form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		max-width: 42rem;
	}
	fieldset {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
	}
	fieldset.highlight {
		border-color: var(--color-danger);
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 0.75rem;
		font-size: var(--text-sm);
		color: var(--color-text-muted);
	}
	.source {
		font-family: var(--font-mono);
	}
	.reset,
	.radio {
		display: inline-flex;
		gap: 0.3rem;
		align-items: center;
		font-weight: 400;
	}
	.channels {
		flex-direction: row;
		flex-wrap: wrap;
		gap: 1rem;
		background: none;
	}
	button {
		align-self: flex-start;
	}
	.test {
		max-width: 42rem;
		margin-top: var(--space-6);
	}
	.test h2 {
		margin-top: 0;
	}
</style>
