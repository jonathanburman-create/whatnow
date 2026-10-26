<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, ApiError, type Pick, type Tier } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { doneOn, tiers, unplanned } from '#lib/plan.ts';
	import { survived } from '#lib/time.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { editor } from '#lib/editor.svelte.ts';
	import { today } from '#lib/today.svelte.ts';
	import { welcomeDue } from '#lib/welcome.ts';
	import Capture from '#lib/components/Capture.svelte';
	import DayStrip from '#lib/components/DayStrip.svelte';
	import DaysDone from '#lib/components/DaysDone.svelte';
	import DoneToday from '#lib/components/DoneToday.svelte';
	import Header from '#lib/components/Header.svelte';
	import TaskCard from '#lib/components/TaskCard.svelte';
	import WhatNow from '#lib/components/WhatNow.svelte';
	import Working from '#lib/components/Working.svelte';
	import { work } from '#lib/work.svelte.ts';

	let pick = $state<Pick | null>(null);
	let asking = $state(false);
	let replanning = $state(false);
	/** What now found nothing open and eligible. */
	let nothing = $state(false);
	/** Could starts folded on a phone, where the list is long enough already. */
	let couldOpen = $state(true);
	/** The plan's note, one line until tapped. */
	let noteOpen = $state(false);

	onMount(() => {
		if (matchMedia('(max-width: 800px)').matches) couldOpen = false;
	});

	const t = $derived(today.today);
	const planned = $derived(t ? tiers(t, today.tasks) : null);
	const tray = $derived(t ? unplanned(t, today.tasks) : []);
	const done = $derived(t ? doneOn(t.date, today.tasks) : []);
	const pickTask = $derived(pick ? today.tasks.find((x) => x.id === pick?.task_id) : undefined);
	const tierNames: Tier[] = ['must', 'should', 'could'];
	// A replan can come from anywhere: this button, a pace change, the agent, or an edit that matters.
	const replanBusy = $derived(replanning || work.busy('replan'));

	// A day with no plan starts with the morning check-in, once first-run setup has had its turn.
	$effect(() => {
		if (t && !t.plan && live.view && !welcomeDue(live.view.settings, '/')) goto('/morning', { replaceState: true });
	});

	// A nudge's What now? arrives as ?whatnow, from any page.
	$effect(() => {
		if (t?.plan && page.url.searchParams.has('whatnow')) {
			goto('/', { replaceState: true });
			if (!t.day?.wrapped_at) whatNow();
		}
	});

	// The new plan arrives over the stream as plan.updated, which refreshes the page.
	async function replan() {
		replanning = true;
		try {
			await api.replan();
		} catch (err) {
			toasts.error(err);
		} finally {
			replanning = false;
		}
	}

	async function whatNow(exclude = 0, reason = '') {
		asking = true;
		nothing = false;
		try {
			pick = await api.whatNow(exclude, reason);
		} catch (err) {
			pick = null;
			// 404 means nothing is open and eligible: news to say gently, not an error.
			if (err instanceof ApiError && err.status === 404) nothing = true;
			else toasts.error(err);
		} finally {
			asking = false;
		}
	}
</script>

