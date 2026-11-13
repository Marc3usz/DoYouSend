# ADR-0019: Wysyłka jednym kanałem, ponowienia i dokończenie przerwanych wsadów

- **Status:** przyjęta (2026-11-13)
- **Data:** 2026-11-13
- **Uczestnicy:** Marc3usz (wszystkie domeny po ADR-0020)

## Kontekst

- #85 dodał w kreatorze wybór kanałów: oba, tylko e-mail albo tylko SMS. `description.md`
  zakłada wysyłkę „oboma kanałami”, a wysyłkę, w której odbiorca nie dostał wiadomości oboma
  kanałami, każe oznaczyć jako częściową. Wybór kanału nie był nigdzie zapisany, więc wsad
  „tylko SMS” wyglądał w historii jak zwykły wsad dwukanałowy, bez żadnego oznaczenia.
- Ponowienie (`POST /batches/{id}/retry`) wysyłało jeszcze raz tylko błędy przejściowe. Błąd
  trwały (np. zły numer, który ktoś potem poprawił) i raport operatora o niedoręczeniu nie dały
  się ponowić, a ponowienie szło na stary adres z chwili wysyłki. Ponowienie zerowało też
  licznik prób i błąd, więc historia gubiła informację o ponownej wysyłce, której wymaga
  specyfikacja („informacje o błędach i ponownej wysyłce”).
- Wysyłka działa w goroutine procesu API. Gdy proces zginął w trakcie (drugie Ctrl-C, OOM,
  `docker stop` po czasie), wsad zostawał na zawsze w `running`, a ponowienie go odrzucało.
- SMSAPI: timeout po wysłaniu żądania, nieczytelna odpowiedź i dziwne 2xx były klasyfikowane
  jako przejściowe i automatycznie ponawiane, choć SMS mógł już wyjść.

## Decyzja

1. **Kanały są częścią wsadu.** `message_batches.channels` (migracja 0011) zapisuje wybór
   wysyłającego, a historia go pokazuje („Tylko SMS”). To jest oznaczenie wysyłki częściowej
   na poziomie wsadu, gdy jeden kanał wybrano świadomie. Flaga `is_partial` odbiorcy zostaje
   tylko dla braku danych kontaktowych, czyli dla przypadku, o którym mówi specyfikacja.
   Wsady sprzed migracji dostają kanały wywnioskowane z dostaw.
2. **Ponowienie wysyła wszystko, co na pewno nie dotarło.** Klasy błędów (`delivery.RetryClassOf`):
   - *pewne* (przejściowe, trwałe, raport o niedoręczeniu) są ponawiane zawsze;
   - *niepewne* (`unclassified…`, operator mógł przyjąć wiadomość) tylko na wyraźne życzenie
     (`includeUncertain`, w panelu za dodatkowym potwierdzeniem o możliwym duplikacie);
   - błąd renderowania treści nie jest ponawiany nigdy.
   Ponowienie bierze aktualny adres i numer odbiorcy. `attempts` się sumuje, ostatni błąd zostaje
   do udanej wysyłki, a `retry_count` i `last_retry_at` zapisują każde ponowienie.
3. **Start API kończy przerwane wsady** (`delivery.Service.Recover`). Dostawy w `sending`
   stają się `failed` z niepewnym powodem „interrupted while sending”, bo nie wiadomo, czy
   wyszły. Dostawy w `pending` są wysyłane od razu, a status wsadu jest wyliczany od nowa.
   Zakładamy jedną instancję API.
4. **SMSAPI:** przejściowy jest tylko błąd nawiązania połączenia (DNS, odmowa połączenia).
   Wszystko po wysłaniu żądania jest niesklasyfikowane, więc nie jest ponawiane automatycznie.

## Rozważane alternatywy

- **Usunąć wybór kanału.** Odrzucone: szkoła go używa (SMS-y kosztują, a e-mail wystarcza do
  części komunikatów), a specyfikacja nie zabrania wyboru, tylko wymaga oznaczenia.
- **Oznaczać każdego odbiorcę wsadu jednokanałowego jako częściowego.** Odrzucone: licznik
  „jednym kanałem” w historii przestałby odróżniać brak danych od świadomego wyboru.
- **Kolejka zewnętrzna (Redis) zamiast odzyskiwania przy starcie.** Odrzucone na MVP: stan
  wysyłki i tak jest w Postgresie (ADR-0011), a nowa zależność wymagałaby osobnego ADR.

## Konsekwencje

- Historia pokazuje kanały, nadawcę, liczbę ponowień i ostatni błąd każdej dostawy.
- Ponowienie z `includeUncertain` może dać komuś wiadomość dwa razy. Panel o tym ostrzega.
- Przy kilku instancjach API odzyskiwanie przy starcie trzeba będzie przenieść do osobnego
  procesu albo zabezpieczyć blokadą.
