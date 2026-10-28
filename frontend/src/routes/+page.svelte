<script lang="ts">
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>DoYouSend</title>
</svelte:head>

<h1>Dzień dobry{data.user ? `, ${data.user.fullName}` : ''}</h1>
<p class="lead">
	Jedna treść trafia do wybranych osób jednocześnie e-mailem i SMS-em — w obu kanałach dokładnie
	taka sama.
</p>

<ul class="tiles">
	<li>
		<a href="/wiadomosci">
			<strong>Nowa wiadomość</strong>
			<span>Napisz komunikat, wybierz grupy i sprawdź liczbę SMS-ów oraz koszt.</span>
		</a>
	</li>
	{#if data.user?.role === 'admin'}
		<li>
			<a href="/odbiorcy">
				<strong>Odbiorcy</strong>
				<span>Rodzice i uczniowie, ich e-maile i telefony, import z pliku.</span>
				{#if data.counts}
					<small>
						W bazie: {data.counts.total}.
						{#if data.counts.noEmail + data.counts.noPhone > 0}
							<em>Bez e-maila: {data.counts.noEmail}, bez telefonu: {data.counts.noPhone}.</em>
						{:else}
							Wszyscy mają komplet danych.
						{/if}
					</small>
				{/if}
			</a>
		</li>
		<li>
			<a href="/grupy">
				<strong>Grupy</strong>
				<span>Grupy systemowe i własne do masowej wysyłki.</span>
			</a>
		</li>
		<li>
			<a href="/admin">
				<strong>Panel administratora</strong>
				<span>Użytkownicy, konfiguracja wysyłki, statystyki e-mail i SMS, dziennik zdarzeń.</span>
			</a>
		</li>
	{/if}
</ul>

<style>
	.lead {
		color: var(--color-text-muted);
		max-width: 42rem;
	}
	.tiles {
		list-style: none;
		padding: 0;
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr));
		gap: 1rem;
	}
	.tiles a {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		height: 100%;
		box-sizing: border-box;
		padding: 1rem 1.25rem;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-sm);
		color: inherit;
		text-decoration: none;
		transition:
			border-color 0.15s,
			box-shadow 0.15s;
	}
	.tiles a:hover,
	.tiles a:focus-visible {
		border-color: var(--color-primary);
		box-shadow: var(--shadow);
	}
	.tiles span {
		color: var(--color-text-muted);
	}
	.tiles small {
		color: var(--color-text-muted);
	}
	.tiles em {
		font-style: normal;
		color: var(--color-warning);
	}
</style>
