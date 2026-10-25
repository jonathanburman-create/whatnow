<script lang="ts">
	import { page } from '$app/state';
	import AreaFilter from '#lib/components/AreaFilter.svelte';
	import Header from '#lib/components/Header.svelte';
	import TaskCard from '#lib/components/TaskCard.svelte';
	import { inArea, parseArea } from '#lib/areas.ts';
	import { editor } from '#lib/editor.svelte.ts';
	import { filters, listTasks, type Filter } from '#lib/tasks.ts';
	import { today } from '#lib/today.svelte.ts';

	let filter = $state<Filter>('open');

	// The area lives in the address, so a task's area label can link straight to it.
	const area = $derived(parseArea(page.url.searchParams.get('area')));
	const t = $derived(today.today);
	const shown = $derived(t ? inArea(listTasks(today.tasks, today.blockers, filter, t.date), area) : []);
</script>

{#if t}
	<Header today={t} />
	<AreaFilter {area} />
	<div class="chips" role="tablist" aria-label="Filter tasks">
		{#each filters as f (f.id)}
			<button
				class="chip"
				class:on={filter === f.id}
				role="tab"
				aria-selected={filter === f.id}
				onclick={() => (filter = f.id)}
			>
				{f.label}
			</button>
		{/each}
		<button class="btn add" onclick={() => editor.openNew()}>+ Task</button>
	</div>
	<section>
		{#each shown as task (task.id)}
			<TaskCard card={{ task, rationale: '' }} date={t.date} listed />
		{:else}
			<p class="muted">Nothing here.</p>
		{/each}
	</section>
{:else}
	<p class="muted">Loading…</p>
{/if}

<style>
	.add {
		margin-left: auto;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}
	.chip {
		background: var(--card);
		cursor: pointer;
	}
	.chip.on {
		background: var(--chip);
	}
</style>
