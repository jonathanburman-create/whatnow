<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Header from '#lib/components/Header.svelte';
	import TaskCard from '#lib/components/TaskCard.svelte';
	import { areas, inArea, parseArea, type Area } from '#lib/areas.ts';
	import { editor } from '#lib/editor.svelte.ts';
	import { filters, listTasks, type Filter } from '#lib/tasks.ts';
	import { today } from '#lib/today.svelte.ts';

	let filter = $state<Filter>('open');

	// The area lives in the address, so a task's area label can link straight to it.
	const area = $derived(parseArea(page.url.searchParams.get('area')));
	const t = $derived(today.today);
	const shown = $derived(t ? inArea(listTasks(today.tasks, today.blockers, filter, t.date), area) : []);

	function pickArea(id: Area | '') {
		goto(id ? `?area=${id}` : '?', { replace: true, reset: false });
	}
</script>

{#if t}
	<Header today={t} />
	<div class="chips areas" role="tablist" aria-label="Filter by area">
		<button class="chip" class:on={area === ''} role="tab" aria-selected={area === ''} onclick={() => pickArea('')}>
			All areas
		</button>
		{#each areas as a (a.id)}
			<button
				class="chip area"
				class:on={area === a.id}
				role="tab"
				aria-selected={area === a.id}
				style:--area={a.color}
				onclick={() => pickArea(a.id)}
			>
				{a.label}
			</button>
		{/each}
	</div>
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
	.areas {
		margin-bottom: 8px;
	}
	/* On a narrow screen the areas sit in one row that scrolls sideways, so the list starts higher up. */
	@media (max-width: 800px) {
		.areas {
			flex-wrap: nowrap;
			overflow-x: auto;
			scrollbar-width: none;
			margin-inline: -12px;
			padding: 2px 12px 4px;
		}
		.areas .chip {
			flex: none;
		}
	}
	.chip {
		background: var(--card);
		cursor: pointer;
	}
	.chip.on {
		background: var(--chip);
	}
	.chip.area {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.chip.area::before {
		content: '';
		width: 9px;
		height: 9px;
		border-radius: 50%;
		background: var(--area);
	}
</style>
