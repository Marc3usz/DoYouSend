<script lang="ts">
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { senderReasonText } from '$lib/format/senders';
	import { dateTimeText } from '$lib/format/text';
	import type { PageProps } from './$types';
	import { dnsTypeLabels, domainStatusLabels, usersText } from './senders';

	let { data, form }: PageProps = $props();

	const domainAdd = $derived(form?.action === 'domainAdd' ? form : null);
	const addressAdd = $derived(form?.action === 'addressAdd' ? form : null);
	const domainRow = (id: string) =>
		form?.action === 'domain' && 'id' in form && form.id === id ? form : null;
	const addressRow = (id: string) =>
		form?.action === 'address' && 'id' in form && form.id === id ? form : null;
	const removed = $derived(
		form?.action === 'domainRemoved' || form?.action === 'addressRemoved' ? form : null
	);
	const addFields = (f: typeof domainAdd | typeof addressAdd): Record<string, string> =>
		(f && 'fields' in f ? f.fields : undefined) ?? {};
	const keptUserIds = $derived(
		new Set(addressAdd && 'userIds' in addressAdd ? addressAdd.userIds : [])
	);

	// The form being sent; its buttons wait, so nothing goes twice.
	let pending = $state<string | null>(null);

	function guarded(key: string, question?: string): SubmitFunction {
		return ({ cancel }) => {
			if (question && !confirm(question)) {
				cancel();
				return;
			}
			pending = key;
			return async ({ update }) => {
				await update();
				pending = null;
			};
		};
	}

	let copied = $state<string | null>(null);
	async function copy(value: string, key: string) {
		try {
			await navigator.clipboard.writeText(value);
			copied = key;
			setTimeout(() => {
				if (copied === key) copied = null;
			}, 2000);
		} catch {
			// No clipboard (plain HTTP): the value stays selectable in the table.
		}
	}
</script>

<svelte:head>
	<title>Nadawcy e-maili — DoYouSend</title>
</svelte:head>

<p><a href="/admin">← Panel administratora</a></p>
<h1>Nadawcy e-maili</h1>
<p class="lead">
	Skąd wychodzą e-maile szkoły. Wspólny adres ustawiasz w
	<a href="/admin/ustawienia">Ustawieniach wysyłki</a>; tutaj — domeny szkoły i adresy, które
	udostępniasz wybranym pracownikom.
</p>

