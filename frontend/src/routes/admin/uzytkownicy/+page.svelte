<script lang="ts">
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { roleLabels } from '$lib/access';
	import type { PageProps } from './$types';
	import { dateTimeText } from '../format';
	import { senderStatusLabels, type NewUserValues } from './users';

	let { data, form }: PageProps = $props();

	const createForm = $derived(form?.action === 'create' ? form : null);
	const createValues: NewUserValues = $derived(
		(createForm && 'values' in createForm ? createForm.values : undefined) ?? {
			email: '',
			fullName: '',
			role: 'sender'
		}
	);
	const createFields: Record<string, string> = $derived(
		(createForm && 'fields' in createForm ? createForm.fields : undefined) ?? {}
	);
	const senderOf = (id: string) => data.senders?.items.find((s) => s.userId === id);
	const rowMessage = (id: string) =>
		form && form.action !== 'create' && 'id' in form && form.id === id ? form : null;

	// The form being sent, as "<action>:<user id>"; its button shows progress
	// and every button of that row waits, so nothing is sent twice.
	let pending = $state<string | null>(null);

	/** enhance with a progress state and, for destructive actions, a confirmation. */
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
	const busy = (id: string) => pending !== null && pending.endsWith(`:${id}`);
</script>

<svelte:head>
	<title>Użytkownicy — DoYouSend</title>
</svelte:head>

<p><a href="/admin">← Panel administratora</a></p>
<h1>Użytkownicy</h1>
<p class="lead">
	<strong>Administrator</strong> zarządza odbiorcami, grupami, kontami i konfiguracją.
	<strong>Wysyłający</strong> (dyrektor lub upoważniony pracownik) tworzy i wysyła komunikaty.
</p>
<p>
	Kolumna „Nadawca e-mail” to weryfikacja pojedynczego adresu w SendGrid. Domeny szkoły i adresy
	udostępniane kilku osobom ustawisz w <a href="/admin/nadawcy">Nadawcach e-maili</a>.
</p>

