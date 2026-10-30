<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	import { navLinks, roleLabels } from '$lib/access';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	const links = $derived(navLinks(data.user));
	const isCurrent = (href: string) =>
		page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);
</script>

<header>
	<a class="brand" href="/"
		><span class="logo" aria-hidden="true">✉</span><strong>DoYouSend</strong></a
	>
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
		position: sticky;
		top: 0;
		z-index: 10;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2) var(--space-4);
		padding: var(--space-3) max(var(--space-4), calc((100% - var(--content-width)) / 2));
		background: var(--color-surface);
		border-bottom: 1px solid var(--color-border);
		box-shadow: var(--shadow-sm);
	}
	.brand {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--color-text);
		text-decoration: none;
		font-size: var(--text-lg);
	}
	.logo {
		display: inline-grid;
		place-items: center;
		width: 1.9rem;
		height: 1.9rem;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-on-primary);
		font-size: 1rem;
	}
	.tagline {
		color: var(--color-text-muted);
	}
	nav {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
	}
	nav a {
		padding: 0.3rem 0.6rem;
		border-radius: var(--radius);
		color: var(--color-text-muted);
		font-weight: 500;
		text-decoration: none;
	}
	nav a:hover {
		background: var(--color-surface-muted);
		color: var(--color-text);
	}
	nav a[aria-current='page'] {
		background: var(--color-primary-soft);
		color: var(--color-primary);
		font-weight: 600;
	}
	.account {
		margin-left: auto;
		display: flex;
		align-items: center;
		gap: var(--space-3);
		font-size: var(--text-sm);
	}
	.account button {
		min-height: 2rem;
		padding: 0.2rem 0.75rem;
	}
	.account small {
		color: var(--color-text-muted);
	}
	main {
		max-width: var(--content-width);
		margin: 0 auto;
		padding: var(--space-5) var(--space-4) var(--space-6);
	}
	@media (max-width: 72rem) {
		.account small {
			display: none;
		}
	}
	@media (max-width: 40rem) {
		.account {
			margin-left: 0;
			width: 100%;
			justify-content: space-between;
		}
	}
</style>
