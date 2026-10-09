<script lang="ts">
	import { page } from '$app/state';
	import type { PageProps } from './$types';
	import GroupForm from './GroupForm.svelte';
	import { kindLabel, peopleCount } from './messages';
	import { groupSections } from './sections';

	let { data, form }: PageProps = $props();

	const deleted = $derived(page.url.searchParams.has('usunieto'));
	const sections = $derived(groupSections(data.groups));
</script>

<svelte:head>
	<title>Grupy — DoYouSend</title>
</svelte:head>

<h1>Grupy odbiorców</h1>

<p class="note">
	Grupy systemowe i klasowe wynikają z danych odbiorców i zmieniają się same. Grupy własne układasz
	ręcznie. Ta sama osoba w kilku wybranych grupach dostanie wiadomość tylko raz.
</p>

{#if deleted}
	<p class="notice" role="status">
		Grupa została usunięta. Jej członkowie zostali w bazie odbiorców.
	</p>
{/if}

{#if data.failure}
	<div class="error" role="alert"><p>{data.failure}</p></div>
{:else}
	<h2>Wszyscy</h2>
	<table>
		<thead><tr><th>Nazwa</th><th>Rodzaj</th><th>Członkowie</th><th>Opis</th></tr></thead>
		<tbody>
			{#each sections.builtIn as g (g.id)}
				<tr>
					<td><a href="/grupy/{g.id}">{g.name}</a></td>
					<td>{kindLabel(g.kind)}</td>
					<td>{peopleCount(g.memberCount)}</td>
					<td>{g.description}</td>
				</tr>
			{/each}
		</tbody>
	</table>

	<h2>Klasy</h2>
	{#if sections.classes.length === 0}
		<p class="note">
			Nikt nie ma jeszcze przypisanej klasy. Wpisz klasę w danych ucznia albo rodzica (albo dodaj
			kolumnę „klasa” w pliku importu), a grupy „Uczniowie klasy …” i „Rodzice uczniów klasy …”
			pojawią się tu same.
		</p>
	{:else}
		<table>
			<thead><tr><th>Klasa</th><th>Uczniowie</th><th>Rodzice uczniów</th></tr></thead>
			<tbody>
				{#each sections.classes as row (row.className)}
					<tr>
						<th scope="row">{row.className}</th>
						<td>
							{#if row.students}
								<a href="/grupy/{row.students.id}">{peopleCount(row.students.memberCount)}</a>
							{/if}
						</td>
						<td>
							{#if row.parents}
								<a href="/grupy/{row.parents.id}">{peopleCount(row.parents.memberCount)}</a>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}

	<h2>Grupy własne</h2>
	{#if sections.custom.length === 0}
		<p class="note">Nie ma jeszcze grup własnych.</p>
	{:else}
		<table>
			<thead><tr><th>Nazwa</th><th>Członkowie</th><th>Opis</th></tr></thead>
			<tbody>
				{#each sections.custom as g (g.id)}
					<tr>
						<td><a href="/grupy/{g.id}">{g.name}</a></td>
						<td>{peopleCount(g.memberCount)}</td>
						<td>{g.description}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}

	<h3>Nowa grupa własna</h3>
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
	h2 {
		margin-top: 1.5rem;
	}
</style>
