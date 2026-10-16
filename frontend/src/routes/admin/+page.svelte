<script lang="ts">
	import type { PageProps } from './$types';
	import {
		actionText,
		actorText,
		creditsText,
		dateTimeText,
		LOW_EMAIL_CREDITS_THRESHOLD,
		LOW_POINTS_THRESHOLD,
		plnText,
		pointsText,
		setupChecks
	} from './format';

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

		{#if data.profile}
			<hr class="divider" />
			<div class="profile-box">
				<div class="profile-header">
					<strong
						>Stan konta SMSAPI {#if data.config?.sms.provider === 'fake'}<span class="badge-test"
								>(tryb testowy)</span
							>{/if}</strong
					>
					{#if data.config?.sms.provider !== 'fake' && data.profile.points < LOW_POINTS_THRESHOLD}
						<span class="badge-warn" title="Niski stan punktów SMSAPI — doładuj konto"
							>Niski stan</span
						>
					{/if}
				</div>
				<dl>
					<dt>Dostępne punkty</dt>
					<dd
						class={data.config?.sms.provider !== 'fake' &&
						data.profile.points < LOW_POINTS_THRESHOLD
							? 'points-low'
							: 'points-ok'}
					>
						{pointsText(data.profile.points)}
					</dd>
					<dt>Konto / profil</dt>
					<dd>{data.profile.name} ({data.profile.userType})</dd>
				</dl>
			</div>
		{:else if data.config?.sms.provider === 'smsapi' && !data.config?.dryRun}
			<hr class="divider" />
			<p class="error" role="alert">Nie udało się wczytać stanu konta SMSAPI.</p>
		{/if}
	</section>

	<section class="card" aria-labelledby="email-heading">
		<h2 id="email-heading">E-maile — {data.month.label}</h2>
		{#if data.emailStats}
			<p class="big">{data.emailStats.totalMessages}</p>
			<p class="sub">wysłanych wiadomości e-mail w bieżącym miesiącu</p>
			<dl>
				<dt>Wiadomości e-mail</dt>
				<dd>{data.emailStats.totalMessages}</dd>
				<dt>Doręczone</dt>
				<dd>{data.emailStats.deliveredMessages}</dd>
				<dt>Wysłane</dt>
				<dd>{data.emailStats.sentMessages}</dd>
				<dt>Nieudane</dt>
				<dd>{data.emailStats.failedMessages}</dd>
				<dt>W trakcie</dt>
				<dd>{data.emailStats.inFlightMessages}</dd>
			</dl>
		{:else}
			<p class="error" role="alert">Nie udało się wczytać statystyk e-mail.</p>
		{/if}

		{#if data.emailProfile}
			<hr class="divider" />
			<div class="profile-box">
				<div class="profile-header">
					<strong
						>Stan konta {data.emailProfile.provider === 'sendgrid'
							? 'SendGrid'
							: data.emailProfile.provider}
						{#if data.emailProfile.provider === 'mailpit' || data.config?.email.provider === 'mailpit'}<span
								class="badge-test">(tryb testowy)</span
							>{/if}</strong
					>
					{#if data.emailProfile.remainCredits !== undefined && data.emailProfile.remainCredits < LOW_EMAIL_CREDITS_THRESHOLD}
						<span class="badge-warn" title="Niski limit kredytów SendGrid — sprawdź stan konta"
							>Niski limit</span
						>
					{/if}
				</div>
				<dl>
					{#if data.emailProfile.remainCredits !== undefined}
						<dt>Dostępne kredyty</dt>
						<dd
							class={data.emailProfile.remainCredits < LOW_EMAIL_CREDITS_THRESHOLD
								? 'points-low'
								: 'points-ok'}
						>
							{creditsText(data.emailProfile.remainCredits, data.emailProfile.totalCredits)}
						</dd>
					{/if}
					{#if data.emailProfile.usedCredits !== undefined}
						<dt>Wykorzystane</dt>
						<dd>{data.emailProfile.usedCredits}</dd>
					{/if}
					{#if data.emailProfile.resetFrequency}
						<dt>Odnowienie limitu</dt>
						<dd>
							{data.emailProfile.resetFrequency === 'day'
								? 'Codziennie'
								: data.emailProfile.resetFrequency}
							{#if data.emailProfile.nextReset}
								({data.emailProfile.nextReset})
							{/if}
						</dd>
					{/if}
					{#if data.emailProfile.provider === 'mailpit'}
						<dt>Tryb</dt>
						<dd>Lokalna skrzynka Mailpit (brak limitów)</dd>
					{/if}
				</dl>
			</div>
		{:else if data.config?.email.provider === 'sendgrid' && !data.config?.dryRun}
			<hr class="divider" />
			<p class="error" role="alert">Nie udało się wczytać stanu konta SendGrid.</p>
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
	.divider {
		border: 0;
		border-top: 1px solid #ddd;
		margin: 1rem 0 0.75rem 0;
	}
	.profile-box {
		font-size: 0.95em;
	}
	.profile-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 0.5rem;
	}
	.badge-warn {
		background: #fff3cd;
		color: #8a4b00;
		border: 1px solid #ffeeba;
		border-radius: 4px;
		font-size: 0.75rem;
		font-weight: 600;
		padding: 0.15rem 0.4rem;
	}
	.points-low {
		color: #b00020;
		font-weight: 700;
	}
	.points-ok {
		color: #1a7f37;
		font-weight: 600;
	}
	.badge-test {
		font-size: 0.8em;
		font-weight: normal;
		color: #666;
		margin-left: 0.25rem;
	}
</style>
