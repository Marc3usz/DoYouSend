<script lang="ts">
	import type { RecipientCounts } from '$lib/api/recipients';
	import { coverage, peopleLabel } from './health';

	let { counts }: { counts: RecipientCounts } = $props();

	const rows = $derived(coverage(counts));
	const problems = $derived(counts.noEmail + counts.noPhone);
</script>

<section class="health" aria-labelledby="health-title">
	<h2 id="health-title">Stan bazy odbiorców</h2>

	<dl class="totals">
		<div>
			<dt>Wszyscy</dt>
			<dd>{counts.total}</dd>
		</div>
		<div>
			<dt><a href="/odbiorcy?type=parent">Rodzice</a></dt>
			<dd>{counts.parents}</dd>
		</div>
		<div>
			<dt><a href="/odbiorcy?type=student">Uczniowie</a></dt>
			<dd>{counts.students}</dd>
		</div>
	</dl>

	<div class="channels">
		{#each rows as row (row.channel)}
			<div class="channel">
				<div class="head">
					<strong>{row.label}</strong>
					<span>
						{row.reachable} z {counts.total} osiągalnych
						{#if row.missing > 0}
							— <a href={row.href}>{peopleLabel(row.missing)} do poprawy</a>
						{/if}
					</span>
				</div>
				<div
					class="bar"
					role="img"
					aria-label="{row.label}: {row.percent}% odbiorców ma poprawne dane"
				>
					<div class="fill" class:warn={row.missing > 0} style:width="{row.percent}%"></div>
				</div>
			</div>
		{/each}
	</div>

	{#if problems > 0}
		<p class="cta">
			Osoby bez kompletu danych nie dostaną wiadomości w jednym z kanałów.
			<a href="/odbiorcy/do-poprawy">Przejdź do listy „Do poprawy”</a>
		</p>
	{:else if counts.total > 0}
		<p class="ok" role="status">Każdy odbiorca ma poprawny e-mail i numer telefonu.</p>
	{/if}
</section>

<style>
	.health {
		border: 1px solid var(--color-border);
		border-radius: 6px;
		padding: 0.75rem 1rem;
		margin: 1rem 0;
	}
	h2 {
		font-size: 1.05rem;
		margin: 0 0 0.75rem;
	}
	.totals {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem 2rem;
		margin: 0 0 1rem;
	}
	.totals div {
		display: flex;
		flex-direction: column-reverse;
	}
	.totals dt {
		color: var(--color-text-muted);
		font-size: 0.9rem;
	}
	.totals dd {
		margin: 0;
		font-size: 1.6rem;
		font-weight: 600;
	}
	.channels {
		display: grid;
		gap: 0.75rem;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		gap: 0.25rem 1rem;
		font-size: 0.9rem;
		margin-bottom: 0.25rem;
	}
	.bar {
		height: 0.6rem;
		background: var(--color-surface-muted);
		border-radius: 0.3rem;
		overflow: hidden;
	}
	.fill {
		height: 100%;
		background: var(--color-success);
	}
	.fill.warn {
		background: var(--color-warning-border);
	}
	.cta,
	.ok {
		margin: 0.75rem 0 0;
		font-size: 0.9rem;
	}
	.ok {
		color: var(--color-success);
	}
</style>
