<script lang="ts">
	import { api } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { work } from '#lib/work.svelte.ts';
	import Working from './Working.svelte';

	interface Props {
		/** Called once a capture is sent, such as to close the phone's add sheet. */
		oncaptured?: () => void;
		/** The input, for a caller that focuses it. */
		input?: HTMLInputElement;
	}
	let { oncaptured, input = $bindable() }: Props = $props();

	let text = $state('');

	// The box clears at once: the raw task is saved before cleanup ever runs, so capture never
	// waits on a model. A failed save puts the text back rather than losing it.
	async function capture(e: SubmitEvent) {
		e.preventDefault();
		const raw = text.trim();
		if (!raw) return;
		text = '';
		oncaptured?.();
		if (!(await toasts.track('Captured', () => api.capture(raw))) && !text) text = raw;
	}
</script>

<form class="capture" onsubmit={capture}>
	<input class="field" bind:this={input} bind:value={text} placeholder="Capture anything… press Enter" aria-label="Capture" />
</form>
{#if work.busy('cleanup')}<Working label="Tidying up what you captured…" />{/if}

<style>
	.capture input {
		box-shadow: var(--shadow-sm);
		padding: 12px 14px;
	}
</style>
