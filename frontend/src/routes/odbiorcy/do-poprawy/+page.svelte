<script lang="ts">
	import type { PageProps } from './$types';
	import type { FixView } from '../health';
	import { peopleLabel } from '../health';
	import { issueHint, issueText, recipientTypeLabel } from '../messages';

	let { data }: PageProps = $props();

	const views: { id: FixView; label: string }[] = [
		{ id: 'all', label: 'Wszyscy z problemem' },
		{ id: 'email', label: 'Bez działającego e-maila' },
		{ id: 'sms', label: 'Bez działającego telefonu' },
		{ id: 'both', label: 'Bez żadnego kanału' }
	];
	const href = (id: FixView) => (id === 'all' ? '/odbiorcy/do-poprawy' : `?pokaz=${id}`);
</script>

<svelte:head>
	<title>Do poprawy — Odbiorcy — DoYouSend</title>
</svelte:head>

<p><a href="/odbiorcy">← Odbiorcy</a></p>
<h1>Dane do poprawy</h1>
<p class="lead">
	Osoby, które nie dostaną wiadomości w jednym kanale albo w żadnym. Popraw dane przed wysyłką albo
	wyklucz te osoby w kreatorze wiadomości.
</p>

{#if data.failure}
	<div class="error" role="alert"><p>{data.failure}</p></div>
{:else}
	{#if data.summary}
		<p class="summary">
			Bez e-maila: <strong>{data.summary.noEmail}</strong> · bez telefonu:
			<strong>{data.summary.noPhone}</strong> · bez żadnego kanału:
			<strong>{data.summary.truncated ? 'co najmniej ' : ''}{data.summary.unreachable}</strong>
		</p>
		{#if data.summary.truncated}
			<p class="notice" role="status">
				Lista pokazuje pierwsze {data.limit} osób z każdego problemu. Popraw te dane i odśwież stronę,
				żeby zobaczyć kolejne.
			</p>
		{/if}
	{/if}

	<nav class="tabs" aria-label="Rodzaj problemu">
		{#each views as v (v.id)}
			<a href={href(v.id)} aria-current={data.view === v.id ? 'page' : undefined}>{v.label}</a>
		{/each}
	</nav>

	{#if data.items.length === 0}
		<p class="ok" role="status">
			{#if data.view === 'all'}
				Nic do poprawy — każdy odbiorca ma poprawny e-mail i numer telefonu.
			{:else}
				Nikt nie pasuje do tego widoku.
			{/if}
		</p>
	{:else}
		<p class="count">{peopleLabel(data.items.length)} na liście.</p>
		<table>
			<thead>
				<tr
					><th>Nazwisko i imię</th><th>Typ</th><th>Klasa</th><th>E-mail</th><th>Telefon</th><th
						>Problem</th
					><th><span class="visually-hidden">Akcje</span></th></tr
				>
			</thead>
			<tbody>
				{#each data.items as item (item.recipient.id)}
					{@const r = item.recipient}
					<tr>
						<td>{r.lastName} {r.firstName}</td>
						<td>{recipientTypeLabel(r.type)}</td>
						<td>{r.classes.length > 0 ? r.classes.join(', ') : '—'}</td>
						<td>{r.email ?? '—'}</td>
						<td>{r.phone ?? '—'}</td>
						<td>
							{#each r.issues as issue (issue.channel)}
								<span class="issue" title={issueHint(issue)}>{issueText(issue)}</span>
							{/each}
							{#if item.channels.length > 1}
								<span class="issue severe">Nieosiągalny</span>
							{/if}
						</td>
						<td>
							<a href="/odbiorcy/{r.id}" aria-label="Popraw dane: {r.firstName} {r.lastName}"
								>Popraw</a
							>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
{/if}

<style>
	.lead {
		color: #555;
		max-width: 42rem;
	}
	.tabs {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 1rem;
		margin: 1rem 0;
		border-bottom: 1px solid #ddd;
	}
	.tabs a {
		padding: 0.35rem 0;
		text-decoration: none;
		border-bottom: 2px solid transparent;
	}
	.tabs a[aria-current='page'] {
		border-bottom-color: currentColor;
		font-weight: 600;
	}
	.notice {
		border: 1px solid #d98a00;
		background: #fff8e8;
		padding: 0.5rem 1rem;
	}
	.error {
		border: 1px solid #c00;
		background: #fff4f4;
		padding: 0.5rem 1rem;
	}
	.ok {
		color: #1b6b43;
	}
	table {
		border-collapse: collapse;
		width: 100%;
		margin-bottom: 1rem;
	}
	th,
	td {
		border-bottom: 1px solid #ddd;
		padding: 0.35rem 0.5rem;
		text-align: left;
		vertical-align: top;
	}
	.issue {
		display: inline-block;
		margin-right: 0.35rem;
		padding: 0 0.4rem;
		border-radius: 0.25rem;
		background: #fff1d6;
		color: #6b4500;
		font-size: 0.85rem;
	}
	.visually-hidden {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
	.issue.severe {
		background: #fde2e2;
		color: #7a1010;
	}
</style>
