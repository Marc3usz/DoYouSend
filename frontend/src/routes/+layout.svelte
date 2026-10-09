<script lang="ts">
	import { page } from '$app/state';
	import { navLinks, roleLabels } from '$lib/access';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	const links = $derived(navLinks(data.user));
	const isCurrent = (href: string) =>
		page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);
</script>

<header>
	<a class="brand" href="/"><strong>DoYouSend</strong></a>
	{#if data.user}
		<nav aria-label="Główna nawigacja">
			{#each links as link (link.href)}
				<a href={link.href} aria-current={isCurrent(link.href) ? 'page' : undefined}>{link.label}</a
				>
			{/each}
		</nav>
		<div class="account">
			<span>
				{data.user.fullName}
				<small>({roleLabels[data.user.role]})</small>
			</span>
			<form method="POST" action="/logout">
				<button type="submit">Wyloguj</button>
			</form>
		</div>
	{:else}
		<span class="tagline">komunikaty e-mail i SMS</span>
	{/if}
</header>

<main>
	{@render children()}
</main>

<style>
	header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 1.5rem;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid #ddd;
	}
	.brand {
		color: inherit;
		text-decoration: none;
	}
	.tagline {
		color: #555;
	}
	nav {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 1rem;
	}
	nav a {
		text-decoration: none;
		padding: 0.25rem 0;
		border-bottom: 2px solid transparent;
	}
	nav a[aria-current='page'] {
		border-bottom-color: currentColor;
		font-weight: 600;
	}
	.account {
		margin-left: auto;
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}
	.account small {
		color: #555;
	}
	.account button {
		font: inherit;
		padding: 0.25rem 0.75rem;
	}
	main {
		padding: 1rem;
		max-width: 60rem;
	}
</style>
