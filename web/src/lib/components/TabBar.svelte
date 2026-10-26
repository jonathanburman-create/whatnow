<script lang="ts">
	import { page } from '$app/state';
	import { chat } from '#lib/chat.svelte.ts';

	// Phones only: the pages a thumb reaches for, plus the chat, along the bottom of the screen.
	const tabs = [
		{ href: '/', label: 'Today', icon: '☀' },
		{ href: '/tasks', label: 'Tasks', icon: '☰' },
		{ href: '/ideas', label: 'Ideas', icon: '✦' }
	];
	const current = $derived(page.url.pathname);
</script>

<nav class="tabbar" aria-label="Pages">
	{#each tabs as tab (tab.href)}
		<a
			href={tab.href}
			class="tab"
			class:on={current === tab.href && !chat.open}
			aria-current={current === tab.href ? 'page' : undefined}
			onclick={() => chat.open && chat.toggleRemembered()}
		>
			<span class="icon" aria-hidden="true">{tab.icon}</span>
			{tab.label}
		</a>
	{/each}
	<button class="tab" class:on={chat.open} aria-pressed={chat.open} onclick={() => chat.toggleRemembered()}>
		<span class="icon" aria-hidden="true">💬</span>
		Chat
		{#if (chat.unread || chat.busy) && !chat.open}<span class="dot" aria-label="New reply"></span>{/if}
	</button>
</nav>

<style>
	.tabbar {
		display: none;
	}

	@media (max-width: 800px) {
		.tabbar {
			position: fixed;
			left: 0;
			right: 0;
			bottom: 0;
			z-index: 15;
			display: grid;
			grid-template-columns: repeat(4, 1fr);
			background: var(--card);
			border-top: var(--line);
			padding: 4px 4px calc(4px + env(safe-area-inset-bottom));
		}
	}
	.tab {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1px;
		padding: 5px 0 4px;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--ink-soft);
		font-size: 12px;
		font-weight: var(--heavy);
		text-decoration: none;
	}
	.tab.on {
		color: var(--ink);
		background: var(--chip);
	}
	.icon {
		font-size: 18px;
		line-height: 1.2;
	}
	.dot {
		position: absolute;
		top: 4px;
		right: calc(50% - 22px);
		width: 9px;
		height: 9px;
		border-radius: 50%;
		background: var(--now);
		border: 2px solid var(--ink);
	}
</style>
