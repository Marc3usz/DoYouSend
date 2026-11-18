#!/usr/bin/env bash
# Szuka w sledzonych plikach sekretow i danych kontaktowych spoza puli testowej
# (CLAUDE.md, zasady 1 i 2). Uruchamiane w CI; lokalnie: bash scripts/check-secrets.sh
set -uo pipefail
cd "$(dirname "$0")/.."

# Pliki generowane: sumy kontrolne i URL-e paczek, nie dane projektu.
exclude=(':!frontend/package-lock.json' ':!backend/go.sum')
failed=0

report() {
  echo "::error::$1"
  failed=1
}

if git ls-files --error-unmatch .env >/dev/null 2>&1; then
  report ".env jest w repozytorium - usun go i uniewaznij klucze"
fi

# 1. Przypisanie dlugiej wartosci do klucza/hasla/tokenu.
assign='(api[_-]?key|secret|password|token)\s*[:=]\s*["'"'"']?[A-Za-z0-9_\-]{16,}'
if git grep -nIE "$assign" -- . ':!.env.example' "${exclude[@]}"; then
  report "Mozliwy sekret przypisany do zmiennej - przenies go do .env albo do panelu"
fi

# 2. Ksztalty kluczy operatorow: SendGrid (SG.<id>.<sekret>), Notion (ntn_..., secret_...),
# klucze prywatne. Atrapy w testach ("SG.test-key", "ntn_test") sa za krotkie, zeby pasowac.
shapes='\bSG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}|\bntn_[A-Za-z0-9]{30,}|\bsecret_[A-Za-z0-9]{30,}|-----BEGIN [A-Z ]*PRIVATE KEY-----'
if git grep -nIE "$shapes" -- . "${exclude[@]}"; then
  report "Klucz w formacie operatora (SendGrid/Notion) albo klucz prywatny"
fi

# 3. Adresy e-mail spoza domen testowych (RFC 2606/6761). git@github.com to adres SSH repozytorium.
emails="$(git grep -nIoE '[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}' -- . "${exclude[@]}" |
  grep -viE '@(([a-z0-9-]+\.)*example\.(test|com|org|net)|localhost)$' |
  grep -vE ':git@github\.com$')"
if [ -n "$emails" ]; then
  echo "$emails"
  report "Adres e-mail spoza domen testowych - uzyj @example.test"
fi

# 4. Polskie numery komorkowe spoza puli testowej +48 500 100 1NN.
phones="$(git grep -nIoE '\+48[ -]?[0-9]{3}[ -]?[0-9]{3}[ -]?[0-9]{3}' -- . "${exclude[@]}" |
  grep -vE ':\+48[ -]?500[ -]?100[ -]?1[0-9]{2}$')"
if [ -n "$phones" ]; then
  echo "$phones"
  report "Numer telefonu spoza puli testowej - uzyj +48 500 100 1NN"
fi

if [ "$failed" -eq 0 ]; then
  echo "Brak sekretow i danych kontaktowych spoza puli testowej."
fi
exit "$failed"
