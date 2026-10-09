<script lang="ts">
	import type { PageProps } from './$types';
	import { actionText, actorText, dateTimeText, plnText, setupChecks } from './format';

	let { data }: PageProps = $props();

	const activeUsers = $derived(data.users?.filter((u) => !u.disabled) ?? []);
	const admins = $derived(activeUsers.filter((u) => u.role === 'admin').length);
	const checks = $derived(data.config ? setupChecks(data.config) : []);
</script>

<svelte:head>
	<title>Panel administratora — DoYouSend</title>
</svelte:head>

<h1>Panel administratora</h1>

<div class="grid">
	<section class="card" aria-labelledby="sms-heading">
		<h2 id="sms-heading">SMS-y — {data.month.label}</h2>
		{#if data.usage}
			<p class="big">{plnText(data.usage.billedCostMilli)}</p>
			<p class="sub">koszt wysłanych SMS-ów (naliczany przez operatora)</p>
			<dl>
				<dt>Wiadomości SMS</dt>
				<dd>{data.usage.totalMessages}</dd>
				<dt>Części SMS</dt>
				<dd>{data.usage.totalParts}</dd>
				<dt>Doręczone</dt>
				<dd>{data.usage.deliveredMessages}</dd>
				<dt>Nieudane</dt>
				<dd>{data.usage.failedMessages}</dd>
				<dt>W trakcie</dt>
				<dd>{data.usage.inFlightMessages}</dd>
				<dt>Cena za część</dt>
				<dd>{plnText(data.usage.pricePerPartMilli)}</dd>
			</dl>
		{:else}
			<p class="error" role="alert">Nie udało się wczytać statystyk SMS.</p>
		{/if}
	</section>

	<section class="card" aria-labelledby="setup-heading">
		<h2 id="setup-heading">Konfiguracja wysyłki</h2>
		{#if data.config}
			<ul class="checks">
				{#each checks as check (check.text)}
					<li class={check.ok ? 'ok' : 'warn'}>
						<span aria-hidden="true">{check.ok ? '✓' : '!'}</span>
						{check.text}
					</li>
				{/each}
			</ul>
			<dl>
				<dt>Nadawca e-mail</dt>
				<dd>{data.config.email.from}</dd>
				<dt>Nadawca SMS</dt>
				<dd>{data.config.sms.senderName}</dd>
			</dl>
			<p class="sub">
				Ustawienia operatorów zmienia się w pliku <code>.env</code> na serwerze. Podłączenie prawdziwej
				bramki wymaga zgody opiekuna projektu.
			</p>
		{:else}
			<p class="error" role="alert">Nie udało się wczytać konfiguracji.</p>
		{/if}
	</section>

	<section class="card" aria-labelledby="users-heading">
		<h2 id="users-heading">Użytkownicy</h2>
		{#if data.users}
			<p class="big">{activeUsers.length}</p>
			<p class="sub">
				aktywnych kont, w tym {admins}
				{admins === 1 ? 'administrator' : 'administratorów'}
			</p>
			<p><a href="/admin/uzytkownicy">Zarządzaj użytkownikami →</a></p>
		{:else}
			<p class="error" role="alert">Nie udało się wczytać listy użytkowników.</p>
		{/if}
	</section>

	<section class="card" aria-labelledby="data-heading">
		<h2 id="data-heading">Dane odbiorców</h2>
		<ul class="links">
			<li><a href="/odbiorcy">Odbiorcy</a> — lista, dodawanie i poprawianie danych</li>
			<li><a href="/odbiorcy/import">Import z pliku</a> — CSV lub XLSX</li>
			<li><a href="/grupy">Grupy</a> — grupy własne i systemowe</li>
		</ul>
	</section>
</div>

<section aria-labelledby="audit-heading">
	<h2 id="audit-heading">Ostatnie zdarzenia</h2>
	{#if data.audit && data.audit.items.length > 0}
		<table>
			<thead><tr><th>Kiedy</th><th>Kto</th><th>Co</th></tr></thead>
			<tbody>
				{#each data.audit.items as entry (entry.id)}
					<tr>
						<td>{dateTimeText(entry.createdAt)}</td>
						<td>{actorText(entry)}</td>
						<td>{actionText(entry.action)}</td>
					</tr>
				{/each}
			</tbody>
		</table>
		<p><a href="/admin/dziennik">Cały dziennik zdarzeń →</a></p>
	{:else if data.audit}
		<p>Brak zdarzeń.</p>
	{:else}
		<p class="error" role="alert">Nie udało się wczytać dziennika.</p>
	{/if}
</section>

<style>
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(17rem, 1fr));
		gap: 1rem;
		margin-bottom: 2rem;
	}
	.card {
		border: 1px solid #ddd;
		border-radius: 6px;
		padding: 1rem 1.25rem;
		background: #fafafa;
	}
	.card h2 {
		margin-top: 0;
		font-size: 1.1rem;
	}
	.big {
		font-size: 2rem;
		font-weight: 700;
		margin: 0;
		font-variant-numeric: tabular-nums;
	}
	.sub {
		color: #555;
		font-size: 0.9em;
		margin-top: 0.25rem;
	}
	dl {
		display: grid;
		grid-template-columns: auto auto;
		gap: 0.25rem 1rem;
	}
	dd {
		margin: 0;
		text-align: right;
		font-variant-numeric: tabular-nums;
		overflow-wrap: anywhere;
	}
	.checks,
	.links {
		padding-left: 0;
		list-style: none;
	}
	.checks li {
		margin: 0.35rem 0;
		display: flex;
		gap: 0.5rem;
	}
	.checks .ok span {
		color: #1a7f37;
	}
	.checks .warn {
		color: #8a4b00;
	}
	.links li {
		margin: 0.4rem 0;
	}
	table {
		border-collapse: collapse;
		width: 100%;
	}
	th,
	td {
		border-bottom: 1px solid #ddd;
		padding: 0.35rem 0.5rem;
		text-align: left;
	}
	.error {
		color: #b00020;
	}
</style>
