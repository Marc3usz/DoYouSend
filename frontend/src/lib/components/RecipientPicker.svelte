<!--
	Table for picking recipients: filters as you type (name, type, class), pages
	through the whole base and selects with checkboxes, so you see who exists
	before searching. Reads GET /recipients/lookup, which is open to senders and
	returns no contact data. The first page comes from the route's load.
-->
<script lang="ts">
	import {
		lookupRecipients,
		type RecipientLookup,
		type RecipientLookupPage
	} from '$lib/api/recipients';
	import { issueHint, issueText, recipientTypeLabel } from '../../routes/odbiorcy/messages';
	import {
		emptyFilters,
		pageSelection,
		pickerPageCount,
		pickerQuery,
		type PickerFilters
	} from './recipientPicker';

	// Wait for a pause in typing before asking the backend again.
	const SEARCH_DELAY_MS = 250;

	type Props = {
		/** The unfiltered first page, loaded server-side; null when that failed. */
		initial: RecipientLookupPage | null;
		pageSize: number;
		selectedIds: ReadonlySet<string>;
		/** Rows shown but not selectable, e.g. people already in the group. */
		lockedIds?: ReadonlySet<string>;
		lockedLabel?: string;
		/** Called with the people to select (selected = true) or deselect. */
		ontoggle: (people: RecipientLookup[], selected: boolean) => void;
		/** Accessible name of the table. */
		caption: string;
	};

	let {
		initial,
		pageSize,
		selectedIds,
		lockedIds = new Set<string>(),
		lockedLabel = '',
		ontoggle,
		caption
	}: Props = $props();

	const uid = $props.id();

	let filters = $state<PickerFilters>({ ...emptyFilters });
	let page = $state(1);
	// Only the first render uses `initial`; after that the picker asks the API itself.
	let result = $state<RecipientLookupPage | null>(null);
	let loading = $state(false);
	let failure = $state<string | null>(null);
	let latest = 0;
	let timer: ReturnType<typeof setTimeout> | undefined;

	const shown = $derived(result ?? initial);
	const lastPage = $derived(shown ? pickerPageCount(shown.total, pageSize) : 1);
	const filtered = $derived(filters.q.trim() !== '' || filters.type !== '' || filters.klasa !== '');
	const headState = $derived(shown ? pageSelection(shown.items, selectedIds, lockedIds) : 'none');
	const selectableOnPage = $derived(shown ? shown.items.filter((r) => !lockedIds.has(r.id)) : []);

	async function load(delay: number) {
		clearTimeout(timer);
		const req = ++latest;
		loading = true;
		failure = null;
		timer = setTimeout(async () => {
			try {
				const res = await lookupRecipients(pickerQuery(filters, page, pageSize));
				if (req === latest) result = res;
			} catch {
				if (req === latest) failure = 'Nie udało się pobrać listy odbiorców. Spróbuj ponownie.';
			} finally {
				if (req === latest) loading = false;
			}
		}, delay);
	}

	function onFilter(delay: number) {
		page = 1;
		void load(delay);
	}

	function goTo(p: number) {
		page = p;
		void load(0);
	}

	function clearFilters() {
		filters = { ...emptyFilters };
		onFilter(0);
	}

	function toggleRow(r: RecipientLookup, e: Event) {
		ontoggle([r], (e.currentTarget as HTMLInputElement).checked);
	}

	function toggleHead() {
		ontoggle(selectableOnPage, headState !== 'all');
	}

	// The search box lives inside other forms (the composer); Enter must not submit them.
	function swallowEnter(e: KeyboardEvent) {
		if (e.key === 'Enter') e.preventDefault();
	}
</script>

