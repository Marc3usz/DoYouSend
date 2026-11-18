import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
export default {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter(),
		// One .env for the whole project, in the repo root (see .env.example): the dev
		// server reads PUBLIC_API_URL from there. A built app takes it from the process
		// environment instead.
		env: { dir: '..' }
	}
};
