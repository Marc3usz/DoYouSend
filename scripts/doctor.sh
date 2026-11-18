#!/usr/bin/env bash
# Sprawdza, czy srodowisko deweloperskie ma wszystko, czego wymaga README:
# narzedzia w wymaganych wersjach, dzialajacy Docker i plik .env.
set -u
cd "$(dirname "$0")/.."

GO_MIN_MAJOR=1
GO_MIN_MINOR=24
NODE_MIN_MAJOR=22

failed=0
ok()   { printf '  [ok]   %-15s %s\n' "$1" "$2"; }
warn() { printf '  [uwaga] %-14s %s\n' "$1" "$2"; }
bad()  { printf '  [BRAK] %-15s %s\n' "$1" "$2"; failed=1; }

have() { command -v "$1" >/dev/null 2>&1; }

# "go1.23.3" albo "v22.12.0" -> "1 23" / "22 12"
major_minor() { printf '%s' "$1" | sed -E 's/^[^0-9]*([0-9]+)\.([0-9]+).*/\1 \2/'; }

echo "Wymagane narzedzia:"

if have go; then
  # GOTOOLCHAIN=local pokazuje wersje zainstalowana, bez pobierania nowszej.
  installed="$(GOTOOLCHAIN=local go env GOVERSION 2>/dev/null)"
  read -r major minor <<<"$(major_minor "$installed")"
  if [ -z "${minor:-}" ]; then
    bad go "nie udalo sie odczytac wersji (go env GOVERSION)"
  elif [ "$major" -gt "$GO_MIN_MAJOR" ] || { [ "$major" -eq "$GO_MIN_MAJOR" ] && [ "$minor" -ge "$GO_MIN_MINOR" ]; }; then
    ok go "$installed"
  else
    toolchain="$(go env GOTOOLCHAIN 2>/dev/null)"
    case "$toolchain" in
      local*)
        bad go "$installed, wymagane ${GO_MIN_MAJOR}.${GO_MIN_MINOR}+ (GOTOOLCHAIN=local blokuje pobranie nowszej wersji)" ;;
      *)
        warn go "$installed; przy pierwszym uzyciu Go pobierze toolchain ${GO_MIN_MAJOR}.${GO_MIN_MINOR} (wymaga sieci)" ;;
    esac
  fi
else
  bad go "zainstaluj Go ${GO_MIN_MAJOR}.${GO_MIN_MINOR}+"
fi

if have node; then
  installed="$(node --version 2>/dev/null)"
  read -r major _ <<<"$(major_minor "$installed")"
  if [ -n "${major:-}" ] && [ "$major" -ge "$NODE_MIN_MAJOR" ]; then
    ok node "$installed"
  else
    bad node "$installed, wymagane ${NODE_MIN_MAJOR}+"
  fi
else
  bad node "zainstaluj Node ${NODE_MIN_MAJOR}+"
fi

if have npm; then ok npm "$(npm --version 2>/dev/null)"; else bad npm "instalowany razem z Node"; fi
if have git; then ok git "$(git --version 2>/dev/null)"; else bad git "zainstaluj Git"; fi

if have make; then
  ok make "$(make --version 2>/dev/null | head -n 1)"
else
  bad make "zainstaluj make (Windows: choco install make / scoop install make / winget install ezwinports.make)"
fi

if have docker; then
  ok docker "$(docker --version 2>/dev/null)"
  if compose="$(docker compose version 2>/dev/null)"; then
    ok "docker compose" "$compose"
  else
    bad "docker compose" "brak wtyczki Compose v2 (jest w Docker Desktop)"
  fi
  if docker info >/dev/null 2>&1; then
    ok "docker daemon" "dziala"
  else
    bad "docker daemon" "nie odpowiada - uruchom Docker Desktop i poczekaj, az wystartuje"
  fi
else
  bad docker "zainstaluj Docker Desktop"
fi

echo "Konfiguracja:"
if [ -f .env ]; then
  ok .env "obecny"
else
  bad .env "skopiuj: cp .env.example .env"
fi

if [ "$failed" -ne 0 ]; then
  echo "Popraw pozycje oznaczone [BRAK] i uruchom make doctor ponownie."
fi
exit "$failed"
