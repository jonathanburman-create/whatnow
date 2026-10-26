<script lang="ts">
	import { api, configured, paceLabels, type Today } from '#lib/api.ts';
	import { dismiss } from '#lib/dismiss.ts';
	import { session } from '#lib/session.svelte.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { countdown, wrapDue } from '#lib/time.ts';
	import { today as todayState } from '#lib/today.svelte.ts';

	interface Props {
		today: Today;
		/** Today passes these to offer What now?; other pages leave them out. */
		asking?: boolean;
		onwhatnow?: () => void;
	}
	let { today, asking = false, onwhatnow }: Props = $props();

	let paceOpen = $state(false);
	let menuOpen = $state(false);
	let accountOpen = $state(false);
	const user = $derived(session.value?.hosted ? session.value.user : null);
	const next = $derived(countdown(todayState.events, live.now));
	const wrapped = $derived(today.day?.wrapped_at != null);
	const canWrap = $derived(today.day !== null && !wrapped);
	const dateFor = (weekday: 'long' | 'short') =>
		new Date(`${today.date}T12:00:00`).toLocaleDateString(undefined, { weekday, month: 'short', day: 'numeric' });
	const date = $derived(dateFor('long'));
	// Phones get the short form, so the date, pace, and What now? fit on one line.
	const shortDate = $derived(dateFor('short'));

	async function signOut() {
		try {
			await api.logout();
			location.assign('/');
		} catch (err) {
			toasts.error(err);
		}
	}

	function setPace(pace: number) {
		paceOpen = false;
		toasts.track(`Pace: ${paceLabels[pace - 1]}`, () => api.setPace(pace));
	}
</script>

<header class="sticker">
	<div class="left">
		<h1><span class="wide">{date}</span><span class="phone">{shortDate}</span></h1>
		{#if today.day}
			<details class="pace" bind:open={paceOpen} {@attach dismiss}>
				<summary class="chip"><span class="wide">{'pace: '}</span>{paceLabels[today.day.pace - 1]}</summary>
				<div class="sticker menu">
					{#each paceLabels as label, i (label)}
						<button class="item" class:current={i + 1 === today.day.pace} onclick={() => setPace(i + 1)}>
							{label}
						</button>
					{/each}
				</div>
			</details>
		{/if}
		{#if !live.connected}
			<span class="chip offline">reconnecting…</span>
		{/if}
	</div>
	<div class="right">
		{#if next}
			<span class="next wide">{next}</span>
		{/if}
		{#if canWrap && wrapDue(live.now, live.endOfDay)}
			<a class="btn" href="/wrap">Wrap up</a>
		{/if}
		{#if onwhatnow && !wrapped}
			<button class="btn primary" disabled={asking || !today.day} onclick={onwhatnow}>
				{asking ? 'Thinking…' : 'What now?'}
			</button>
		{/if}
		{#if user}
			<details class="more wide" bind:open={accountOpen} {@attach dismiss}>
				<summary class="btn" aria-label="Account">{user.name}</summary>
				<div class="sticker menu">
					<a class="item" href="/privacy" onclick={() => (accountOpen = false)}>Privacy</a>
					<button class="item" onclick={signOut}>Sign out</button>
				</div>
			</details>
		{/if}
		<details class="more" bind:open={menuOpen} {@attach dismiss}>
			<summary class="btn" aria-label="Menu">⋯</summary>
			<nav class="sticker menu">
				<!-- On a phone the tab bar has the pages, and the account's links move in here. -->
				<a class="item wide" href="/" onclick={() => (menuOpen = false)}>Today</a>
				<a class="item wide" href="/tasks" onclick={() => (menuOpen = false)}>All tasks</a>
				<a class="item wide" href="/ideas" onclick={() => (menuOpen = false)}>Ideas</a>
				{#if canWrap}<a class="item" href="/wrap" onclick={() => (menuOpen = false)}>Wrap up early</a>{/if}
				<a class="item" href="/settings" onclick={() => (menuOpen = false)}>Settings</a>
				{#if user}
					<a class="item phone" href="/privacy" onclick={() => (menuOpen = false)}>Privacy</a>
					<button class="item phone" onclick={signOut}>Sign out</button>
				{/if}
			</nav>
		</details>
	</div>
	{#if next}
		<span class="next phone">{next}</span>
	{/if}
	{#if live.view && !configured(live.view)}
		<p class="notice">
			No model set up yet. Capture works and plans come out rough. <a href="/settings">Set one up</a>
		</p>
	{/if}
</header>

<style>
	header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 12px 16px;
	}
	.left,
	.right {
		display: flex;
		align-items: center;
		gap: 12px;
	}
	h1 {
		margin: 0;
		font-size: 22px;
		font-weight: var(--heavy);
	}
	.next {
		font-weight: var(--heavy);
	}
	.pace {
		position: relative;
	}
	.pace summary {
		list-style: none;
		cursor: pointer;
	}
	.menu {
		position: absolute;
		top: 32px;
		left: 0;
		z-index: 10;
		display: flex;
		flex-direction: column;
		padding: 6px;
		min-width: 170px;
	}
	.item {
		background: none;
		border: none;
		text-align: left;
		padding: 6px 8px;
		border-radius: var(--radius-sm);
	}
	.item:hover,
	.item.current {
		background: var(--chip);
	}
	a.btn,
	a.item {
		color: var(--ink);
		text-decoration: none;
	}
	.more {
		position: relative;
	}
	.more summary {
		list-style: none;
	}
	.more .menu {
		left: auto;
		right: 0;
		top: 40px;
	}
	.offline {
		background: var(--must);
	}
	.notice {
		flex-basis: 100%;
		margin: 0;
		font-size: 14px;
	}
	.phone {
		display: none;
	}

	/* Phones: one slim row (short date, pace, What now?, menu), with the next event under it. */
	@media (max-width: 800px) {
		header {
			gap: 6px 8px;
			padding: 8px 10px;
		}
		/* The date never shrinks under the buttons; in the evening, when Wrap up joins them, they
		   drop to a row of their own. */
		.left {
			flex: 1 0 auto;
			gap: 8px;
		}
		.right {
			margin-left: auto;
			gap: 8px;
		}
		h1 {
			font-size: 18px;
			white-space: nowrap;
		}
		.wide {
			display: none;
		}
		.phone {
			display: revert;
		}
		.next.phone {
			flex-basis: 100%;
			font-size: 13px;
		}
		.btn {
			padding: 5px 10px;
		}
	}
</style>
