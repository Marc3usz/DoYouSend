export type Theme = 'system' | 'light' | 'dark';

export const themeStorageKey = 'theme';

export const themeLabels: Record<Theme, string> = {
	system: 'Motyw: systemowy',
	light: 'Motyw: jasny',
	dark: 'Motyw: ciemny'
};

const order: Theme[] = ['system', 'light', 'dark'];

export function parseTheme(value: string | null): Theme {
	return value === 'light' || value === 'dark' ? value : 'system';
}

export function nextTheme(current: Theme): Theme {
	return order[(order.indexOf(current) + 1) % order.length] ?? 'system';
}
