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
		<button type="submit" disabled={submitting}>{submitting ? 'Loguję…' : 'Zaloguj'}</button>
	</form>

	<p class="hint">
		Nie masz konta albo nie pamiętasz hasła? Poproś administratora — konta zakłada on w panelu
		administratora.
	</p>
</section>

<style>
	section {
		max-width: 24rem;
		margin: 2rem auto;
	}
	.lead,
	.hint {
		color: #555;
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
	input {
		font: inherit;
		font-weight: normal;
		padding: 0.5rem;
		border: 1px solid #bbb;
		border-radius: 4px;
	}
	button {
		font: inherit;
		padding: 0.5rem 1rem;
		margin-top: 0.5rem;
	}
	.error {
		border: 1px solid #c00;
		background: #fff4f4;
		padding: 0 1rem;
	}
</style>
