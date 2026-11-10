<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { navigating, page as pageState } from '$app/state';
	import type { PageProps } from './$types';
	import HealthPanel from './HealthPanel.svelte';
	import { listHref, pageCount, parseListParams, sortHref, sortIndicator } from './list';
	import { issueHint, issueText, recipientTypeLabel } from './messages';

	let { data }: PageProps = $props();

	const lastPage = $derived(data.page ? pageCount(data.page.total) : 1);
	const filtered = $derived(
		data.params.q !== '' ||
			data.params.type !== '' ||
			data.params.klasa !== '' ||
			data.params.issue !== ''
	);
	const deleted = $derived(pageState.url.searchParams.has('usunieto'));

	// Filters apply as you type: after a pause the form state goes to the URL,
	// and the server load runs again. The submit button stays for no-JS use.
	const FILTER_DELAY_MS = 250;
	let filterTimer: ReturnType<typeof setTimeout> | undefined;

	// The text fields keep what is being typed. The URL holds the normalized form
	// ("Jan " -> "Jan", "3 a" -> "3A"), and writing it back mid-typing would eat
	// the space; so they follow the URL only when it means something else (a
	// "Wyczyść" link, the back button).
	let qInput = $state(untrack(() => data.params.q));
	let klasaInput = $state(untrack(() => data.params.klasa));
	// What applyFilters last put into the URL: a load it caused must not touch the
	// fields, the person may have typed on while it ran.
	let pushed: { q: string; klasa: string } | null = null;
	$effect(() => {
		const { q, klasa } = data.params;
		untrack(() => {
			if (pushed && pushed.q === q && pushed.klasa === klasa) return;
			pushed = null;
			const typed = parseListParams(new URLSearchParams({ q: qInput, klasa: klasaInput }));
			if (typed.q !== q) qInput = q;
			if (typed.klasa !== klasa) klasaInput = klasa;
		});
	});

	function applyFilters(form: HTMLFormElement, delay: number) {
		clearTimeout(filterTimer);
		filterTimer = setTimeout(() => {
			const fd = new FormData(form);
			const search = new URLSearchParams();
			for (const [k, v] of fd) if (typeof v === 'string') search.set(k, v);
			const params = parseListParams(search);
			const href = listHref({ ...params, page: 1 });
			if (href !== pageState.url.pathname + pageState.url.search) {
				pushed = { q: params.q, klasa: params.klasa };
				void goto(href, { keepFocus: true, noScroll: true, replaceState: true });
			}
		}, delay);
	}
</script>

<svelte:head>
	<title>Odbiorcy — DoYouSend</title>
</svelte:head>

<h1>Odbiorcy</h1>

<p class="actions">
	<a class="button primary" href="/odbiorcy/nowy">Dodaj odbiorcę</a>
	<a class="button" href="/odbiorcy/import">Import z pliku</a>
	<a class="button" href="/odbiorcy/do-poprawy">Do poprawy</a>
	<a class="button" href="/grupy">Grupy</a>
</p>

