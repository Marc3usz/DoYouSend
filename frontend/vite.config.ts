import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
	// The same PUBLIC_API_URL as hooks.server.ts uses. loadEnv reads the root .env
	// (kit.env.dir in svelte.config.js); a variable set in the process environment wins.
	const apiUrl = loadEnv(mode, '..', 'PUBLIC_').PUBLIC_API_URL || 'http://localhost:8080';

	return {
		plugins: [sveltekit()],
		server: {
			port: 5173,
			// Wywolania /api/* ida do backendu w Go, dzieki czemu w dev nie ma CORS-u.
			proxy: { '/api': { target: apiUrl.replace(/\/+$/, ''), changeOrigin: true } }
		}
	};
});
