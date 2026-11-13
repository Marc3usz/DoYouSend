// Polish texts about e-mail sender addresses (composer, account page, admin).
import type { SenderKind, SenderOption } from '$lib/api/senders';

export const senderKindLabels: Record<SenderKind, string> = {
	shared: 'wspólny adres szkoły',
	own: 'Twój adres',
	delegated: 'adres udostępniony przez administratora'
};

/** Why an address cannot be used yet; null when it can. */
export function senderReasonText(reason: string | null | undefined): string | null {
	switch (reason) {
		case null:
		case undefined:
			return null;
		case 'domain_not_authenticated':
			return 'domena nie jest uwierzytelniona';
		case 'not_verified':
			return 'adres nie jest zweryfikowany';
		case 'shared_only':
			return 'serwer SMTP wysyła tylko ze wspólnego adresu';
		default:
			return 'adres jest niedostępny';
	}
}

/** "Name <address>" or the bare address. */
export function addressText(s: { email: string; name: string }): string {
	return s.name ? `${s.name} <${s.email}>` : s.email;
}

/** One option of the sender picker, with the reason when it is disabled. */
export function senderOptionText(o: SenderOption): string {
	const base = `${addressText(o)} — ${senderKindLabels[o.kind]}`;
	const reason = o.usable ? null : senderReasonText(o.reason);
	return reason ? `${base} (${reason})` : base;
}

/**
 * The option to preselect: the default when it can be used, otherwise the
 * first usable one, otherwise nothing (the backend then picks its default).
 */
export function initialSenderId(
	options: { defaultId: string; items: SenderOption[] } | null
): string | undefined {
	if (!options) return undefined;
	const usable = options.items.filter((o) => o.usable);
	if (usable.some((o) => o.id === options.defaultId)) return options.defaultId;
	return usable[0]?.id;
}
