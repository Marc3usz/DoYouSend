<script lang="ts">
	import { enhance } from '$app/forms';
	import { page } from '$app/state';
	import type { PageProps } from './$types';
	import RecipientForm from '../RecipientForm.svelte';
	import { valuesOf } from '../form';
	import { issueHint, issueText, recipientTypeLabel } from '../messages';

	let { data, form }: PageProps = $props();

	const r = $derived(data.recipient);
	const groupCount = $derived(r.groupIds?.length ?? 0);
	let deleting = $state(false);
	// The values of a rejected save stay on screen; otherwise the stored data.
	const values = $derived(form?.values ?? valuesOf(r));
	const saved = $derived(form?.saved === true);
	const created = $derived(!form && page.url.searchParams.has('dodano'));

	function confirmDelete(): boolean {
		const groups = groupCount > 0 ? ` Zostanie też wypisana z grup własnych (${groupCount}).` : '';
		return confirm(`Usunąć odbiorcę ${r.firstName} ${r.lastName}?${groups}`);
	}
</script>

<svelte:head>
	<title>{r.firstName} {r.lastName} — DoYouSend</title>
</svelte:head>

<p><a href="/odbiorcy">← Odbiorcy</a></p>
<h1>{r.firstName} {r.lastName}</h1>
<p class="meta">
	{recipientTypeLabel(r.type)}{#if r.classes.length > 0}, klasa {r.classes.join(', ')}{/if}
</p>

<section aria-labelledby="groups-heading">
	<h2 id="groups-heading">Grupy</h2>
	{#if data.groups === null}
		<p class="meta">
			Nie udało się wczytać nazw grup{#if groupCount > 0}
				(grupy własne: {groupCount}){/if}.
		</p>
	{:else if data.groups.length === 0}
		<p class="meta">
			Tylko grupa wszystkich {r.type === 'student' ? 'uczniów' : 'rodziców'}. Nie należy do żadnej
			grupy własnej ani klasowej.
		</p>
	{:else}
		<ul class="groups">
			{#each data.groups as g (g.id)}
				<li>
					<a href="/grupy/{g.id}">{g.name}</a>{#if g.kind === 'system'}
						<span class="meta"> · klasowa</span>{/if}
				</li>
			{/each}
		</ul>
		<p class="meta">Oraz grupa wszystkich {r.type === 'student' ? 'uczniów' : 'rodziców'}.</p>
	{/if}
</section>

{#if created}
	<p class="notice" role="status">Odbiorca został dodany.</p>
{:else if saved}
	<p class="notice" role="status">Zmiany zostały zapisane.</p>
{/if}

{#if r.issues.length > 0}
	<ul class="issues">
		{#each r.issues as issue (issue.channel)}
			<li><strong>{issueText(issue)}.</strong> {issueHint(issue)}</li>
		{/each}
	</ul>
{/if}

<h2>Dane</h2>
<RecipientForm
	{values}
	errors={form?.errors}
	failure={form?.failure}
	action="?/save"
	submitLabel="Zapisz zmiany"
/>

<h2>Usuwanie</h2>
<p class="meta">
	Nie da się usunąć osoby, do której poszła już jakaś wiadomość — historia wysyłek musi to
	pokazywać.
</p>
{#if form?.deleteFailure}
	<div class="error" role="alert"><p>{form.deleteFailure}</p></div>
{/if}
<form
	method="POST"
	action="?/delete"
	use:enhance={({ cancel }) => {
		if (!confirmDelete()) {
			cancel();
			return;
		}
		deleting = true;
		return async ({ update }) => {
			await update();
			deleting = false;
		};
	}}
>
	<button type="submit" class="danger" disabled={deleting}>
		{deleting ? 'Usuwam…' : 'Usuń odbiorcę'}
	</button>
</form>

<style>
	.meta {
		color: var(--color-text-muted);
	}
	.groups {
		padding-left: 1.25rem;
	}
	.issues {
		border: 1px solid var(--color-warning-border);
		background: var(--color-warning-soft);
		padding: 0.5rem 1rem 0.5rem 2rem;
	}
</style>
