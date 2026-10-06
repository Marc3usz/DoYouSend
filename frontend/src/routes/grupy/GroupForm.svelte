<script lang="ts">
	import { enhance } from '$app/forms';
	import type { GroupFormErrors, GroupFormValues } from './messages';

	type Props = {
		values: GroupFormValues;
		errors?: GroupFormErrors;
		failure?: string | null;
		submitLabel: string;
		action: string;
	};

	let { values, errors = {}, failure = null, submitLabel, action }: Props = $props();

	let saving = $state(false);
</script>

<form
	method="POST"
	{action}
	novalidate
	use:enhance={() => {
		saving = true;
		return async ({ update }) => {
			await update({ reset: false });
			saving = false;
		};
	}}
>
	{#if failure}
		<div class="error" role="alert"><p>{failure}</p></div>
	{/if}
	<label>
		Nazwa
		<input
			name="name"
			value={values.name}
			maxlength="100"
			autocomplete="off"
			aria-invalid={errors.name ? 'true' : undefined}
			aria-describedby={errors.name ? 'err-name' : undefined}
		/>
		{#if errors.name}<span class="field-error" id="err-name">{errors.name}</span>{/if}
	</label>
	<label>
		Opis <span class="hint">(opcjonalnie)</span>
		<textarea
			name="description"
			rows="2"
			maxlength="500"
			aria-invalid={errors.description ? 'true' : undefined}
			aria-describedby={errors.description ? 'err-description' : undefined}
			>{values.description}</textarea
		>
		{#if errors.description}
			<span class="field-error" id="err-description">{errors.description}</span>
		{/if}
	</label>
	<button type="submit" disabled={saving}>{saving ? 'Zapisuję…' : submitLabel}</button>
</form>

<style>
	form {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		max-width: 36rem;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}
	[aria-invalid='true'] {
		border-color: #c00;
	}
	.field-error {
		color: #c00;
		font-size: 0.85rem;
	}
	.hint {
		color: #555;
		font-size: 0.85rem;
	}
	.error {
		border: 1px solid #c00;
		background: #fff4f4;
		padding: 0.5rem 1rem;
	}
	button {
		align-self: flex-start;
	}
</style>
