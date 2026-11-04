<script lang="ts">
	import { enhance } from '$app/forms';
	import type { SettingItem } from '$lib/api/admin';
	import type { PageProps } from './$types';
	import { CONFIRM_PHRASE, groupLabels, labelOf, sourceLabels } from './settings';

	let { data, form }: PageProps = $props();

	const groups: SettingItem['group'][] = ['general', 'email', 'sms', 'notion'];
	const fields: Record<string, string> = $derived(
		(form && 'fields' in form ? form.fields : undefined) ?? {}
	);
	const kept: Record<string, string> = $derived(
		(form && 'values' in form ? form.values : undefined) ?? {}
	);
	const needsConfirmation = $derived(
		!!form && 'needsConfirmation' in form && form.needsConfirmation
	);
	const valueOf = (item: SettingItem) => kept[item.key] ?? item.value ?? '';
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

{#if data.loadFailed || !data.settings}
	<p class="error" role="alert">Nie udało się wczytać ustawień. Odśwież stronę.</p>
{:else}
	{@const settings = data.settings}
	<p class={settings.sendsExternally ? 'alert alert-warning' : 'alert'} role="status">
		{#if settings.sendsExternally}
			<strong>Wysyłka działa naprawdę.</strong> Zatwierdzone komunikaty trafiają do rodziców i uczniów.
		{:else if settings.dryRun}
			<strong>Tryb testowy.</strong> Nic nie wychodzi poza serwer.
			{#if settings.dryRunLocked}Wyłączenie trybu testowego jest zablokowane na tym serwerze.{/if}
		{:else}
			<strong>Operatorzy lokalni.</strong> Wiadomości trafiają do Mailpita i historii, nie do odbiorców.
		{/if}
	</p>

	<form method="POST" action="?/save" use:enhance novalidate>
		{#if form && 'done' in form}
			<p class="done" role="status">{form.done}</p>
		{/if}
		{#if form && 'failure' in form && form.failure}
			<p class="error" role="alert">{form.failure}</p>
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
							<select id={item.key} name={item.key} disabled={locked}>
								<option value="true" selected={valueOf(item) === 'true'}>tak</option>
								<option value="false" selected={valueOf(item) === 'false'}>nie</option>
							</select>
						{:else if item.kind === 'enum'}
							<select id={item.key} name={item.key}>
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
								placeholder={item.set
									? '•••••••• (ustawiony — wpisz, aby zmienić)'
									: 'nie ustawiony'}
							/>
						{:else}
							<input
								id={item.key}
								name={item.key}
								value={valueOf(item)}
								inputmode={item.kind === 'price' || item.kind === 'port' ? 'decimal' : undefined}
								autocomplete="off"
								aria-invalid={fields[item.key] ? 'true' : undefined}
							/>
						{/if}
						{#if fields[item.key]}<span class="field-error">{fields[item.key]}</span>{/if}
						{#if item.unreadable}
							<span class="field-error"
								>Zapisanej wartości nie da się odczytać (zmienił się klucz szyfrujący) — wpisz ją
								ponownie.</span
							>
						{/if}
						<span class="meta">
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
				Gdy zmiana włącza prawdziwą wysyłkę (tryb testowy wyłączony i operator sendgrid lub smsapi),
				wpisz <strong>{CONFIRM_PHRASE}</strong>. Wymaga to zgody opiekuna projektu. Wsady wysyłane w
				tej chwili dokończą wysyłkę z nowymi ustawieniami.
			</label>
			<input id="confirm" name="confirm" autocomplete="off" />
			{#if needsConfirmation}
				<span class="field-error">Klucze API wpisane przed chwilą trzeba wpisać ponownie.</span>
			{/if}
		</fieldset>

		<button type="submit" class="primary">Zapisz ustawienia</button>
	</form>
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
		border: 1px solid var(--color-border);
		border-radius: 0.5rem;
		padding: 1rem;
	}
	fieldset.highlight {
		border-color: var(--color-danger);
	}
	legend {
		font-weight: 700;
		padding: 0 0.3rem;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.field > label,
	fieldset > label {
		font-weight: 600;
	}
	.meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 0.75rem;
		font-size: 0.85em;
		color: var(--color-text-muted);
	}
	.source {
		font-family: var(--font-mono, monospace);
	}
	.reset {
		display: inline-flex;
		gap: 0.3rem;
		align-items: center;
		font-weight: 400;
	}
	button {
		align-self: flex-start;
	}
	.done {
		color: var(--color-success);
	}
</style>
