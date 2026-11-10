# ADR-0018: Ustawienia wysyłki edytowane w panelu administratora

- **Status:** propozycja
- **Data:** 2026-11-03
- **Uczestnicy:** Marc3usz (domena D) proponuje na prośbę właściciela produktu;
  MichalK252 (domeny B i C) decyduje o zmianach w `providers/setup` i `delivery`
- **Zastępuje:** decyzję 1 z ADR-0015 („konfiguracja operatorów wyłącznie przez `.env`”).
  Decyzje 2 i 3 z ADR-0015 zostają bez zmian.

## Kontekst

- ADR-0015 zostawił konfigurację operatorów w `.env`, z restartem serwera.
- Właściciel produktu: „nikt tego nie będzie tak konfigurował”. Szkoła nie ma osoby, która
  edytuje pliki na serwerze. Administrator ma tylko panel.
- Ryzyka z ADR-0015 są nadal aktualne: przypadkowe włączenie płatnej wysyłki, wyciek kluczy
  z bazy, przełączenie operatora w trakcie wysyłki. Ta decyzja odpowiada na każde z nich.
- Ustalenia z właścicielem (2026-11-03): klucze API też w panelu, szyfrowane. DRY_RUN i
  wybór operatora też w panelu, ale z potwierdzeniem i z blokadą `DRY_RUN_LOCK` w `.env`.

## Decyzja

1. **Kolejność źródeł:** wartość z panelu (tabela `app_settings`, migracja 0010), potem zmienna
   środowiskowa o tej samej nazwie co dotąd, potem wartość domyślna. Dotychczasowe `.env`
   działają bez zmian, a panel pokazuje źródło każdej wartości. „Usuń z panelu” (`null`)
   przywraca `.env`.
2. **Zakres:** 21 ustawień w grupach ogólne / e-mail / SMS. Są to DRY_RUN, operator e-mail,
   nadawca, SMTP, klucz SendGrid, sandbox, region, klucz publiczny webhooka, adres szkoły dla
   weryfikacji nadawców, operator SMS, klucz SMSAPI, nazwa nadawcy SMS, tryb testowy, cena
   części SMS i token raportów doręczeń. Infrastruktura zostaje w `.env`: `DATABASE_URL`,
   porty, `SESSION_COOKIE_SECURE`, `APP_ENV`, klucz szyfrujący i `DRY_RUN_LOCK`.
3. **Sekrety szyfrowane AES-256-GCM.** Nazwa ustawienia jest daną uwierzytelnianą, więc
   szyfrogramu nie da się przenieść do innego pola. Klucz pochodzi z `SETTINGS_ENCRYPTION_KEY`
   albo z pliku `SETTINGS_KEY_FILE` (domyślnie `settings.key`, uprawnienia 0600), który API
   tworzy przy pierwszym starcie. Zrzut bazy bez klucza nie ujawnia sekretów. API nigdy nie
   zwraca sekretu, tylko `set: true/false`. Gdy klucz się zmieni, sekret jest oznaczany jako
   `unreadable` i trzeba go wpisać ponownie.
4. **Zmiana bez restartu.** `setup.Live` trzyma operatorów zbudowanych z bieżącej konfiguracji.
   Dispatcher, profile kont, weryfikacja nadawców i webhooki pytają `Live` przy każdym
   wywołaniu. Zapis przebiega w kolejności: walidacja pól, potem zbudowanie operatorów
   (`Prepare`; błąd = nic nie zapisane), zapis w bazie, na końcu podmiana (`Activate`).
   Nieudany zapis zostawia poprzednich operatorów.
5. **Bezpieczniki prawdziwej wysyłki:**
   - zmiana, po której wiadomości wychodzą na zewnątrz (DRY_RUN wyłączony i `sendgrid` albo
     `smsapi`), a wcześniej nie wychodziły, wymaga `confirmLiveSending: true`. Panel prosi
     o wpisanie potwierdzenia.
   - `DRY_RUN_LOCK=true` w `.env` wymusza DRY_RUN niezależnie od panelu (środowiska demo i
     testowe) i blokuje jego wyłączenie (409 `dry_run_locked`).
   - gdy serwer właśnie wysyła wsad (wysyłka albo ponowienie), wyłączenie DRY_RUN i zmiana
     operatora e-mail lub SMS przy wyłączonym DRY_RUN są odrzucane (409 `batches_running`). Włączenie DRY_RUN działa
     zawsze, bo zatrzymuje prawdziwą wysyłkę. Licznik trwających wysyłek jest w pamięci
     procesu (`delivery.Service`), a nie w statusie `running` w bazie: wsad, który został
     `running` po awarii procesu, nie może zablokować ustawień na stałe.
   - każda zmiana trafia do dziennika (`settings_changed`): lista kluczy, a przy zmianie
     DRY_RUN i operatorów także nowa wartość. Nigdy wartości sekretów.
   - wartość z `.env`, której panel by nie przyjął (np. `DRY_RUN=tak`), daje wartość domyślną
     i ostrzeżenie w logu. Dla DRY_RUN wartość domyślna to „włączony”.
   - gdy zapisana konfiguracja nie startuje (np. nieczytelny klucz), API startuje z samego
     `.env` z wymuszonym DRY_RUN i zapisuje przyczynę w logu.
6. **Zgoda opiekuna (CLAUDE.md, zasada 3)** obowiązuje dalej. Zmienia się tylko miejsce, w
   którym administrator to wykonuje.

## Rozważane alternatywy

- **Zostać przy `.env` (ADR-0015):** odrzucone przez właściciela produktu, bo nie ma kto tego
  obsługiwać.
- **Sekrety tylko w `.env`, reszta w panelu:** bezpieczniejsze, ale podłączenie operatora
  i tak wymagałoby dostępu do serwera.
- **Restart procesu po zapisie zamiast podmiany w locie:** wymaga nadzorcy procesu
  (systemd/Docker restart policy) i przerywa trwające wsady. Podmiana jest prostsza.

## Konsekwencje

- Plik `settings.key` (albo `SETTINGS_ENCRYPTION_KEY`) trzeba objąć kopią zapasową, osobno od
  bazy. Utrata klucza oznacza ponowne wpisanie kluczy API, nie utratę danych.
- Wyłączenie DRY_RUN albo zmiana operatora czeka, aż skończą się trwające wysyłki, więc
  reszta testowego wsadu nie wyjdzie naprawdę. Sprawdzenie i zapis nie są jedną transakcją:
  wsad rozpoczęty w tej samej chwili co zapis może jeszcze trafić na nowych operatorów.
  To okno trwa milisekundy, a panel i tak wymaga potwierdzenia prawdziwej wysyłki.
- Licznik działa w jednym procesie API. Przy kilku instancjach API trzeba by go przenieść
  do bazy (np. z dzierżawą z czasem wygaśnięcia).
- `setup.ConfigFromEnv` nie jest już używane przez API. Zostaje dla narzędzi i testów.
- `GET /api/admin/config` zostaje jako widok tylko do odczytu, liczony z bieżących ustawień.
