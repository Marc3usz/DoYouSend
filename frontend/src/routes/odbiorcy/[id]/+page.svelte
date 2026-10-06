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
	{recipientTypeLabel(r.type)}{#if groupCount > 0}, w grupach własnych: {groupCount}{/if}
</p>

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
		if (!confirmDelete()) cancel();
	}}
>
	<button type="submit" class="danger">Usuń odbiorcę</button>
</form>

<style>
	.meta {
		color: #555;
	}
	.notice {
		border: 1px solid #2a7;
		background: #f2fbf6;
		padding: 0.5rem 1rem;
	}
	.issues {
		border: 1px solid #e0b453;
		background: #fff8e8;
		padding: 0.5rem 1rem 0.5rem 2rem;
	}
	.error {
		border: 1px solid #c00;
		background: #fff4f4;
		padding: 0.5rem 1rem;
	}
	.danger {
		color: #fff;
		background: #b00;
		border: 1px solid #900;
		padding: 0.3rem 0.8rem;
	}
</style>