{#if t?.plan}
	<Header today={t} {asking} onwhatnow={() => whatNow()} />
	{#if t.plan.opening && !t.day?.wrapped_at}
		<!-- On a phone the chat is tucked away, so the plan's note shows here. -->
		<button class="note" class:open={noteOpen} aria-expanded={noteOpen} onclick={() => (noteOpen = !noteOpen)}>
			{t.plan.opening}
		</button>
	{/if}
	<div class="strip" class:quiet={today.events.length === 0}>
		<DayStrip events={today.events} now={live.now} endOfDay={live.endOfDay} startedAt={t.day?.started_at ?? null} />
	</div>
	<div class="add">
		<Capture />
		<button class="btn" onclick={() => editor.openNew()}>+ Task</button>
	</div>

	{#if pick}
		<WhatNow
			{pick}
			task={pickTask}
			{asking}
			onnotthis={(reason) => whatNow(pick?.task_id, reason)}
			onclose={() => (pick = null)}
		/>
	{:else if nothing}
		<p class="sticker nothing">Nothing to do right now. Enjoy it.</p>
	{/if}

	{#if t.day?.wrapped_at}
		<DaysDone day={t.day} done={done.length} meetings={survived(today.events, live.now)} />
	{:else if planned}
		{#if replanBusy}<Working label="Replanning…" />{/if}
		{#if t.plan.fallback}
			<p class="chip rough">rough plan, model was out</p>
		{/if}
		{#each tierNames as tier (tier)}
			{@const folded = tier === 'could' && !couldOpen && planned.could.length > 0}
			<section>
				<h2 class="label">
					{#if tier === 'could'}
						<button class="fold" aria-expanded={!folded} onclick={() => (couldOpen = !couldOpen)}>
							{tier} <span class="count" aria-hidden="true">{planned[tier].length}</span>
							<span aria-hidden="true">{folded ? '▸' : '▾'}</span>
						</button>
					{:else}
						{tier} <span class="count" aria-hidden="true">{planned[tier].length}</span>
					{/if}
				</h2>
				{#if !folded}
					{#each planned[tier] as card (card.task.id)}
						<TaskCard {card} {tier} date={t.date} />
					{:else}
						<p class="muted empty">Nothing here.</p>
					{/each}
				{/if}
			</section>
		{/each}
	{/if}

	<div class="lower">
		<section>
			<div class="tray-head">
				<h2 class="label">Unplanned</h2>
				{#if tray.length && !t.day?.wrapped_at}
					<button class="btn" disabled={replanBusy} onclick={replan}>
						{replanBusy ? 'Replanning…' : 'Replan'}
					</button>
				{/if}
			</div>
			{#each tray as card (card.task.id)}
				<TaskCard {card} date={t.date} />
			{:else}
				<p class="muted empty">Tray's empty.</p>
			{/each}
		</section>
		<section>
			<DoneToday {done} meetings={survived(today.events, live.now)} />
		</section>
	</div>
{:else}
	<p class="muted">Loading…</p>
{/if}

<style>
	h2 {
		margin: 6px 0 8px;
	}
	.empty {
		margin: 0 0 8px;
		font-size: 14px;
	}
	.nothing {
		margin: 0;
		padding: 12px 16px;
		font-weight: var(--heavy);
	}
	.rough {
		align-self: flex-start;
		margin: 0;
	}
	.add {
		display: flex;
		gap: 10px;
		align-items: center;
	}
	.add > :global(:first-child) {
		flex: 1;
	}
	.tray-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 1rem;
	}
	.lower {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 16px;
		align-items: start;
	}

	.count {
		opacity: 0.55;
		margin-left: 2px;
	}
	.fold {
		background: none;
		border: none;
		padding: 0;
		font: inherit;
		color: inherit;
		letter-spacing: inherit;
		text-transform: inherit;
		cursor: pointer;
	}
	.note {
		display: none;
	}

	@media (max-width: 800px) {
		.lower {
			grid-template-columns: 1fr;
		}
		h2 {
			margin: 2px 0 6px;
		}
		/* The + button above the tab bar captures instead; cleanup's progress line still shows. */
		.add :global(form),
		.add > .btn {
			display: none;
		}
		.add:not(:has(> :not(form, .btn))) {
			display: none;
		}
		.strip.quiet {
			display: none;
		}
		.note {
			display: block;
			background: none;
			border: none;
			border-left: 3px solid var(--primary);
			padding: 2px 0 2px 10px;
			text-align: left;
			font-size: 14px;
			color: var(--ink-soft);
			overflow: hidden;
			text-overflow: ellipsis;
			white-space: nowrap;
		}
		.note.open {
			white-space: normal;
			color: var(--ink);
		}
	}
</style>
