import { describe, expect, it } from 'vitest';
import { ApiError } from '$lib/api/client';
import { importFileError, importFileErrorCodes } from '$lib/api/recipients';
import {
	duplicateText,
	fieldErrorText,
	fileErrorText,
	recipientTypeLabel,
	saveErrorText,
	savedSummary
} from './messages';

describe('fieldErrorText', () => {
	it('uses the stable field key, not the English message', () => {
		expect(fieldErrorText({ field: 'last_name', message: 'last name is required' })).toBe(
			'Brak nazwiska'
		);
	});
});

describe('fileErrorText', () => {
	it('has a Polish text for every contract code', () => {
		for (const code of importFileErrorCodes) {
			const { text, detail } = fileErrorText({ code, message: 'detail' });
			expect(text, code).not.toBe('');
			expect(detail).toBe('detail');
		}
	});

	it('falls back to a generic text when the check itself failed', () => {
		expect(fileErrorText(undefined)).toEqual({
			text: 'Nie udało się sprawdzić pliku. Spróbuj ponownie za chwilę.'
		});
	});
});

describe('duplicateText', () => {
	it('points at the earlier row of the file', () => {
		expect(duplicateText({ row: 4, field: 'email', duplicateOfRow: 2 })).toBe(
			'Ten sam e-mail co w wierszu 2'
		);
	});

	it('mentions a recipient already stored', () => {
		expect(duplicateText({ row: 4, field: 'phone', existingRecipientId: 'id-1' })).toBe(
			'Ten sam numer telefonu ma już odbiorca zapisany w bazie'
		);
	});
});

describe('recipientTypeLabel', () => {
	it('translates both types', () => {
		expect(recipientTypeLabel('parent')).toBe('rodzic');
		expect(recipientTypeLabel('student')).toBe('uczeń');
	});
});

describe('importFileError', () => {
	it('reads a contract error body', () => {
		const err = new ApiError(422, 'x', { code: 'missing_columns', message: 'phone, type' });
		expect(importFileError(err)).toEqual({ code: 'missing_columns', message: 'phone, type' });
	});

	it('ignores unknown codes, other errors and bodies without a code', () => {
		expect(importFileError(new ApiError(422, 'x', { code: 'nope', message: 'x' }))).toBeUndefined();
		expect(importFileError(new ApiError(500, 'x'))).toBeUndefined();
		expect(importFileError(new Error('network'))).toBeUndefined();
	});
});

describe('saveErrorText', () => {
	it('says nothing was stored when the save itself failed', () => {
		expect(saveErrorText(undefined).text).toContain('Nic nie zostało zapisane');
	});

	it('uses the file error texts for a whole-file error', () => {
		expect(saveErrorText({ code: 'empty_file', message: 'x' })).toEqual(
			fileErrorText({ code: 'empty_file', message: 'x' })
		);
	});
});

describe('savedSummary', () => {
	const recipient = {
		firstName: 'Jan',
		lastName: 'Kowalski',
		email: 'jan.kowalski@example.test',
		phone: null,
		type: 'parent' as const
	};

	it('counts stored and skipped rows', () => {
		expect(
			savedSummary({
				valid: [{ row: 2, recipient }],
				invalid: [{ row: 3, errors: [] }],
				duplicates: [{ row: 4, field: 'email', duplicateOfRow: 2 }]
			})
		).toBe('Zapisano odbiorców: 1. Pominięto wierszy: 2 (błędne i duplikaty, lista niżej).');
	});

	it('leaves out the skipped part when nothing was skipped', () => {
		expect(savedSummary({ valid: [{ row: 2, recipient }], invalid: [], duplicates: [] })).toBe(
			'Zapisano odbiorców: 1.'
		);
	});
});
