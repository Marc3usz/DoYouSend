// Polish number, money and date formatting shared by every domain's pages.
// Nothing here counts anything: the numbers come from the backend.

/** Picks the Polish plural form: 1 SMS, 2 SMS-y, 5 SMS-ów, 22 SMS-y. */
export function plural(n: number, one: string, few: string, many: string): string {
	const abs = Math.abs(n);
	if (abs === 1) return one;
	const lastDigit = abs % 10;
	const lastTwo = abs % 100;
	return lastDigit >= 2 && lastDigit <= 4 && (lastTwo < 12 || lastTwo > 14) ? few : many;
}

export const smsText = (n: number) => `${n} ${plural(n, 'SMS', 'SMS-y', 'SMS-ów')}`;

export const recipientsText = (n: number) =>
	`${n} ${plural(n, 'odbiorca', 'odbiorcy', 'odbiorców')}`;

/** "1 osoba", "2 osoby", "5 osób". */
export const peopleText = (n: number) => `${n} ${plural(n, 'osoba', 'osoby', 'osób')}`;

const MILLI_PER_ZLOTY = 1000;

const pln = new Intl.NumberFormat('pl-PL', {
	style: 'currency',
	currency: 'PLN',
	minimumFractionDigits: 2,
	maximumFractionDigits: 3,
	useGrouping: true
});

/** Formats a cost given in thousandths of a złoty (80 -> "0,08 zł"). */
export function costText(costMilli: number): string {
	return pln.format(costMilli / MILLI_PER_ZLOTY);
}

const dateTime = new Intl.DateTimeFormat('pl-PL', {
	dateStyle: 'medium',
	timeStyle: 'short',
	timeZone: 'Europe/Warsaw'
});

/** A date and time in Warsaw, or a dash when there is none. */
export function dateTimeText(iso: string | null | undefined): string {
	return iso ? dateTime.format(new Date(iso)) : '—';
}
