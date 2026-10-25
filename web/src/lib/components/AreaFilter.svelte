<script lang="ts">
	import { goto } from '$app/navigation';
	import { areas, type Area } from '#lib/areas.ts';

	interface Props {
		/** The area picked, or empty for all of them. It lives in the address, as ?area=. */
		area: Area | '';
	}
	let { area }: Props = $props();

	function pick(id: Area | '') {
		goto(id ? `?area=${id}` : '?', { replace: true, reset: false });
	}
</script>

<div class="chips areas" role="tablist" aria-label="Filter by area">
	<button class="chip" class:on={area === ''} role="tab" aria-selected={area === ''} onclick={() => pick('')}>
		All areas
	</button>
	{#each areas as a (a.id)}
		<button
			class="chip area"
			class:on={area === a.id}
			role="tab"
			aria-selected={area === a.id}
			style:--area={a.color}
			onclick={() => pick(a.id)}
		>
			{a.label}
		</button>
	{/each}
</div>

<style>
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
	/* On a narrow screen the areas sit in one row that scrolls sideways, so the list starts higher up. */
	@media (max-width: 800px) {
		.areas {
			flex-wrap: nowrap;
			overflow-x: auto;
			scrollbar-width: none;
			margin-inline: -12px;
			padding: 2px 12px 4px;
		}
		.chip {
			flex: none;
		}
	}
</style>
