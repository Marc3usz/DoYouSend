<script lang="ts">
	import { enhance } from '$app/forms';
	import { roleLabels } from '$lib/access';
	import type { PageProps } from './$types';
	import { dateTimeText } from '../format';
	import type { NewUserValues } from './users';

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
	const rowMessage = (id: string) =>
		form && form.action !== 'create' && 'id' in form && form.id === id ? form : null;
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

{#if data.loadFailed}
	<p class="error" role="alert">Nie udało się wczytać listy kont. Odśwież stronę.</p>
{:else}
	<table>
		<thead>
			<tr
				><th>Imię i nazwisko</th><th>E-mail</th><th>Rola</th><th>Stan</th><th>Ostatnie logowanie</th
				><th>Akcje</th></tr
			>
		</thead>
		<tbody>
			{#each data.users as user (user.id)}
				{@const message = rowMessage(user.id)}
				{@const isSelf = user.id === data.user?.id}
				<tr class:disabled={user.disabled}>
					<td>{user.fullName}{isSelf ? ' (Ty)' : ''}</td>
					<td>{user.email}</td>
					<td>
						<form method="POST" action="?/update" use:enhance class="inline">
							<input type="hidden" name="id" value={user.id} />
							<select name="role" aria-label="Rola: {user.fullName}" disabled={isSelf}>
								<option value="sender" selected={user.role === 'sender'}>{roleLabels.sender}</option
								>
								<option value="admin" selected={user.role === 'admin'}>{roleLabels.admin}</option>
							</select>
							{#if !isSelf}<button type="submit">Zmień</button>{/if}
						</form>
					</td>
					<td>{user.disabled ? 'zablokowane' : 'aktywne'}</td>
					<td>{dateTimeText(user.lastLoginAt)}</td>
					<td>
						<div class="actions">
							{#if !isSelf}
								<form method="POST" action="?/update" use:enhance class="inline">
									<input type="hidden" name="id" value={user.id} />
									<input type="hidden" name="disabled" value={user.disabled ? 'false' : 'true'} />
									<button type="submit">{user.disabled ? 'Odblokuj' : 'Zablokuj'}</button>
								</form>
							{/if}
							<details>
								<summary>Nowe hasło</summary>
								<form method="POST" action="?/password" use:enhance class="password">
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
									<button type="submit">Ustaw hasło</button>
								</form>
							</details>
							{#if message?.failure}
								<p class="error" role="alert">{message.failure}</p>
							{:else if message && 'done' in message}
								<p class="done" role="status">{message.done}</p>
							{/if}
						</div>
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
{/if}

<section aria-labelledby="new-user">
	<h2 id="new-user">Nowe konto</h2>
	<form method="POST" action="?/create" use:enhance class="create" novalidate>
		{#if createForm && 'done' in createForm}
			<p class="done" role="status">{createForm.done}</p>
		{/if}
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
			/>
			{#if createFields.fullName}<span class="field-error">{createFields.fullName}</span>{/if}
		</label>
		<label>
			E-mail (login)
			<input
				name="email"
				type="email"
				value={createValues.email}
				autocomplete="off"
				aria-invalid={createFields.email ? 'true' : undefined}
			/>
			{#if createFields.email}<span class="field-error">{createFields.email}</span>{/if}
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
			/>
			{#if createFields.password}<span class="field-error">{createFields.password}</span>{/if}
		</label>
		<p class="hint">
			Przekaż hasło osobiście, nie e-mailem. Użytkownik może poprosić o nowe w każdej chwili.
		</p>
		<button type="submit">Utwórz konto</button>
	</form>
</section>

<style>
	.lead,
	.hint {
		color: #555;
	}
	table {
		border-collapse: collapse;
		width: 100%;
		margin-bottom: 2rem;
	}
	th,
	td {
		border-bottom: 1px solid #ddd;
		padding: 0.4rem 0.5rem;
		text-align: left;
		vertical-align: top;
	}
	tr.disabled td {
		color: #777;
	}
	.inline {
		display: inline-flex;
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
		font-weight: 600;
	}
	input:not([type='hidden']),
	select {
		font: inherit;
		font-weight: normal;
		padding: 0.35rem;
	}
	button {
		font: inherit;
		align-self: flex-start;
		padding: 0.3rem 0.8rem;
	}
	.field-error,
	.error {
		color: #b00020;
		font-weight: normal;
	}
	.done {
		color: #1a7f37;
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
			border-bottom: 2px solid #ddd;
			padding: 0.5rem 0;
		}
		td {
			border: none;
			padding: 0.2rem 0;
		}
	}
</style>