{#if data.counts && !filtered}
	<HealthPanel counts={data.counts} />
{/if}

{#if deleted}
	<p class="notice" role="status">Odbiorca został usunięty.</p>
{/if}

<!-- A plain GET form: the filters land in the URL, so a view can be linked. -->
<form
	method="GET"
	class="filters"
	oninput={(e) => applyFilters(e.currentTarget, FILTER_DELAY_MS)}
	onchange={(e) => applyFilters(e.currentTarget, 0)}
>
	{#if data.params.sort !== 'name'}
		<input type="hidden" name="sort" value={data.params.sort} />
	{/if}
	{#if data.params.order !== 'asc'}
		<input type="hidden" name="order" value={data.params.order} />
	{/if}
	<label>
		Szukaj
		<input
			type="search"
			name="q"
			bind:value={qInput}
			maxlength="100"
			placeholder="imię, nazwisko, e-mail lub telefon"
		/>
	</label>
	<label>
		Typ
		<select name="type" value={data.params.type}>
			<option value="">wszyscy</option>
			<option value="parent">rodzice</option>
			<option value="student">uczniowie</option>
		</select>
	</label>
	<label>
		Klasa
		<input name="klasa" bind:value={klasaInput} maxlength="6" size="5" placeholder="np. 3A" />
	</label>
	<label>
		Problem z danymi
		<select name="issue" value={data.params.issue}>
			<option value="">dowolny</option>
			<option value="email">bez działającego e-maila</option>
			<option value="sms">bez działającego telefonu</option>
		</select>
	</label>
	<button type="submit">Filtruj</button>
	{#if filtered}
		<a href="/odbiorcy">Wyczyść</a>
	{/if}
</form>

{#if data.failure}
	<div class="error" role="alert"><p>{data.failure}</p></div>
{:else if data.page}
	<p class="summary" aria-live="polite">
		{#if navigating.to}Szukam…
		{/if}{#if filtered}Pasujących odbiorców{:else}Odbiorców w bazie{/if}:
		<strong>{data.page.total}</strong>
	</p>

	{#if data.page.items.length === 0}
		<p>
			{#if filtered}
				Nikt nie pasuje do tych filtrów.
			{:else if data.page.total === 0}
				Baza jest pusta. <a href="/odbiorcy/nowy">Dodaj pierwszego odbiorcę</a> albo
				<a href="/odbiorcy/import">sprawdź plik do importu</a>.
			{:else}
				Ta strona listy jest pusta. <a href={listHref({ ...data.params, page: 1 })}
					>Wróć do początku</a
				>.
			{/if}
		</p>
	{:else}
		<table>
			<thead>
				<tr>
					<th>
						<a class="th-sort" href={sortHref(data.params, 'name')}>
							Nazwisko i imię <span class="sort-icon">{sortIndicator(data.params, 'name')}</span>
						</a>
					</th>
					<th>
						<a class="th-sort" href={sortHref(data.params, 'type')}>
							Typ <span class="sort-icon">{sortIndicator(data.params, 'type')}</span>
						</a>
					</th>
					<th>
						<a class="th-sort" href={sortHref(data.params, 'class')}>
							Klasa <span class="sort-icon">{sortIndicator(data.params, 'class')}</span>
						</a>
					</th>
					<th>
						<a class="th-sort" href={sortHref(data.params, 'email')}>
							E-mail <span class="sort-icon">{sortIndicator(data.params, 'email')}</span>
						</a>
					</th>
					<th>
						<a class="th-sort" href={sortHref(data.params, 'phone')}>
							Telefon <span class="sort-icon">{sortIndicator(data.params, 'phone')}</span>
						</a>
					</th>
					<th>Uwagi</th>
				</tr>
			</thead>
			<tbody>
				{#each data.page.items as r (r.id)}
					<tr>
						<td><a href="/odbiorcy/{r.id}">{r.lastName} {r.firstName}</a></td>
						<td>{recipientTypeLabel(r.type)}</td>
						<td>{r.classes.length > 0 ? r.classes.join(', ') : '—'}</td>
						<td>{r.email ?? '—'}</td>
						<td>{r.phone ?? '—'}</td>
						<td>
							{#each r.issues as issue (issue.channel)}
								<span class="issue" title={issueHint(issue)}>{issueText(issue)}</span>
							{/each}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>

		{#if lastPage > 1}
			<nav class="pager" aria-label="Strony listy">
				{#if data.params.page > 1}
					<a href={listHref({ ...data.params, page: data.params.page - 1 })}>← Poprzednia</a>
				{/if}
				<span>Strona {data.params.page} z {lastPage}</span>
				{#if data.params.page < lastPage}
					<a href={listHref({ ...data.params, page: data.params.page + 1 })}>Następna →</a>
				{/if}
			</nav>
		{/if}
	{/if}
{/if}

<style>
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.filters {
		display: flex;
		gap: 0.75rem;
		align-items: end;
		flex-wrap: wrap;
		margin: 1rem 0;
	}
	.filters label {
		display: flex;
		flex-direction: column;
		font-size: 0.9rem;
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
	}
	.th-sort {
		color: inherit;
		text-decoration: none;
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
	}
	.th-sort:hover {
		text-decoration: underline;
	}
	.sort-icon {
		font-size: 0.75em;
		opacity: 0.7;
	}
</style>
