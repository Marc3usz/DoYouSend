# ADR-0003: Operator e-mail i bramka SMS

- **Status:** propozycja, do domknięcia przed M3
- **Data:** 2026-09-22
- **Uczestnicy:** DEV C prowadzi, decyzja zespołowa

## Kontekst

`description.md` zostawia wybór operatorów zespołowi i dopuszcza konsultację z programistami
Techni. Wybór wpływa na koszty, raporty doręczeń i formalności (nazwa nadawcy SMS w Polsce
wymaga rejestracji u operatora).

## Decyzja

**Do podjęcia.** Do czasu decyzji pracujemy na implementacjach lokalnych: Mailpit (SMTP)
i provider `fake` dla SMS-ów, obie za wspólnym interfejsem `providers`.

Kryteria wyboru:

1. masowa wysyłka i raporty doręczeń (webhook lub odpytywanie),
2. konto testowe/sandbox bez kosztów,
3. obsługa polskich znaków i poprawne liczenie części przy UCS-2,
4. koszt za SMS i wymagania przy rejestracji nazwy nadawcy,
5. sensowne SDK albo zwykłe REST API.

## Konsekwencje

- Wybór operatora nie może wyciekać poza `internal/providers`; reszta kodu zna tylko interfejs.
- Klucze trafiają wyłącznie do `.env`; przejście na wysyłkę produkcyjną wymaga zgody
  opiekuna projektu.
