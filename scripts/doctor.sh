#!/usr/bin/env bash
# Sprawdza, czy srodowisko deweloperskie ma wszystko, czego wymaga README.
set -u
ok=0
check() {
  if command -v "$1" >/dev/null 2>&1; then
    printf '  [ok]   %-8s %s\n' "$1" "$($2 2>&1 | head -1)"
  else
    printf '  [BRAK] %-8s %s\n' "$1" "$3"; ok=1
  fi
}
echo "Wymagane narzedzia:"
check go      "go version"        "zainstaluj Go 1.24+"
check node    "node --version"    "zainstaluj Node 22+"
check npm     "npm --version"     "instalowany razem z Node"
check docker  "docker --version"  "zainstaluj Docker Desktop"
check git     "git --version"     "zainstaluj Git"
[ -f .env ] && echo "  [ok]   .env     obecny" || { echo "  [BRAK] .env     skopiuj: cp .env.example .env"; ok=1; }
exit $ok
