<script lang="ts">
	import { api, type Idea } from '#lib/api.ts';
	import { areaOf, areas, type Area } from '#lib/areas.ts';
	import { dismiss } from '#lib/dismiss.ts';
	import { linkify } from '#lib/linkify.ts';
	import { toasts } from '#lib/toasts.svelte.ts';

	interface Props {
		idea: Idea;
	}
	let { idea }: Props = $props();

	let menuOpen = $state(false);
	let editing = $state(false);
	let saving = $state(false);
	let form = $state({ title: '', notes: '', area: '' as Area | '' });
	const area = $derived(areaOf(idea.area));
	const added = $derived(
		new Date(idea.created_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
	);

	function edit() {
		menuOpen = false;
		form = { title: idea.title, notes: idea.notes, area: idea.area ?? '' };
		editing = true;
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		const title = form.title.trim();
		if (!title) return;
		saving = true;
		const fields = { title, notes: form.notes, area: form.area || null };
		if (await toasts.track(`Saved: ${title}`, () => api.updateIdea(idea.id, fields))) editing = false;
		saving = false;
	}

	function toTask() {
		toasts.track(`Now a task: ${idea.title}`, () => api.ideaToTask(idea.id));
	}

	function remove() {
		menuOpen = false;
		toasts.track(`Deleted: ${idea.title}`, () => api.deleteIdea(idea.id));
	}
</script>

<article class="sticker card" data-idea={idea.id}>
	{#if editing}
		<form class="edit" onsubmit={save}>
			<input class="field" bind:value={form.title} aria-label="Idea" />
			<textarea class="field" rows="3" bind:value={form.notes} aria-label="Notes" placeholder="Notes"></textarea>
			<div class="row">
				<select class="field" bind:value={form.area} aria-label="Area">
					<option value="">No area</option>
					{#each areas as a (a.id)}
						<option value={a.id}>{a.label}</option>
					{/each}
				</select>
				<button type="button" class="btn" onclick={() => (editing = false)}>Cancel</button>
				<button class="btn primary" disabled={saving || !form.title.trim()}>Save</button>
			</div>
		</form>
	{:else}
		<div class="main">
			<div class="meta">
				{#if area}
					<a class="area" href="/ideas?area={area.id}" style:--area={area.color}>{area.label}</a>
				{/if}
				<span class="when">{added}</span>
			</div>
			<p class="title">{idea.title}</p>
			{#if idea.notes}
				<p class="notes">{#each linkify(idea.notes) as s, i (i)}{#if s.href}<a href={s.href} target="_blank" rel="noopener noreferrer">{s.text}</a>{:else}{s.text}{/if}{/each}</p>
			{/if}
		</div>
		<div class="actions">
			<button class="btn task" onclick={toTask} title="Make it a task" aria-label="Make it a task">→ Task</button>
			<details class="more" bind:open={menuOpen} {@attach dismiss}>
				<summary aria-label="More actions">⋯</summary>
				<div class="sticker menu">
					<button onclick={edit}>Edit</button>
					<button onclick={remove}>Delete</button>
				</div>
			</details>
		</div>
	{/if}
</article>

<style>
	.card {
		display: flex;
		align-items: flex-start;
		gap: 12px;
		padding: 12px 14px;
		margin-bottom: 10px;
	}
	.main {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.meta {
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 12px;
	}
	.area {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-weight: var(--heavy);
		color: var(--ink-soft);
		text-decoration: none;
	}
	.area::before {
		content: '';
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--area);
	}
	.area:hover {
		color: var(--ink);
	}
	.when {
		color: var(--ink-soft);
	}
	.title {
		margin: 0;
		font-weight: var(--heavy);
		overflow-wrap: anywhere;
	}
	.notes {
		margin: 0;
		font-size: 13px;
		white-space: pre-line;
		overflow-wrap: anywhere;
		color: var(--ink-soft);
	}
	.actions {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.task {
		font-size: 12px;
		padding: 2px 9px;
		white-space: nowrap;
		box-shadow: var(--shadow-sm);
	}
	.more {
		position: relative;
	}
	.more summary {
		list-style: none;
		cursor: pointer;
		font-weight: var(--heavy);
		padding: 0 4px;
	}
	.menu {
		position: absolute;
		right: 0;
		top: 24px;
		z-index: 10;
		display: flex;
		flex-direction: column;
		padding: 6px;
		min-width: 140px;
	}
	.menu button {
		background: none;
		border: none;
		text-align: left;
		padding: 6px 8px;
		border-radius: var(--radius-sm);
	}
	.menu button:hover {
		background: var(--chip);
	}
	.edit {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.row {
		display: flex;
		gap: 8px;
		align-items: center;
	}
	.row select {
		flex: 1;
		min-width: 0;
	}
</style>
