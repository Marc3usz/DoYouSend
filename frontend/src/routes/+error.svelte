<script lang="ts">
	import { page } from '$app/state';

	const titles: Record<number, string> = {
		403: 'Brak dostępu',
		404: 'Nie ma takiej strony',
		500: 'Coś poszło nie tak',
		502: 'Serwer nie odpowiada',
		503: 'Serwer nie odpowiada'
	};

	const status = $derived(page.status);
	const title = $derived(titles[status] ?? (status >= 500 ? 'Coś poszło nie tak' : 'Błąd'));
	// Our own load functions throw Polish messages; SvelteKit's defaults are English.
	const message = $derived(
		status === 404 && page.error?.message === 'Not Found'
			? 'Adres jest błędny albo strona została usunięta.'
			: status >= 500 && (!page.error?.message || page.error.message === 'Internal Error')
				? 'Spróbuj odświeżyć stronę za chwilę.'
				: (page.error?.message ?? '')
	);
</script>

<svelte:head>
	<title>{title} — DoYouSend</title>
</svelte:head>

<section class="card">
	<p class="code">Błąd {status}</p>
	<h1>{title}</h1>
	{#if message}<p>{message}</p>{/if}
	<p class="links">
		<a href="/" class="button primary">Strona główna</a>
		<button type="button" onclick={() => history.back()}>Wróć</button>
		{#if status >= 500}
			<button type="button" onclick={() => location.reload()}>Odśwież</button>
		{/if}
	</p>
</section>

<style>
	section {
		max-width: 36rem;
		margin: var(--space-6) auto;
	}
	.code {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		margin: 0;
	}
	h1 {
		margin-top: var(--space-1);
	}
	.links {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin: var(--space-4) 0 0;
	}
</style>
