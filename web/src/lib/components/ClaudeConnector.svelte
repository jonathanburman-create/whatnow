<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type PushKey } from '#lib/api.ts';
	import { sentLabel } from '#lib/integrations.ts';
	import { toasts } from '#lib/toasts.svelte.ts';

	let key = $state<PushKey | null>(null);
	let url = $state('');
	let captureURL = $state('');
	/** A key just made, shown this once. */
	let newKey = $state('');
	let loaded = $state(false);

	onMount(() => {
		api.connectorKey().then(
			(r) => {
				key = r.key;
				url = r.url;
				captureURL = r.capture_url;
				loaded = true;
			},
			(err) => toasts.error(err)
		);
	});

	async function create() {
		try {
			newKey = (await api.createConnectorKey()).key;
			key = (await api.connectorKey()).key;
		} catch (err) {
			toasts.error(err);
		}
	}

	async function revoke() {
		try {
			await api.revokeConnectorKey();
			key = null;
			newKey = '';
		} catch (err) {
			toasts.error(err);
		}
	}

	function copy(text: string) {
		navigator.clipboard.writeText(text).then(
			() => toasts.show('Copied'),
			(err) => toasts.error(err)
		);
	}
</script>

<section class="sticker card">
	<h2 class="label">Claude connector</h2>
	<p class="muted">
		Lets Claude in the Claude app read your day and file what you dictate: tasks, ideas, and events. In
		Claude, add a custom connector with this address, and send the key as a request header:
		<code>Authorization: Bearer</code> followed by the key.
	</p>
	{#if loaded}
		<div class="row">
			<code class="shown">{url}</code>
			<button type="button" class="btn small" onclick={() => copy(url)}>Copy</button>
		</div>
		<p class="muted hint">For an iPhone Shortcut, send <code>{'{"raw_text": "…"}'}</code> with the same key to:</p>
		<div class="row">
			<code class="shown">{captureURL}</code>
			<button type="button" class="btn small" onclick={() => copy(captureURL)}>Copy</button>
		</div>
		{#if newKey}
			<p class="hint">Your key. Copy it now: it won't be shown again.</p>
			<div class="row">
				<code class="shown">{newKey}</code>
				<button type="button" class="btn small" onclick={() => copy(newKey)}>Copy</button>
			</div>
		{:else if key}
			<p class="muted hint">{key.last_used_at ? `Last used ${sentLabel(key.last_used_at, new Date())}` : 'Not used yet'}</p>
		{/if}
		<div class="actions">
			<button type="button" class="btn" onclick={create}>{key ? 'Replace key' : 'Create key'}</button>
			{#if key}<button type="button" class="link" onclick={revoke}>Revoke</button>{/if}
		</div>
	{/if}
</section>

<style>
	.card {
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	h2 {
		margin: 0;
		font-size: 14px;
	}
	p {
		margin: 0;
		font-size: 14px;
	}
	.row {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.shown {
		flex: 1;
		min-width: 0;
		overflow-wrap: anywhere;
		padding: 6px 8px;
		background: var(--paper-deep);
		border-radius: var(--radius-sm);
	}
	.small {
		font-size: 13px;
		padding: 3px 10px;
	}
	.actions {
		display: flex;
		align-items: center;
		gap: 12px;
	}
	.link {
		background: none;
		border: none;
		color: var(--danger);
		text-decoration: underline;
		padding: 0;
	}
</style>
