<script lang="ts">
	import AddSheet from '#lib/components/AddSheet.svelte';
	import ChatDock from '#lib/components/ChatDock.svelte';
	import InstallBanner from '#lib/components/InstallBanner.svelte';
	import EditSheet from '#lib/components/EditSheet.svelte';
	import TabBar from '#lib/components/TabBar.svelte';
	import { today } from '#lib/today.svelte.ts';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

<div class="page">
	<main>
		<InstallBanner />
		{@render children()}
	</main>
	<ChatDock opening={today.today?.plan?.opening ?? ''} />
</div>
<AddSheet />
<TabBar />
<EditSheet />

<style>
	.page {
		display: flex;
		gap: 16px;
		padding: 16px;
		align-items: flex-start;
	}
	main {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}

	/* Room at the bottom for the tab bar and the + button, so the last card can scroll clear of them. */
	@media (max-width: 800px) {
		.page {
			padding: 10px 10px calc(140px + env(safe-area-inset-bottom));
		}
		main {
			gap: 10px;
		}
	}
</style>
