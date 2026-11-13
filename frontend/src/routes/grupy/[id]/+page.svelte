<script lang="ts">
	import { enhance } from '$app/forms';
	import { page } from '$app/state';
	import type { RecipientLookup } from '$lib/api/recipients';
	import RecipientPicker from '$lib/components/RecipientPicker.svelte';
	import type { PageProps } from './$types';
	import GroupForm from '../GroupForm.svelte';
	import { kindLabel, peopleCount } from '../messages';
	import { issueHint, issueText, recipientTypeLabel } from '$lib/format/recipients';

	let { data, form }: PageProps = $props();

	const g = $derived(data.group);
	const custom = $derived(g.kind === 'custom');
	const created = $derived(!form && page.url.searchParams.has('utworzono'));
	const memberIds = $derived(new Set(data.members.map((m) => m.id)));

	// People ticked in the picker, kept across its pages and filters.
	let picked = $state<RecipientLookup[]>([]);
	const pickedIds = $derived(new Set(picked.map((r) => r.id)));

	function togglePeople(people: RecipientLookup[], selected: boolean) {
		if (selected) {
			picked = [...picked, ...people.filter((r) => !pickedIds.has(r.id))];
		} else {
			const drop = new Set(people.map((r) => r.id));
			picked = picked.filter((r) => !drop.has(r.id));
		}
	}

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
	<RecipientPicker
		initial={data.people}
		pageSize={20}
		selectedIds={pickedIds}
		lockedIds={memberIds}
		lockedLabel="(już w grupie)"
		ontoggle={togglePeople}
		caption="Osoby do dodania do grupy"
	/>
	<form
		method="POST"
		action="?/add"
		use:enhance={() =>
			async ({ result, update }) => {
				await update();
				if (result.type === 'success') picked = [];
			}}
		class="add"
	>
		{#each picked as r (r.id)}
			<input type="hidden" name="recipientId" value={r.id} />
		{/each}
		<button type="submit" class="primary" disabled={picked.length === 0}>
			Dodaj zaznaczonych{picked.length > 0 ? ` (${picked.length})` : ''}
		</button>
		{#if picked.length > 0}
			<button type="button" class="link" onclick={() => (picked = [])}>Odznacz wszystkich</button>
		{/if}
	</form>

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
		color: var(--color-text-muted);
	}
	.issue {
		display: inline-block;
		margin-right: 0.35rem;
		padding: 0 0.4rem;
		border-radius: 0.25rem;
		background: var(--color-warning-soft);
		color: var(--color-warning);
		font-size: 0.85rem;
	}
	.add {
		display: flex;
		gap: 1rem;
		align-items: center;
	}
	.link {
		background: none;
		border: none;
		padding: 0;
		color: var(--color-danger);
		text-decoration: underline;
		cursor: pointer;
	}
</style>
