// Globalne typy SvelteKit.
import type { User } from '$lib/api/auth';

declare global {
	namespace App {
		interface Locals {
			/** Zalogowany uzytkownik; null przed zalogowaniem (hooks.server.ts). */
			user: User | null;
		}
		// interface PageData {}
		// interface Error {}
	}
}

export {};
