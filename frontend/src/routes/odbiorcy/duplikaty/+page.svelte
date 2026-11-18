<script lang="ts">
	import { plural } from '$lib/format/text';
	import { recipientTypeLabel } from '$lib/format/recipients';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>Możliwe duplikaty — Odbiorcy — DoYouSend</title>
</svelte:head>

<p><a href="/odbiorcy">← Odbiorcy</a></p>
<h1>Możliwe duplikaty</h1>
<p class="lead">
	Osoby tego samego typu o tym samym imieniu i nazwisku. Zwykle to ta sama osoba dodana drugi raz z
	innym e-mailem albo telefonem — wtedy popraw dane w jednym wpisie i usuń drugi. Dwie różne osoby
	mogą jednak nosić to samo imię i nazwisko, więc sprawdź klasę i kontakt przed usunięciem.
</p>

{#if data.failure}
	<div class="error" role="alert"><p>{data.failure}</p></div>
{:else if data.groups.length === 0}
	<p class="ok" role="status">Nie znaleziono możliwych duplikatów.</p>
{:else}
	<p class="count">
		{data.groups.length}
		{plural(data.groups.length, 'grupa', 'grupy', 'grup')} do sprawdzenia.
	</p>
	{#each data.groups as group (group.recipients.map((r) => r.id).join())}
		<section class="group" aria-label={group.name}>
			<h2>
				{group.name} <small>({recipientTypeLabel(group.recipients[0]?.type ?? 'parent')})</small>
			</h2>
			<table>
				<thead>
					<tr
						><th>Klasa</th><th>E-mail</th><th>Telefon</th><th
							><span class="visually-hidden">Akcje</span></th
						></tr
					>
				</thead>
				<tbody>
					{#each group.recipients as r (r.id)}
						<tr>
							<td>{r.classes.length > 0 ? r.classes.join(', ') : '—'}</td>
							<td>{r.email ?? '—'}</td>
							<td>{r.phone ?? '—'}</td>
							<td>
								<a href="/odbiorcy/{r.id}" aria-label="Otwórz wpis: {r.firstName} {r.lastName}"
									>Otwórz</a
								>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</section>
	{/each}
{/if}

<style>
	.lead {
		color: var(--color-text-muted);
		max-width: 42rem;
	}
	.ok {
		color: var(--color-success);
	}
	.group {
		margin: 1.25rem 0;
	}
	.group h2 {
		font-size: 1.1rem;
		margin-bottom: 0.4rem;
	}
	.group small {
		color: var(--color-text-muted);
		font-weight: normal;
	}
</style>
