<script lang="ts">
	import { enhance } from '$app/forms';
	import { page } from '$app/state';
	import type { PageProps } from './$types';
	import GroupForm from '../GroupForm.svelte';
	import { kindLabel, peopleCount } from '../messages';
	import { issueHint, issueText, recipientTypeLabel } from '../../odbiorcy/messages';

	let { data, form }: PageProps = $props();

	const g = $derived(data.group);
	const custom = $derived(g.kind === 'custom');
	const created = $derived(!form && page.url.searchParams.has('utworzono'));

	function confirmDelete(): boolean {
		return confirm(
			`Usunąć grupę „${g.name}”? Jej członkowie (${peopleCount(g.memberCount)}) zostaną w bazie odbiorców.`
		);
	}
</script>

<svelte:head>
	<title>{g.name} — DoYouSend</title>
</svelte:head>

<p><a href="/grupy">← Grupy</a></p>
<h1>{g.name}</h1>
<p class="meta">Grupa {kindLabel(g.kind)}, {peopleCount(g.memberCount)}</p>
{#if g.description}<p>{g.description}</p>{/if}

{#if created}
	<p class="notice" role="status">Grupa została utworzona. Dodaj do niej osoby poniżej.</p>
{:else if form?.saved}
	<p class="notice" role="status">Zmiany zostały zapisane.</p>
{:else if form?.added !== undefined}
	<p class="notice" role="status">Dodano: {peopleCount(form.added)}.</p>
{:else if form?.removed !== undefined}
	<p class="notice" role="status">Usunięto z grupy: {peopleCount(form.removed)}.</p>
{/if}

{#if !custom}
	<p class="note">
		Skład grupy systemowej zmienia się sam, gdy dodajesz lub edytujesz odbiorców — nie da się go
		zmienić ręcznie.
	</p>
{/if}

{#if form?.memberFailure}
	<div class="error" role="alert"><p>{form.memberFailure}</p></div>
{/if}

<h2>Członkowie</h2>
{#if data.members.length === 0}
	<p>Grupa nie ma jeszcze członków.</p>
{:else}
	<table>
		<thead>
			<tr>
				<th>Nazwisko i imię</th>
				<th>Typ</th>
				<th>Uwagi</th>
				{#if custom}<th><span class="visually-hidden">Akcje</span></th>{/if}
			</tr>
		</thead>
		<tbody>
			{#each data.members as r (r.id)}
				<tr>
					<td><a href="/odbiorcy/{r.id}">{r.lastName} {r.firstName}</a></td>
					<td>{recipientTypeLabel(r.type)}</td>
					<td>
						{#each r.issues as issue (issue.channel)}
							<span class="issue" title={issueHint(issue)}>{issueText(issue)}</span>
						{/each}
					</td>
					{#if custom}
						<td>
							<form method="POST" action="?/remove" use:enhance>
								<input type="hidden" name="recipientId" value={r.id} />
								<button type="submit" class="link">Usuń z grupy</button>
							</form>
						</td>
					{/if}
				</tr>
			{/each}
		</tbody>
	</table>
{/if}

{#if custom}
	<h2>Dodaj osoby</h2>
	<!-- A GET form: the search lands in the URL and works without JavaScript. -->
	<form method="GET" class="search">
		<input
			type="search"
			name="q"
			value={data.q}
			maxlength="100"
			placeholder="imię, nazwisko, e-mail lub telefon"
			aria-label="Szukaj odbiorców"
		/>
		<button type="submit">Szukaj</button>
	</form>

	{#if data.q !== ''}
		{#if data.candidates.length === 0}
			<p>Nie znaleziono nikogo spoza grupy dla „{data.q}”.</p>
		{:else}
			<form method="POST" action="?/add" use:enhance>
				<ul class="candidates">
					{#each data.candidates as r (r.id)}
						<li>
							<label>
								<input type="checkbox" name="recipientId" value={r.id} />
								{r.lastName}
								{r.firstName} ({recipientTypeLabel(r.type)})
							</label>
						</li>
					{/each}
				</ul>
				{#if data.moreCandidates}
					<p class="note">Pokazano pierwsze wyniki — zawęź wyszukiwanie, jeśli brakuje osoby.</p>
				{/if}
				<button type="submit">Dodaj zaznaczone</button>
			</form>
		{/if}
	{/if}

	<h2>Nazwa i opis</h2>
	<GroupForm
		action="?/save"
		values={form?.values ?? { name: g.name, description: g.description }}
		errors={form?.errors}
		failure={form?.failure}
		submitLabel="Zapisz zmiany"
	/>

	<h2>Usuwanie</h2>
	{#if form?.deleteFailure}
		<div class="error" role="alert"><p>{form.deleteFailure}</p></div>
	{/if}
	<form
		method="POST"
		action="?/delete"
		use:enhance={({ cancel }) => {
			if (!confirmDelete()) cancel();
		}}
	>
		<button type="submit" class="danger">Usuń grupę</button>
	</form>
{/if}

<style>
	.meta,
	.note {
		color: #555;
	}
	.notice {
		border: 1px solid #2a7;
		background: #f2fbf6;
		padding: 0.5rem 1rem;
	}
	.error {
		border: 1px solid #c00;
		background: #fff4f4;
		padding: 0.5rem 1rem;
	}
	table {
		border-collapse: collapse;
		width: 100%;
		margin-bottom: 1rem;
	}
	th,
	td {
		border-bottom: 1px solid #ddd;
		padding: 0.35rem 0.5rem;
		text-align: left;
		vertical-align: top;
	}
	.issue {
		display: inline-block;
		margin-right: 0.35rem;
		padding: 0 0.4rem;
		border-radius: 0.25rem;
		background: #fff1d6;
		color: #6b4500;
		font-size: 0.85rem;
	}
	.search {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 0.75rem;
	}
	.candidates {
		list-style: none;
		padding: 0;
	}
	.link {
		background: none;
		border: none;
		padding: 0;
		color: #b00;
		text-decoration: underline;
		cursor: pointer;
	}
	.danger {
		color: #fff;
		background: #b00;
		border: 1px solid #900;
		padding: 0.3rem 0.8rem;
	}
	.visually-hidden {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
</style>
