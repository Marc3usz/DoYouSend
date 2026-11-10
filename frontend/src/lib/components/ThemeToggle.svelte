<script lang="ts">
	import { onMount } from 'svelte';
	import { nextTheme, parseTheme, themeLabels, themeStorageKey, type Theme } from '$lib/theme';

	const icons: Record<Theme, string> = { system: '◐', light: '☀', dark: '☾' };

	let theme = $state<Theme>('system');

	onMount(() => {
		try {
			theme = parseTheme(localStorage.getItem(themeStorageKey));
		} catch {
			// Storage blocked: stay on the system theme.
		}
	});

	function cycle() {
		theme = nextTheme(theme);
		const root = document.documentElement;
		if (theme === 'system') {
			delete root.dataset.theme;
		} else {
			root.dataset.theme = theme;
		}
		try {
			if (theme === 'system') {
				localStorage.removeItem(themeStorageKey);
			} else {
				localStorage.setItem(themeStorageKey, theme);
			}
		} catch {
			// The choice still applies for this page view.
		}
	}
</script>

<button type="button" class="theme-toggle" onclick={cycle} title={themeLabels[theme]}>
	<span aria-hidden="true">{icons[theme]}</span>
	<span class="visually-hidden">{themeLabels[theme]}. Kliknij, aby zmienić.</span>
</button>

<style>
	.theme-toggle {
		min-height: 2rem;
		min-width: 2rem;
		padding: 0.2rem 0.5rem;
		line-height: 1;
	}
	.visually-hidden {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
</style>
