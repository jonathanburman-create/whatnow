<script lang="ts">
	import { flushSync } from 'svelte';
	import { page } from '$app/state';
	import { editor } from '#lib/editor.svelte.ts';
	import Capture from './Capture.svelte';

	// Phones only: a round + above the tab bar opens a sheet with the capture box, so the top of
	// the page can start with the plan instead of an input.
	let open = $state(false);
	let input = $state<HTMLInputElement>();

	// Mounted and focused in the same tap, so the phone's keyboard comes up straight away.
	function show() {
		open = true;
		flushSync();
		input?.focus();
	}
	function detailed() {
		open = false;
		editor.openNew();
	}
</script>

{#if open}
	<button class="backdrop" aria-label="Close" onclick={() => (open = false)}></button>
	<div class="sticker sheet" role="dialog" aria-label="Add">
		<Capture bind:input oncaptured={() => (open = false)} />
		<div class="row">
			<button class="btn" onclick={detailed}>Detailed task…</button>
			<button class="link" onclick={() => (open = false)}>Cancel</button>
		</div>
	</div>
{:else if !page.url.pathname.startsWith('/ideas')}
	<!-- Ideas has its own box at the top. -->
	<button class="fab" aria-label="Add a task or note" onclick={show}>+</button>
{/if}

<style>
	.fab,
	.backdrop,
	.sheet {
		display: none;
	}

	@media (max-width: 800px) {
		.fab {
			display: block;
			position: fixed;
			right: 16px;
			bottom: calc(76px + env(safe-area-inset-bottom));
			z-index: 14;
			width: 56px;
			height: 56px;
			border-radius: 50%;
			border: var(--line);
			box-shadow: var(--shadow);
			background: var(--primary);
			color: var(--on-primary);
			font-size: 30px;
			font-weight: var(--heavy);
			line-height: 1;
		}
		.backdrop {
			display: block;
			position: fixed;
			inset: 0;
			z-index: 16;
			border: none;
			background: rgb(0 0 0 / 0.25);
		}
		.sheet {
			display: flex;
			flex-direction: column;
			gap: 10px;
			position: fixed;
			left: 8px;
			right: 8px;
			bottom: calc(70px + env(safe-area-inset-bottom));
			z-index: 17;
			padding: 12px;
		}
	}
	.row {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.link {
		background: none;
		border: none;
		color: var(--ink-soft);
		text-decoration: underline;
	}
</style>
