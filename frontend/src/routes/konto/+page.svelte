<script lang="ts">
	import { enhance } from '$app/forms';
	import { roleLabels } from '$lib/access';
	import { addressText, senderKindLabels, senderReasonText } from '$lib/format/senders';
	import type { PageProps } from './$types';
	import { MIN_PASSWORD_LENGTH, type PasswordFields } from './account';

	let { data, form }: PageProps = $props();

	const fields: PasswordFields = $derived((form && 'fields' in form ? form.fields : null) ?? {});
	let saving = $state(false);
</script>

<svelte:head>
	<title>Moje konto — DoYouSend</title>
</svelte:head>

<h1>Moje konto</h1>

{#if data.user}
	<dl class="me">
		<dt>Imię i nazwisko</dt>
		<dd>{data.user.fullName}</dd>
		<dt>E-mail (login)</dt>
		<dd>{data.user.email}</dd>
		<dt>Rola</dt>
		<dd>{roleLabels[data.user.role]}</dd>
	</dl>
	<p class="hint">Imię, nazwisko i rolę zmienia administrator.</p>
{/if}

<section class="card" aria-labelledby="password-heading">
	<h2 id="password-heading">Zmiana hasła</h2>
	<form
		method="POST"
		action="?/password"
		use:enhance={() => {
			saving = true;
			return async ({ update }) => {
				// Never keep passwords in the fields, whatever the outcome.
				await update({ reset: true });
				saving = false;
			};
		}}
		novalidate
	>
		<div aria-live="polite">
			{#if form && 'done' in form}<p class="notice">{form.done}</p>{/if}
		</div>
		{#if form && 'failure' in form && form.failure}
			<p class="error" role="alert">{form.failure}</p>
		{/if}

		<label for="currentPassword">Obecne hasło</label>
		<input
			id="currentPassword"
			name="currentPassword"
			type="password"
			autocomplete="current-password"
			aria-invalid={fields.currentPassword ? 'true' : undefined}
			aria-describedby={fields.currentPassword ? 'currentPassword-error' : undefined}
		/>
		{#if fields.currentPassword}
			<span class="field-error" id="currentPassword-error">{fields.currentPassword}</span>
		{/if}

		<label for="newPassword">Nowe hasło</label>
		<input
			id="newPassword"
			name="newPassword"
			type="password"
			autocomplete="new-password"
			minlength={MIN_PASSWORD_LENGTH}
			aria-invalid={fields.newPassword ? 'true' : undefined}
			aria-describedby={fields.newPassword ? 'newPassword-error' : 'newPassword-hint'}
		/>
		{#if fields.newPassword}
			<span class="field-error" id="newPassword-error">{fields.newPassword}</span>
		{:else}
			<span class="hint" id="newPassword-hint"
				>Co najmniej {MIN_PASSWORD_LENGTH} znaków. Najłatwiej zapamiętać kilka słów.</span
			>
		{/if}

		<label for="repeat">Powtórz nowe hasło</label>
		<input
			id="repeat"
			name="repeat"
			type="password"
			autocomplete="new-password"
			aria-invalid={fields.repeat ? 'true' : undefined}
			aria-describedby={fields.repeat ? 'repeat-error' : undefined}
		/>
		{#if fields.repeat}
			<span class="field-error" id="repeat-error">{fields.repeat}</span>
		{/if}

		<button type="submit" class="primary" disabled={saving}>
			{saving ? 'Zmieniam…' : 'Zmień hasło'}
		</button>
	</form>
</section>

<section class="card" aria-labelledby="senders-heading">
	<h2 id="senders-heading">Adresy, z których możesz wysyłać e-maile</h2>
	{#if data.senders === null}
		<p class="error" role="alert">Nie udało się wczytać adresów. Odśwież stronę.</p>
	{:else if data.senders.items.length === 0}
		<p>Nie masz jeszcze żadnego adresu nadawcy. Poproś administratora.</p>
	{:else}
		<ul class="senders">
			{#each data.senders.items as option (option.id)}
				<li>
					<strong>{addressText(option)}</strong>
					<span class="hint">{senderKindLabels[option.kind]}</span>
					{#if option.id === data.senders.defaultId}<span class="badge">domyślny</span>{/if}
					{#if option.usable}
						<span class="badge success">gotowy</span>
					{:else}
						<span class="badge warning">{senderReasonText(option.reason)}</span>
					{/if}
				</li>
			{/each}
		</ul>
		<p class="hint">
			Adres wybierasz przy każdej wiadomości w polu „Nadawca e-maila”. Adresy, których nie da się
			jeszcze użyć, przygotowuje administrator.
		</p>
	{/if}
</section>

<style>
	.me {
		display: grid;
		grid-template-columns: max-content auto;
		gap: 0.3rem 1rem;
	}
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
	.card {
		max-width: 36rem;
	}
	.card h2 {
		margin-top: 0;
	}
	form {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}
	label {
		margin-top: 0.5rem;
	}
	button {
		align-self: flex-start;
		margin-top: 0.75rem;
	}
	.senders {
		list-style: none;
		padding: 0;
		margin: 0 0 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.senders li {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.25rem 0.6rem;
		overflow-wrap: anywhere;
	}
</style>
