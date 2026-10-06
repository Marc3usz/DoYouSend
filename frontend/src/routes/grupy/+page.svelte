<script lang="ts">
	import { page } from '$app/state';
	import type { PageProps } from './$types';
	import GroupForm from './GroupForm.svelte';
	import { kindLabel, peopleCount } from './messages';

	let { data, form }: PageProps = $props();

	const deleted = $derived(page.url.searchParams.has('usunieto'));
</script>

<svelte:head>
	<title>Grupy — DoYouSend</title>
</svelte:head>

<h1>Grupy odbiorców</h1>

<p class="note">
	Grupy systemowe wynikają z typu odbiorcy i zmieniają się same. Grupy własne układasz ręcznie. Ta
	sama osoba w kilku wybranych grupach dostanie wiadomość tylko raz.
</p>

{#if deleted}
	<p class="notice" role="status">
		Grupa została usunięta. Jej członkowie zostali w bazie odbiorców.
	</p>
{/if}

{#if data.failure}
	<div class="error" role="alert"><p>{data.failure}</p></div>
{:else}
	<table>
		<thead><tr><th>Nazwa</th><th>Rodzaj</th><th>Członkowie</th><th>Opis</th></tr></thead>
		<tbody>
			{#each data.groups as g (g.id)}
				<tr>
					<td><a href="/grupy/{g.id}">{g.name}</a></td>
					<td>{kindLabel(g.kind)}</td>
					<td>{peopleCount(g.memberCount)}</td>
					<td>{g.description}</td>
				</tr>
			{/each}
		</tbody>
	</table>

	<h2>Nowa grupa</h2>
	<GroupForm
		action="?/create"
		values={form?.values ?? { name: '', description: '' }}
		errors={form?.errors}
		failure={form?.failure}
		submitLabel="Utwórz grupę"
	/>
{/if}

<style>
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
		margin-bottom: 1.5rem;
	}
	th,
	td {
		border-bottom: 1px solid #ddd;
		padding: 0.35rem 0.5rem;
		text-align: left;
		vertical-align: top;
	}
</style>
