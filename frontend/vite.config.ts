import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		port: 5173,
		// Wywolania /api/* ida do backendu w Go, dzieki czemu w dev nie ma CORS-u.
		proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: true } }
	}
});
