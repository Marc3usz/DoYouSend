<script lang="ts">
	import { enhance } from '$app/forms';
	import type { RecipientFormErrors, RecipientFormValues } from './form';

	type Props = {
		values: RecipientFormValues;
		errors?: RecipientFormErrors;
		failure?: string | null;
		submitLabel: string;
		/** Form action, e.g. "?/save"; the default action when not set. */
		action?: string;
	};

	let { values, errors, failure = null, submitLabel, action }: Props = $props();

	let saving = $state(false);
	const fields = $derived(errors?.fields ?? {});
</script>

<!-- reset: false keeps what was typed when the save is rejected; after a
     successful edit the page data is reloaded and fills the fields again. -->
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

	<div class="row">
		<label>
			Imię
			<input
				name="firstName"
				value={values.firstName}
				autocomplete="off"
				aria-invalid={fields.firstName ? 'true' : undefined}
				aria-describedby={fields.firstName ? 'err-firstName' : undefined}
			/>
			{#if fields.firstName}<span class="field-error" id="err-firstName">{fields.firstName}</span
				>{/if}
		</label>
		<label>
			Nazwisko
			<input
				name="lastName"
				value={values.lastName}
				autocomplete="off"
				aria-invalid={fields.lastName ? 'true' : undefined}
				aria-describedby={fields.lastName ? 'err-lastName' : undefined}
			/>
			{#if fields.lastName}<span class="field-error" id="err-lastName">{fields.lastName}</span>{/if}
		</label>
	</div>

	<label>
		Typ
		<select
			name="type"
			value={values.type}
			aria-invalid={fields.type ? 'true' : undefined}
			aria-describedby={fields.type ? 'err-type' : undefined}
		>
			<option value="">— wybierz —</option>
			<option value="parent">rodzic</option>
			<option value="student">uczeń</option>
		</select>
		{#if fields.type}<span class="field-error" id="err-type">{fields.type}</span>{/if}
	</label>

	<label>
		Klasy
		<input
			name="classes"
			value={values.classes}
			autocomplete="off"
			placeholder="np. 3A — rodzic dzieci z dwóch klas: 1B, 3A"
			aria-invalid={fields.classes ? 'true' : undefined}
			aria-describedby={fields.classes ? 'err-classes' : 'hint-classes'}
		/>
		{#if fields.classes}<span class="field-error" id="err-classes">{fields.classes}</span
			>{:else}<span class="hint" id="hint-classes"
				>Uczeń: jego klasa. Rodzic: klasy dzieci. Na tej podstawie powstają grupy „Uczniowie klasy
				3A” i „Rodzice uczniów klasy 3A”.</span
			>{/if}
	</label>

	<fieldset aria-describedby={fields.contact ? 'err-contact' : undefined}>
		<legend>Kontakt — co najmniej jedno z dwóch</legend>
		{#if fields.contact}<p class="field-error" id="err-contact">{fields.contact}</p>{/if}
		<div class="row">
			<label>
				E-mail
				<input
					type="email"
					name="email"
					value={values.email}
					autocomplete="off"
					aria-invalid={fields.email ? 'true' : undefined}
					aria-describedby={fields.email ? 'err-email' : undefined}
				/>
				{#if fields.email}<span class="field-error" id="err-email">{fields.email}</span>{/if}
			</label>
			<label>
				Telefon
				<input
					type="tel"
					name="phone"
					value={values.phone}
					autocomplete="off"
					placeholder="+48 500 100 101"
					aria-invalid={fields.phone ? 'true' : undefined}
					aria-describedby={fields.phone ? 'err-phone' : undefined}
				/>
				{#if fields.phone}<span class="field-error" id="err-phone">{fields.phone}</span>{/if}
			</label>
		</div>
		{#if errors?.duplicateOf}
			<p><a href="/odbiorcy/{errors.duplicateOf}">Zobacz odbiorcę, który ma już te dane</a></p>
		{/if}
		<p class="hint">Numer bez kierunkowego zostanie zapisany jako polski (+48).</p>
	</fieldset>

	<button type="submit" disabled={saving}>{saving ? 'Zapisuję…' : submitLabel}</button>
</form>

<style>
	form {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
		max-width: 36rem;
	}
	.row {
		display: flex;
		gap: 1rem;
		flex-wrap: wrap;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		flex: 1 1 14rem;
	}
	fieldset {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		border: 1px solid #ddd;
	}
	[aria-invalid='true'] {
		border-color: #c00;
	}
	.field-error {
		color: #c00;
		font-size: 0.85rem;
		margin: 0;
	}
	.hint {
		color: #555;
		font-size: 0.85rem;
		margin: 0;
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