<div aria-live="polite">
	{#if removed}
		<p class="notice">{removed.done}</p>
	{/if}
</div>

<section aria-labelledby="domains-heading">
	<h2 id="domains-heading">Domeny</h2>
	<p>
		Gdy domena szkoły jest uwierzytelniona w SendGrid, każdy użytkownik, którego adres logowania
		jest w tej domenie, automatycznie wysyła e-maile ze swojego adresu — bez osobnej weryfikacji.
	</p>
	{#if data.domains === null}
		<p class="error" role="alert">Nie udało się wczytać domen. Odśwież stronę.</p>
	{:else}
		{#if !data.domains.live}
			<p class="alert">
				Tryb testowy: rekordy DNS poniżej są symulowane i nic nie jest wysyłane do SendGrid. Domena
				zostanie uznana za uwierzytelnioną przy pierwszym sprawdzeniu.
			</p>
		{/if}

		{#each data.domains.items as domain (domain.id)}
			{@const row = domainRow(domain.id)}
			<article class="card" aria-labelledby="domain-{domain.id}">
				<header class="row-head">
					<h3 id="domain-{domain.id}">{domain.domain}</h3>
					<span
						class="badge"
						class:success={domain.status === 'valid'}
						class:warning={domain.status === 'pending'}
						class:danger={domain.status === 'missing'}>{domainStatusLabels[domain.status]}</span
					>
				</header>
				<p class="hint">
					Dodano {dateTimeText(domain.createdAt)} · ostatnie sprawdzenie {dateTimeText(
						domain.checkedAt
					)}{#if domain.validatedAt}
						· uwierzytelniona {dateTimeText(domain.validatedAt)}{/if}
				</p>
				{#if domain.status !== 'valid'}
					<p>
						Dodaj poniższe rekordy u dostawcy DNS domeny <strong>{domain.domain}</strong> (tam, gdzie
						kupiono domenę albo gdzie zarządza nią informatyk szkoły), a potem naciśnij „Sprawdź DNS”.
						Zmiany w DNS potrafią być widoczne dopiero po kilku godzinach.
					</p>
				{/if}
				{#if domain.dnsRecords.length > 0}
					<div class="table-scroll">
						<table>
							<caption class="visually-hidden">Rekordy DNS domeny {domain.domain}</caption>
							<thead>
								<tr>
									<th scope="col">Typ</th>
									<th scope="col">Nazwa (host)</th>
									<th scope="col">Wartość</th>
									<th scope="col">W DNS</th>
								</tr>
							</thead>
							<tbody>
								{#each domain.dnsRecords as record, i (`${record.type}:${record.host}`)}
									{@const key = `${domain.id}:${i}`}
									<tr>
										<td>{dnsTypeLabels[record.type]}</td>
										<td>
											<code class="dns">{record.host}</code>
											<button
												type="button"
												class="link"
												onclick={() => copy(record.host, `${key}:host`)}
												aria-label="Kopiuj nazwę {record.host}"
												>{copied === `${key}:host` ? 'skopiowano' : 'kopiuj'}</button
											>
										</td>
										<td>
											<code class="dns">{record.data}</code>
											<button
												type="button"
												class="link"
												onclick={() => copy(record.data, `${key}:data`)}
												aria-label="Kopiuj wartość rekordu {record.host}"
												>{copied === `${key}:data` ? 'skopiowano' : 'kopiuj'}</button
											>
										</td>
										<td>
											{#if record.valid}
												<span class="ok" aria-hidden="true">✓</span><span class="visually-hidden"
													>widoczny</span
												>
											{:else}
												<span class="bad" aria-hidden="true">✗</span><span class="visually-hidden"
													>jeszcze niewidoczny</span
												>
											{/if}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
				<div class="actions">
					<form
						method="POST"
						action="?/domainValidate"
						use:enhance={guarded(`validate:${domain.id}`)}
					>
						<input type="hidden" name="id" value={domain.id} />
						<button type="submit" class="primary" disabled={pending !== null}>
							{pending === `validate:${domain.id}` ? 'Sprawdzam…' : 'Sprawdź DNS'}
						</button>
					</form>
					<form
						method="POST"
						action="?/domainRemove"
						use:enhance={guarded(
							`domainRemove:${domain.id}`,
							`Usunąć domenę ${domain.domain}? Jej użytkownicy przestaną wysyłać ze swoich adresów i wrócą do wspólnego adresu szkoły.`
						)}
					>
						<input type="hidden" name="id" value={domain.id} />
						<button type="submit" class="danger" disabled={pending !== null}>Usuń domenę</button>
					</form>
				</div>
				<div aria-live="polite">
					{#if row && 'done' in row}<p class="notice">{row.done}</p>{/if}
				</div>
				{#if row && 'failure' in row && row.failure}
					<p class="error" role="alert">{row.failure}</p>
				{/if}
			</article>
		{:else}
			<p class="hint">Nie dodano jeszcze żadnej domeny.</p>
		{/each}

		<form
			method="POST"
			action="?/domainAdd"
			use:enhance={guarded('domainAdd')}
			class="add"
			novalidate
		>
			<h3>Dodaj domenę</h3>
			<div aria-live="polite">
				{#if domainAdd && 'done' in domainAdd}<p class="notice">{domainAdd.done}</p>{/if}
			</div>
			{#if domainAdd && 'failure' in domainAdd && domainAdd.failure}
				<p class="error" role="alert">{domainAdd.failure}</p>
			{/if}
			<label for="new-domain">Domena</label>
			<input
				id="new-domain"
				name="domain"
				placeholder="szkola.example.test"
				autocomplete="off"
				value={domainAdd && 'domain' in domainAdd ? domainAdd.domain : ''}
				aria-invalid={addFields(domainAdd).domain ? 'true' : undefined}
				aria-describedby={addFields(domainAdd).domain ? 'new-domain-error' : 'new-domain-hint'}
			/>
			{#if addFields(domainAdd).domain}
				<span class="field-error" id="new-domain-error">{addFields(domainAdd).domain}</span>
			{/if}
			<p class="hint" id="new-domain-hint">
				Sama domena, bez „@” i „https://”. SendGrid przygotuje rekordy DNS do dodania.
			</p>
			<button type="submit" disabled={pending !== null}>
				{pending === 'domainAdd' ? 'Dodaję…' : 'Dodaj domenę'}
			</button>
		</form>
	{/if}
</section>

<section aria-labelledby="addresses-heading">
	<h2 id="addresses-heading">Adresy do delegowania</h2>
	<p>
		Wspólne adresy (np. sekretariatu), z których mogą wysyłać wybrane osoby. Kto ma dostęp, zobaczy
		adres na liście „Nadawca e-maila” w kreatorze wiadomości.
	</p>
	{#if data.addresses === null}
		<p class="error" role="alert">Nie udało się wczytać adresów. Odśwież stronę.</p>
	{:else}
		{#each data.addresses as address (address.id)}
			{@const row = addressRow(address.id)}
			{@const delegated = new Set(address.users.map((u) => u.id))}
			<article class="card" aria-labelledby="address-{address.id}">
				<header class="row-head">
					<h3 id="address-{address.id}">{address.email}</h3>
					{#if address.usable}
						<span class="badge success">gotowy do wysyłki</span>
					{:else}
						<span class="badge warning">{senderReasonText(address.reason)}</span>
					{/if}
				</header>
				<p class="hint">Mogą wysyłać: {usersText(address.users)}</p>
				<form
					method="POST"
					action="?/addressUpdate"
					use:enhance={guarded(`addressUpdate:${address.id}`)}
					class="delegate"
				>
					<input type="hidden" name="id" value={address.id} />
					<label for="name-{address.id}">Nazwa nadawcy</label>
					<input id="name-{address.id}" name="name" value={address.name} autocomplete="off" />
					{#if data.users}
						<fieldset>
							<legend>Kto może wysyłać z tego adresu</legend>
							{#each data.users as user (user.id)}
								<label class="check">
									<input
										type="checkbox"
										name="userIds"
										value={user.id}
										checked={delegated.has(user.id)}
									/>
									{user.fullName} <small class="muted">{user.email}</small>
								</label>
							{:else}
								<p class="hint">Brak aktywnych kont.</p>
							{/each}
						</fieldset>
					{:else}
						<p class="error" role="alert">
							Nie udało się wczytać listy kont, więc dostępu nie da się teraz zmienić.
						</p>
					{/if}
					<div class="actions">
						<button type="submit" class="primary" disabled={pending !== null || !data.users}>
							{pending === `addressUpdate:${address.id}` ? 'Zapisuję…' : 'Zapisz'}
						</button>
					</div>
				</form>
				<form
					method="POST"
					action="?/addressRemove"
					use:enhance={guarded(
						`addressRemove:${address.id}`,
						`Usunąć adres ${address.email}? Osoby, które z niego wysyłały, wrócą do wspólnego adresu szkoły.`
					)}
				>
					<input type="hidden" name="id" value={address.id} />
					<button type="submit" class="danger" disabled={pending !== null}>Usuń adres</button>
				</form>
				<div aria-live="polite">
					{#if row && 'done' in row}<p class="notice">{row.done}</p>{/if}
				</div>
				{#if row && 'failure' in row && row.failure}
					<p class="error" role="alert">{row.failure}</p>
				{/if}
			</article>
		{:else}
			<p class="hint">Nie dodano jeszcze żadnego adresu.</p>
		{/each}

		<form
			method="POST"
			action="?/addressAdd"
			use:enhance={guarded('addressAdd')}
			class="add"
			novalidate
		>
			<h3>Dodaj adres</h3>
			<div aria-live="polite">
				{#if addressAdd && 'done' in addressAdd}<p class="notice">{addressAdd.done}</p>{/if}
			</div>
			{#if addressAdd && 'failure' in addressAdd && addressAdd.failure}
				<p class="error" role="alert">{addressAdd.failure}</p>
			{/if}
			<label for="new-address-email">Adres e-mail</label>
			<input
				id="new-address-email"
				name="email"
				type="email"
				placeholder="sekretariat@example.test"
				autocomplete="off"
				value={addressAdd && 'values' in addressAdd ? (addressAdd.values?.email ?? '') : ''}
				aria-invalid={addFields(addressAdd).email ? 'true' : undefined}
				aria-describedby={addFields(addressAdd).email ? 'new-address-email-error' : undefined}
			/>
			{#if addFields(addressAdd).email}
				<span class="field-error" id="new-address-email-error">{addFields(addressAdd).email}</span>
			{/if}
			<label for="new-address-name">Nazwa nadawcy</label>
			<input
				id="new-address-name"
				name="name"
				placeholder="Sekretariat szkoły"
				autocomplete="off"
				value={addressAdd && 'values' in addressAdd ? (addressAdd.values?.name ?? '') : ''}
				aria-invalid={addFields(addressAdd).name ? 'true' : undefined}
				aria-describedby={addFields(addressAdd).name ? 'new-address-name-error' : undefined}
			/>
			{#if addFields(addressAdd).name}
				<span class="field-error" id="new-address-name-error">{addFields(addressAdd).name}</span>
			{/if}
			{#if data.users && data.users.length > 0}
				<fieldset>
					<legend>Kto może wysyłać z tego adresu (można zmienić później)</legend>
					{#each data.users as user (user.id)}
						<label class="check">
							<input
								type="checkbox"
								name="userIds"
								value={user.id}
								checked={keptUserIds.has(user.id)}
							/>
							{user.fullName} <small class="muted">{user.email}</small>
						</label>
					{/each}
				</fieldset>
			{/if}
			{#if addFields(addressAdd).userIds}
				<p class="field-error">{addFields(addressAdd).userIds}</p>
			{/if}
			<p class="hint">
				Adres musi być w uwierzytelnionej domenie albo zweryfikowany w SendGrid, zanim da się z
				niego wysyłać.
			</p>
			<button type="submit" disabled={pending !== null}>
				{pending === 'addressAdd' ? 'Dodaję…' : 'Dodaj adres'}
			</button>
		</form>
	{/if}
</section>

<section aria-labelledby="own-heading">
	<h2 id="own-heading">Własne adresy pracowników</h2>
	<p>
		Pojedynczy adres pracownika spoza uwierzytelnionej domeny zgłaszasz do weryfikacji w kolumnie
		„Nadawca e-mail” na stronie <a href="/admin/uzytkownicy">Użytkownicy</a>.
	</p>
</section>

<style>
	section {
		margin-bottom: var(--space-6);
	}
	.row-head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 1rem;
	}
	.row-head h3 {
		margin: 0;
		overflow-wrap: anywhere;
	}
	.table-scroll {
		overflow-x: auto;
	}
	.dns {
		overflow-wrap: anywhere;
		word-break: break-all;
	}
	.ok {
		color: var(--color-success);
		font-weight: 700;
	}
	.bad {
		color: var(--color-danger);
		font-weight: 700;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin: 0.5rem 0;
	}
	.add,
	.delegate {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		max-width: 32rem;
	}
	.add {
		margin-top: var(--space-4);
	}
	.add h3 {
		margin-top: 0;
	}
	.add button {
		align-self: flex-start;
	}
	/* Fieldsets default to their content's width; long e-mails must wrap on phones. */
	fieldset {
		min-width: 0;
	}
	.check {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.5rem;
		overflow-wrap: anywhere;
		font-weight: normal;
		padding: 0.15rem 0;
	}
	.muted {
		color: var(--color-text-muted);
	}
</style>
