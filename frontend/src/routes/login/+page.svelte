<script lang="ts">
	import { enhance } from '$app/forms';
	import type { PageProps } from './$types';

	let { form }: PageProps = $props();

	let submitting = $state(false);
</script>

<svelte:head>
	<title>Logowanie — DoYouSend</title>
</svelte:head>

<section>
	<h1>Logowanie</h1>
	<p class="lead">System wysyłania komunikatów szkoły e-mailem i SMS-em.</p>

	<form
		method="POST"
		use:enhance={({ formElement }) => {
			submitting = true;
			return async ({ update }) => {
				// Keep the e-mail after a failed attempt, but never the password.
				await update({ reset: false });
				const password = formElement.elements.namedItem('password');
				if (password instanceof HTMLInputElement) password.value = '';
				submitting = false;
			};
		}}
	>
		{#if form?.failure}
			<div class="error" role="alert"><p>{form.failure}</p></div>
		{/if}

		<label>
			E-mail
			<input name="email" type="email" autocomplete="username" required value={form?.email ?? ''} />
		</label>
		<label>
			Hasło
			<input name="password" type="password" autocomplete="current-password" required />
		</label>
		<button type="submit" class="primary" disabled={submitting}
			>{submitting ? 'Loguję…' : 'Zaloguj'}</button
		>
	</form>

	<p class="hint">
		Nie masz konta albo nie pamiętasz hasła? Poproś administratora — konta zakłada on w panelu
		administratora.
	</p>
</section>

<style>
	section {
		max-width: 26rem;
		margin: var(--space-6) auto;
		padding: var(--space-5) var(--space-6);
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow);
	}
	form {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		margin: 1.5rem 0;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-weight: 600;
	}
	button {
		margin-top: var(--space-2);
	}
</style>
