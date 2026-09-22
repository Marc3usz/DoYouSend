**System komunikacji e-mail i SMS** 

## **Cel projektu**

Celem projektu jest stworzenie systemu, który pozwoli dyrektorowi lub innemu upoważnionemu pracownikowi szkoły wysłać jeden komunikat jednocześnie przez e-mail i SMS.

Wysyłający przygotowuje jedną treść, wybiera konkretnych odbiorców albo całą grupę, a system rozpoczyna wysyłkę do wszystkich wskazanych osób oboma kanałami.

Przykład: Zebranie z rodzicami odbędzie się 15 października o 17:00 w sali 12\.

Rodzic otrzymuje dokładnie tę samą treść na adres e-mail oraz numer telefonu.

## **Użytkownicy systemu**

* **Dyrektor lub upoważniony pracownik** – tworzy wiadomości, wybiera odbiorców i uruchamia wysyłkę.  
* **Administrator** – zarządza użytkownikami, odbiorcami, grupami oraz konfiguracją wysyłki.  
* **Odbiorca** – rodzic albo uczeń otrzymujący komunikaty.

## **Baza odbiorców**

System powinien przechowywać podstawowe dane potrzebne do komunikacji:

* imię i nazwisko,  
* adres e-mail,  
* numer telefonu,  
* typ odbiorcy, np. rodzic lub uczeń,  
* grupy, do których należy dana osoba.

Administrator powinien móc dodawać i edytować odbiorców ręcznie oraz importować większą liczbę osób z pliku.

System musi wskazywać błędne, niepełne albo powtarzające się dane.

## **Grupy odbiorców**

System powinien obsługiwać masową wysyłkę do grup, między innymi:

* wszyscy rodzice,  
* wszyscy uczniowie,  
* rodzice uczniów wybranej klasy,  
* uczniowie wybranej klasy,  
* własne grupy utworzone przez administratora.

Jedna osoba może należeć do kilku grup. Jeżeli podczas jednej wysyłki zostanie wybrana przez kilka grup, powinna otrzymać wiadomość tylko raz.

Wysyłający powinien móc również wskazać jedną osobę albo ręcznie wybrać kilku odbiorców.

## **Tworzenie wiadomości**

Podczas przygotowania komunikatu użytkownik:

* wybiera odbiorców lub grupy,  
* wpisuje temat e-maila,  
* wpisuje jedną wspólną treść wiadomości,  
* sprawdza podgląd wiadomości,  
* zatwierdza wysyłkę.

Właściwa treść e-maila i SMS-a musi być identyczna. System nie może automatycznie skracać, streszczać ani zmieniać tekstu.

Temat e-maila jest osobnym polem, dlatego wszystkie ważne informacje muszą znaleźć się również w głównej treści wiadomości.

Jeżeli wiadomość jest personalizowana, np. zawiera imię odbiorcy, dana osoba nadal otrzymuje identyczną spersonalizowaną treść na e-mail i SMS.

## **Długość wiadomości SMS**

Długa wiadomość może zostać podzielona na kilka części SMS, ale jej treść musi pozostać bez zmian.

Przed zatwierdzeniem wysyłki system powinien pokazać:

* długość wiadomości,  
* liczbę części SMS przypadających na jednego odbiorcę,  
* liczbę odbiorców,  
* łączną liczbę wysyłanych SMS-ów,  
* przewidywany koszt wysyłki.

Jeżeli trzeba przekazać dokument albo inny plik, w identycznej treści e-maila i SMS-a należy umieścić prowadzący do niego link. Sam załącznik e-mailowy nie zapewnia identycznej informacji w obu kanałach.

## **Wysyłka wiadomości**

Po zatwierdzeniu system rozpoczyna wysyłkę wiadomości e-mail i SMS do wszystkich wybranych odbiorców.

System powinien:

* wysyłać wiadomości indywidualnie, bez ujawniania odbiorcom danych innych osób,  
* usuwać duplikaty wynikające z przynależności do kilku grup,  
* sprawdzać poprawność adresów e-mail i numerów telefonu,  
* pokazać przed wysłaniem osoby, które nie mają kompletu danych,  
* nie zatrzymywać całej masowej wysyłki z powodu błędu dotyczącego pojedynczego odbiorcy.

Uruchomienie obu kanałów następuje w tym samym momencie, ale dokładny czas dostarczenia może zależeć od operatora e-mail i SMS.

## **Brak danych kontaktowych**

Jeżeli odbiorca nie ma adresu e-mail albo numeru telefonu, system musi wyraźnie wskazać ten problem przed wysłaniem.

Wysyłający powinien móc:

