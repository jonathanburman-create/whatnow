<script lang="ts">
	import { page } from '$app/state';
	import { api } from '#lib/api.ts';
	import AreaFilter from '#lib/components/AreaFilter.svelte';
	import Header from '#lib/components/Header.svelte';
	import IdeaCard from '#lib/components/IdeaCard.svelte';
	import { areaOf, areas, parseArea, type Area } from '#lib/areas.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { today } from '#lib/today.svelte.ts';

	const area = $derived(parseArea(page.url.searchParams.get('area')));
	const t = $derived(today.today);
	const shown = $derived(area ? today.ideas.filter((i) => i.area === area) : today.ideas);

	let draft = $state('');
	/** The new idea's area. Empty follows the filter, so ideas added while looking at UofT land there. */
	let draftArea = $state<Area | ''>('');
	let adding = $state(false);

	async function add(e: SubmitEvent) {
		e.preventDefault();
		const title = draft.trim();
		if (!title) return;
		adding = true;
		const fields = { title, notes: '', area: draftArea || area || null };
		if (await toasts.track(`Saved idea: ${title}`, () => api.createIdea(fields))) {
			draft = '';
			draftArea = '';
		}
		adding = false;
	}
</script>

{#if t}
	<Header today={t} />
	<section class="sticker intro">
		<h2>Ideas</h2>
		<p class="muted">
			Thoughts worth keeping that aren't tasks. They stay here until you come looking, and never show up in
			your plan.
		</p>
		<form class="add" onsubmit={add}>
			<input class="field" bind:value={draft} placeholder="Jot an idea…" aria-label="New idea" />
			<select class="field" bind:value={draftArea} aria-label="Area for the new idea">
				<option value="">{area ? areaOf(area)?.label : 'No area'}</option>
				{#each areas.filter((a) => a.id !== area) as a (a.id)}
					<option value={a.id}>{a.label}</option>
				{/each}
			</select>
			<button class="btn primary" disabled={adding || !draft.trim()}>Add</button>
		</form>
	</section>
	<AreaFilter {area} />
	<section>
		{#each shown as idea (idea.id)}
			<IdeaCard {idea} />
		{:else}
			<p class="muted empty">
				{area ? `No ideas in ${areaOf(area)?.label} yet.` : 'No ideas yet. Anything you dump that isn’t a task lands here.'}
			</p>
		{/each}
	</section>
{:else}
	<p class="muted">Loading…</p>
{/if}

<style>
	.intro {
		padding: 14px 16px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	h2 {
		margin: 0;
		font-size: 20px;
		font-weight: var(--heavy);
	}
	.intro p {
		margin: 0;
		font-size: 14px;
	}
	.add {
		display: flex;
		gap: 8px;
		margin-top: 4px;
	}
	.add input {
		flex: 1;
		min-width: 0;
	}
	.add select {
		max-width: 190px;
	}
	.empty {
		text-align: center;
		padding: 24px 0;
	}
	@media (max-width: 800px) {
		.intro {
			padding: 10px 12px;
		}
		.intro p {
			display: none;
		}
		.add {
			flex-wrap: wrap;
		}
		.add input {
			flex-basis: 100%;
		}
		.add select {
			flex: 1;
			max-width: none;
		}
	}
</style>
