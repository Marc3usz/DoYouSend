# ADR-0011: Odporność na wyścigi raportów doręczeń i zapis per dostawa

- **Status:** przyjęta
- **Data:** 2026-10-13
- **Uczestnicy:** MichalK252 (DEV C) proponuje i implementuje, S1D0R-10 (DEV B) współdecyduje, Marc3usz (DEV A) recenzuje
- **Powiązane:** Issue #42, PR #40, PR #41, ADR-0003, ADR-0008

## Kontekst

W trakcie masowej wysyłki wiadomości (dla setek lub tysięcy odbiorców w szkole), wywołania bramek operatorów SMSAPI i SendGrid trwają łącznie od kilkudziesięciu sekund do kilku minut. Raporty doręczeń (DLR z SMSAPI oraz Event Webhook z SendGrid) dla pierwszych wysłanych wiadomości docierają do naszej aplikacji już po kilku sekundach — w trakcie trwania pętli `Dispatch`.

Dotychczasowy mechanizm (`delivery.Service.Send`) utrwalał wynik wysyłki dopiero po zakończeniu całego wsadu za pomocą jednego wywołania `Store.SaveOutcome`. Prowadziło to do trzech krytycznych problemów (opisanych w Issue #42):
1. **Utrata raportów doręczeń w locie:** Podczas wysyłki wiersze w tabeli `deliveries` miały `provider_message_id = NULL`. Gdy konsument `PGDeliveryReportConsumer` otrzymywał callback od operatora i wykonywał zapytanie `UPDATE deliveries WHERE provider_message_id = $1`, zapytanie modyfikowało 0 wierszy (`RowsAffected = 0`). Handler webhooka odpowiadał `200 OK`, a raport bezpowrotnie ginął, zaniżając wskaźniki doręczalności.
2. **Brak informacji o postępie przy przerwaniu wysyłki:** Przy zatrzymaniu procesu (np. limit 10 sekund `docker stop` po `SIGTERM`) wszystkie dostawy pozostawały ze statusem `pending` i bez identyfikatorów operatora. Ponowienie wysyłki wsadu skutkowałoby wysłaniem duplikatów wiadomości (podwójne koszty SMS i skargi odbiorców).
3. **Niespójny status wsadu po spóźnionym błędzie:** Raport o statusie `failed` (np. odbicie e-maila lub brak zasięgu telefonu) przychodzący po zakończeniu wsadu aktualizował wiersz w `deliveries`, lecz status wsadu w `message_batches` pozostawał `done` zamiast zmienić się na `done_with_errors`.

## Decyzja

1. **Buforowanie raportów bez dopasowania (`unmatched_delivery_reports`):**
   - Dodajemy tabelę `unmatched_delivery_reports` przechowującą identyfikator `provider_message_id`, kanał, status oraz treść błędu z unikalnym indeksem `(provider_message_id, channel)` i zachowaniem reguły „tylko w przód” (`Advance`).
   - `PGDeliveryReportConsumer` sprawdza liczbę zmodyfikowanych wierszy (`RowsAffected`). Jeśli żaden wiersz w `deliveries` nie pasuje do identyfikatora (co oznacza, że dostawa jest jeszcze w trakcie wysyłki lub utrwalania), raport jest bezpiecznie odkładany do `unmatched_delivery_reports` i logowany na poziomie informacyjnym.
   - Migracja `0007_unmatched_delivery_reports.sql` wprowadza trigger `trg_apply_unmatched_delivery_report` (`BEFORE INSERT OR UPDATE OF provider_message_id ON deliveries`). Gdy wiersz w `deliveries` zyskuje `provider_message_id`, trigger automatycznie dopasowuje buforowany raport, aktualizuje status dostawy w przód i usuwa wpis z bufora.
   - Dodatkowo `PGDeliveryReportConsumer` posiada klauzulę `resolveRaceQuery` na wypadek wyścigu, gdy dostawa została skomitowana tuż przed wstawieniem do bufora.

2. **Automatyczne przeliczanie statusu wsadu po błędzie doręczenia:**
   - Gdy dowolna dostawa przechodzi w stan `failed` na zakończonym wsadzie (`message_batches.status = 'done'`), trigger `trg_delivery_failure_updates_batch` oraz konsument raportów natychmiast aktualizują status powiązanego wsadu na `done_with_errors`.

3. **Inkrementalny zapis postępu per dostawa:**
   - Rozszerzamy interfejs `delivery.Store` o metodę `SaveDelivery(ctx, batchID, recipientID, delivery)`, zaimplementowaną w `PGStore` oraz `MemStore` z regułą `Advance`.
   - `delivery.Dispatcher` zyskuje metodę `DispatchWithObserver` oraz interfejs obserwatora `DeliveryObserver`.
   - Przed wywołaniem bramki dostawa jest oznaczana jako `sending`, a natychmiast po powrocie z bramki jej wynik (`sent` z `provider_message_id` lub `failed`) jest zapisywany w bazie.
   - W przypadku przerwania procesu w połowie, wysłane dostawy mają status `sent` i `provider_message_id`, a niewysłane pozostają `pending`.

## Rozważane alternatywy

- **Zwracanie `404` / `500` z webhooka DLR w razie braku dopasowania:** Odpadło, ponieważ operatorzy (SMSAPI, SendGrid) ponawiają wywołania według własnego harmonogramu (często dopiero po kilkunastu minutach), a po kilku nieudanych próbach trwale wyłączają webhook (blacklisting endpointu).
- **Trzymanie niepasujących raportów wyłącznie w pamięci (in-memory queue):** Odpadło ze względu na ryzyko utraty raportów przy restarcie kontenera oraz brak synchronizacji pomiędzy wieloma replikami API. Tabela w Postgresie zapewnia trwałość i spójność transakcyjną.
- **Odpytywanie periodyczne (polling) tabeli `unmatched_delivery_reports` z crona:** Odpadło na rzecz natychmiastowego wyzwalacza bazodanowego `BEFORE UPDATE OF provider_message_id`. Dzięki triggerowi dopasowanie następuje w ułamku milisekundy w tej samej transakcji, która zapisuje wynik wysyłki, bez konieczności działania procesów w tle.

## Konsekwencje

- Raporty doręczeń przychodzące w trakcie wysyłki wsadu nie giną — dostawy kończą ze statusem `delivered`.
- W razie awarii i restartu kontenera stan bazy dokładnie odzwierciedla wykonaną pracę (`sent` + `provider_message_id` dla wysłanych, `pending` dla niewysłanych).
- Nowa migracja `0007_unmatched_delivery_reports.sql` dodaje tabelę buforującą oraz triggery.
- Wpisy z tabeli `unmatched_delivery_reports` są usuwane w momencie dopasowania do dostawy, dzięki czemu tabela nie rośnie w nieskończoność.