* poprawić dane,  
* wykluczyć odbiorcę z danej wysyłki,  
* wysłać wiadomość tylko dostępnym kanałem.

Taka wysyłka powinna zostać oznaczona jako częściowa, ponieważ odbiorca nie otrzymał wiadomości zarówno e-mailem, jak i SMS-em.

## **Statusy wysyłki**

System powinien przechowywać oddzielny status każdego kanału dla każdego odbiorcy.

Przykładowe statusy:

* oczekuje na wysłanie,  
* wysyłanie,  
* wysłano,  
* dostarczono, jeżeli operator udostępnia taką informację,  
* nie udało się wysłać.

Dla całej masowej wysyłki można zastosować statusy:

* wersja robocza,  
* zaplanowana,  
* w trakcie,  
* zakończona,  
* zakończona z błędami,  
* anulowana.

Użytkownik powinien móc zobaczyć, do kogo wiadomość została wysłana prawidłowo, a u kogo wystąpił błąd. Nieudane wiadomości powinny nadawać się do ponownej wysyłki.

## **Historia komunikacji**

System powinien zapisywać:

* treść i temat wiadomości,  
* autora wiadomości,  
* datę i godzinę wysłania,  
* wybrane grupy,  
* ostateczną listę odbiorców,  
* status e-maila i SMS-a dla każdej osoby,  
* informacje o błędach i ponownej wysyłce.

Wysłanej wiadomości nie należy później edytować. Można utworzyć jej kopię i wykorzystać ją jako podstawę nowego komunikatu.

## **Panel administracyjny**

Administrator powinien mieć możliwość:

* zarządzania odbiorcami i ich danymi,  
* tworzenia i edytowania grup,  
* importowania odbiorców,  
* zarządzania użytkownikami i ich uprawnieniami,  
* konfigurowania operatora e-mail i SMS,  
* przeglądania historii wysyłek,  
* sprawdzania liczby oraz kosztu wykorzystanych SMS-ów.

## **Integracje**

System musi zostać połączony z:

* usługą odpowiedzialną za wysyłkę e-maili,  
* bramką SMS umożliwiającą masową wysyłkę i odbieranie statusów.

Zespół sam analizuje dostępne rozwiązania i decyduje, z jakich operatorów oraz gotowych narzędzi skorzystać. Może konsultować wybór i sposób integracji z zatrudnionymi programistami Techni.

Podczas budowy należy korzystać z konfiguracji testowej. Podłączenie prawdziwych danych odbiorców i produkcyjnej wysyłki wymaga przeglądu rozwiązania oraz zgody opiekuna projektu.

## **Bezpieczeństwo**

* Dostęp do systemu mogą mieć wyłącznie upoważnieni użytkownicy.  
* Użytkownik powinien widzieć tylko dane potrzebne do wykonywania swojej pracy.  
* Hasła, klucze API oraz dane dostępowe nie mogą znajdować się w repozytorium.  
* Podczas testów należy korzystać z fikcyjnych danych, adresów i numerów telefonów.  
* System powinien rejestrować, kto przygotował i uruchomił daną wysyłkę.  
* Masowa wysyłka powinna wymagać końcowego potwierdzenia.

## **Decyzje pozostawione zespołowi**

Zespół sam wybiera:

* technologie i architekturę systemu,  
* sposób przechowywania odbiorców oraz grup,  
* operatora e-mail i bramkę SMS,  
* sposób realizacji masowej wysyłki,  
* wygląd panelu i przebieg poszczególnych ekranów,  
* sposób obsługi kolejek, błędów i ponownych prób,  
* organizację pracy i podział odpowiedzialności.

## **Kryteria ukończenia projektu**

Projekt zostanie uznany za gotowy, gdy:

* można dodać lub zaimportować rodziców i uczniów,  
* można tworzyć grupy odbiorców,  
* można wysłać wiadomość do jednej osoby albo całej grupy,  
* odbiorca otrzymuje identyczną treść e-mailem i SMS-em,  
* długie wiadomości są dzielone na kilka SMS-ów bez zmiany treści,  
* przed wysłaniem widać liczbę odbiorców, SMS-ów i przewidywany koszt,  
* system nie wysyła duplikatów do osób należących do kilku wybranych grup,  
* można sprawdzić status obu kanałów dla każdego odbiorcy,  
* błędy pojedynczych odbiorców nie zatrzymują całej wysyłki,  
* dostępna jest historia komunikacji,  
* projekt można uruchomić na podstawie dołączonej instrukcji,  
* repozytorium nie zawiera prawdziwych danych ani poufnych kluczy.