<div class="picker">
	<div class="filters">
		<label>
			Szukaj
			<input
				type="search"
				bind:value={filters.q}
				oninput={() => onFilter(SEARCH_DELAY_MS)}
				onkeydown={swallowEnter}
				maxlength="100"
				placeholder="imię lub nazwisko"
				autocomplete="off"
			/>
		</label>
		<label>
			Typ
			<select bind:value={filters.type} onchange={() => onFilter(0)}>
				<option value="">wszyscy</option>
				<option value="parent">rodzice</option>
				<option value="student">uczniowie</option>
			</select>
		</label>
		<label>
			Klasa
			<input
				bind:value={filters.klasa}
				oninput={() => onFilter(SEARCH_DELAY_MS)}
				onkeydown={swallowEnter}
				maxlength="6"
				size="5"
				placeholder="np. 3A"
				autocomplete="off"
			/>
		</label>
		{#if filtered}
			<button type="button" class="link" onclick={clearFilters}>Wyczyść</button>
		{/if}
	</div>

	<p class="status" aria-live="polite">
		{#if loading}
			Szukam…
		{:else if shown}
			{filtered ? 'Pasujących osób' : 'Osób w bazie'}: <strong>{shown.total}</strong>
			{#if selectedIds.size > 0}· zaznaczono: <strong>{selectedIds.size}</strong>{/if}
		{/if}
	</p>

	{#if failure || !shown}
		<p class="error" role="alert">
			{failure ?? 'Nie udało się pobrać listy odbiorców.'}
			<button type="button" class="link" onclick={() => load(0)}>Spróbuj ponownie</button>
		</p>
	{:else if shown.items.length === 0}
		<p class="hint">
			{#if filtered}Nikt nie pasuje do tych filtrów.{:else}Baza odbiorców jest pusta.{/if}
		</p>
	{:else}
		<div class="scroll" class:busy={loading}>
			<table>
				<caption class="visually-hidden">{caption}</caption>
				<thead>
					<tr>
						<th class="check">
							<input
								type="checkbox"
								checked={headState === 'all'}
								indeterminate={headState === 'some'}
								disabled={selectableOnPage.length === 0}
								onchange={toggleHead}
								aria-label="Zaznacz wszystkich na tej stronie"
							/>
						</th>
						<th>Nazwisko i imię</th>
						<th>Typ</th>
						<th>Klasa</th>
						<th>Uwagi</th>
					</tr>
				</thead>
				<tbody>
					{#each shown.items as r (r.id)}
						{@const locked = lockedIds.has(r.id)}
						{@const id = `${uid}-${r.id}`}
						<tr class:selected={selectedIds.has(r.id)} class:locked>
							<td class="check">
								<input
									{id}
									type="checkbox"
									checked={locked || selectedIds.has(r.id)}
									disabled={locked}
									onchange={(e) => toggleRow(r, e)}
								/>
							</td>
							<td>
								<label for={id}>{r.lastName} {r.firstName}</label>
								{#if locked && lockedLabel}<span class="locked-label">{lockedLabel}</span>{/if}
							</td>
							<td>{recipientTypeLabel(r.type)}</td>
							<td>{r.classes.length > 0 ? r.classes.join(', ') : '—'}</td>
							<td>
								{#each r.issues as issue (issue.channel)}
									<span class="issue" title={issueHint(issue)}>{issueText(issue)}</span>
								{/each}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		{#if lastPage > 1}
			<nav class="pager" aria-label="Strony listy odbiorców">
				<button type="button" disabled={page <= 1} onclick={() => goTo(page - 1)}>
					← Poprzednia
				</button>
				<span>Strona {page} z {lastPage}</span>
				<button type="button" disabled={page >= lastPage} onclick={() => goTo(page + 1)}>
					Następna →
				</button>
			</nav>
		{/if}
	{/if}
</div>

<style>
	.picker {
		margin: 0.5rem 0 1rem;
	}
	.filters {
		display: flex;
		gap: 0.75rem;
		align-items: end;
		flex-wrap: wrap;
	}
	.filters label {
		display: flex;
		flex-direction: column;
		font-size: 0.9rem;
	}
	.status {
		margin: 0.5rem 0;
		min-height: 1.5em;
		color: var(--color-text-muted);
	}
	.scroll {
		overflow-x: auto;
		transition: opacity 0.15s;
	}
	.busy {
		opacity: 0.6;
	}
	.check {
		width: 2.25rem;
		text-align: center;
	}
	tbody label {
		cursor: pointer;
	}
	tr.selected {
		background: var(--color-primary-soft);
	}
	tr.locked {
		color: var(--color-text-muted);
	}
	.locked-label {
		margin-left: 0.4rem;
		font-size: 0.85rem;
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
	.pager {
		display: flex;
		gap: 1rem;
		align-items: center;
		margin-top: 0.5rem;
	}
</style>
