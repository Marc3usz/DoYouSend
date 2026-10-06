<script lang="ts">
	import type { PageProps } from './$types';
	import RecipientForm from '../RecipientForm.svelte';
	import { emptyValues } from '../form';

	let { form }: PageProps = $props();
</script>

<svelte:head>
	<title>Nowy odbiorca — DoYouSend</title>
</svelte:head>

<p><a href="/odbiorcy">← Odbiorcy</a></p>
<h1>Nowy odbiorca</h1>

<RecipientForm
	values={form?.values ?? emptyValues()}
	errors={form?.errors}
	failure={form?.failure}
	submitLabel="Dodaj odbiorcę"
/>