{#if data.senders && !data.senders.live}
	<p class="alert">
		Nadawcy e-mail działają w trybie testowym (DRY_RUN lub operator inny niż SendGrid): zgłoszenie
		niczego nie wysyła, a pierwsze sprawdzenie od razu pokazuje adres jako potwierdzony.
	</p>
{/if}

{#if data.loadFailed}
	<p class="error" role="alert">Nie udało się wczytać listy kont. Odśwież stronę.</p>
{:else}
	<div class="table-scroll">
		<table>
			<thead>
				<tr>
					<th scope="col">Imię i nazwisko</th>
					<th scope="col">E-mail</th>
					<th scope="col">Rola</th>
					<th scope="col">Stan</th>
					<th scope="col">Ostatnie logowanie</th>
					<th scope="col">Nadawca e-mail</th>
					<th scope="col">Akcje</th>
				</tr>
			</thead>
			<tbody>
				{#each data.users as user (user.id)}
					{@const message = rowMessage(user.id)}
					{@const isSelf = user.id === data.user?.id}
					<tr class:disabled={user.disabled}>
						<td>{user.fullName}{isSelf ? ' (Ty)' : ''}</td>
						<td>{user.email}</td>
						<td>
							<form
								method="POST"
								action="?/update"
								use:enhance={guarded(`role:${user.id}`)}
								class="inline"
							>
								<input type="hidden" name="id" value={user.id} />
								<select name="role" aria-label="Rola: {user.fullName}" disabled={isSelf}>
									<option value="sender" selected={user.role === 'sender'}
										>{roleLabels.sender}</option
									>
									<option value="admin" selected={user.role === 'admin'}>{roleLabels.admin}</option>
								</select>
								{#if !isSelf}
									<button type="submit" disabled={busy(user.id)}>
										{pending === `role:${user.id}` ? 'Zapisuję…' : 'Zmień'}
									</button>
								{/if}
							</form>
						</td>
						<td>{user.disabled ? 'zablokowane' : 'aktywne'}</td>
						<td>{dateTimeText(user.lastLoginAt)}</td>
						<td>
							{#if data.senders === null}
								<span class="muted">niedostępne</span>
							{:else}
								{@const sender = senderOf(user.id)}
								<div class="actions">
									{#if sender}
										<span
											class="badge"
											class:success={sender.status === 'verified'}
											class:danger={sender.status === 'missing'}
											>{senderStatusLabels[sender.status]}</span
										>
										<div class="inline">
											{#if sender.status !== 'verified'}
												<form
													method="POST"
													action="?/senderCheck"
													use:enhance={guarded(`senderCheck:${user.id}`)}
													class="inline"
												>
													<input type="hidden" name="id" value={user.id} />
													<button type="submit" disabled={busy(user.id)}>
														{pending === `senderCheck:${user.id}` ? 'Sprawdzam…' : 'Sprawdź'}
													</button>
												</form>
											{/if}
											{#if sender.status === 'pending'}
												<form
													method="POST"
													action="?/senderResend"
													use:enhance={guarded(`senderResend:${user.id}`)}
													class="inline"
												>
													<input type="hidden" name="id" value={user.id} />
													<button type="submit" disabled={busy(user.id)}
														>Wyślij link ponownie</button
													>
												</form>
											{/if}
											<form
												method="POST"
												action="?/senderRemove"
												use:enhance={guarded(
													`senderRemove:${user.id}`,
													`Usunąć nadawcę ${sender.fromEmail}? Żeby znów wysyłać z tego adresu, trzeba go będzie zgłosić i potwierdzić od nowa.`
												)}
												class="inline"
											>
												<input type="hidden" name="id" value={user.id} />
												<button type="submit" class="danger" disabled={busy(user.id)}>Usuń</button>
											</form>
										</div>
									{:else if !user.disabled}
										<form
											method="POST"
											action="?/senderRequest"
											use:enhance={guarded(`senderRequest:${user.id}`)}
											class="inline"
										>
											<input type="hidden" name="id" value={user.id} />
											<button
												type="submit"
												disabled={busy(user.id)}
												title="SendGrid wyśle link potwierdzający na {user.email}"
											>
												{pending === `senderRequest:${user.id}`
													? 'Zgłaszam…'
													: 'Zgłoś do weryfikacji'}
											</button>
										</form>
									{:else}
										<span class="muted">—</span>
									{/if}
								</div>
							{/if}
						</td>
						<td>
							<div class="actions">
								{#if !isSelf}
									<form
										method="POST"
										action="?/update"
										use:enhance={guarded(
											`block:${user.id}`,
											user.disabled
												? undefined
												: `Zablokować konto ${user.fullName}? Ta osoba zostanie wylogowana i nie zaloguje się, dopóki jej nie odblokujesz.`
										)}
										class="inline"
									>
										<input type="hidden" name="id" value={user.id} />
										<input type="hidden" name="disabled" value={user.disabled ? 'false' : 'true'} />
										<button type="submit" class:danger={!user.disabled} disabled={busy(user.id)}>
											{user.disabled ? 'Odblokuj' : 'Zablokuj'}
										</button>
									</form>
								{/if}
								<details>
									<summary>Nowe hasło</summary>
									<form
										method="POST"
										action="?/password"
										use:enhance={guarded(`password:${user.id}`)}
										class="password"
									>
										<input type="hidden" name="id" value={user.id} />
										<label>
											Nowe hasło (min. 12 znaków)
											<input
												name="password"
												type="password"
												autocomplete="new-password"
												minlength="12"
												required
											/>
										</label>
										<button type="submit" disabled={busy(user.id)}>
											{pending === `password:${user.id}` ? 'Zapisuję…' : 'Ustaw hasło'}
										</button>
									</form>
								</details>
								<div aria-live="polite">
									{#if message?.failure}
										<p class="error" role="alert">{message.failure}</p>
									{:else if message && 'done' in message}
										<p class="notice">{message.done}</p>
									{/if}
								</div>
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<section aria-labelledby="new-user">
	<h2 id="new-user">Nowe konto</h2>
	<form
		method="POST"
		action="?/create"
		use:enhance={guarded('create:new')}
		class="create"
		novalidate
	>
		<div aria-live="polite">
			{#if createForm && 'done' in createForm}
				<p class="notice">{createForm.done}</p>
			{/if}
		</div>
		{#if createForm && 'failure' in createForm && createForm.failure}
			<p class="error" role="alert">{createForm.failure}</p>
		{/if}
		<label>
			Imię i nazwisko
			<input
				name="fullName"
				value={createValues.fullName}
				autocomplete="off"
				aria-invalid={createFields.fullName ? 'true' : undefined}
				aria-describedby={createFields.fullName ? 'new-fullName-error' : undefined}
			/>
			{#if createFields.fullName}<span class="field-error" id="new-fullName-error"
					>{createFields.fullName}</span
				>{/if}
		</label>
		<label>
			E-mail (login)
			<input
				name="email"
				type="email"
				value={createValues.email}
				autocomplete="off"
				aria-invalid={createFields.email ? 'true' : undefined}
				aria-describedby={createFields.email ? 'new-email-error' : undefined}
			/>
			{#if createFields.email}<span class="field-error" id="new-email-error"
					>{createFields.email}</span
				>{/if}
		</label>
		<label>
			Rola
			<select name="role">
				<option value="sender" selected={createValues.role !== 'admin'}>{roleLabels.sender}</option>
				<option value="admin" selected={createValues.role === 'admin'}>{roleLabels.admin}</option>
			</select>
		</label>
		<label>
			Hasło startowe (min. 12 znaków)
			<input
				name="password"
				type="password"
				autocomplete="new-password"
				aria-invalid={createFields.password ? 'true' : undefined}
				aria-describedby={createFields.password ? 'new-password-error' : 'new-password-hint'}
			/>
			{#if createFields.password}<span class="field-error" id="new-password-error"
					>{createFields.password}</span
				>{/if}
		</label>
		<p class="hint" id="new-password-hint">
			Przekaż hasło osobiście, nie e-mailem. Po zalogowaniu użytkownik zmieni je sam w „Moim
			koncie”.
		</p>
		<button type="submit" class="primary" disabled={pending === 'create:new'}>
			{pending === 'create:new' ? 'Tworzę…' : 'Utwórz konto'}
		</button>
	</form>
</section>

<style>
	.table-scroll {
		overflow-x: auto;
	}
	tr.disabled td {
		color: var(--color-text-muted);
	}
	.inline {
		display: inline-flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		align-items: center;
	}
	.actions {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.35rem;
		min-width: 12rem;
	}
	.password,
	.create {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		max-width: 26rem;
	}
	.password {
		margin-top: 0.5rem;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.create button,
	.password button {
		align-self: flex-start;
	}
	.muted {
		color: var(--color-text-muted);
	}
	@media (max-width: 48rem) {
		table,
		thead,
		tbody,
		tr,
		th,
		td {
			display: block;
		}
		thead {
			display: none;
		}
		tr {
			border-bottom: 2px solid var(--color-border);
			padding: 0.5rem 0;
		}
		td {
			border: none;
			padding: 0.2rem 0;
		}
	}
</style>
