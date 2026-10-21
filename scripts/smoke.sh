#!/usr/bin/env bash
set -euo pipefail

BASE=${BASE:-http://localhost:8080}
H='Content-Type: application/json'
acct() { grep -o '"accountId":[0-9]*' | head -1 | cut -d: -f2; }
ref()  { grep -o 'TXN-[0-9-]*' | head -1; }

A=$(curl -sf -X POST "$BASE/api/accounts" -H "$H" -d '{"name":"Amina"}' | acct)
B=$(curl -sf -X POST "$BASE/api/accounts" -H "$H" -d '{"name":"Brian"}' | acct)
echo "accounts: $A and $B"

curl -sf -X POST "$BASE/api/accounts/$A/deposit" -H "$H" \
  -H "Idempotency-Key: $(uuidgen)" -d '{"amount":"5000.00"}' > /dev/null

T=$(curl -sf -X POST "$BASE/api/transfers" -H "$H" -H "Idempotency-Key: $(uuidgen)" \
  -d "{\"sourceAccount\":$A,\"destinationAccount\":$B,\"amount\":\"1500.00\"}" | ref)
echo "transfer: $T"

curl -s "$BASE/api/accounts/$A/balance"; echo
curl -s "$BASE/api/accounts/$B/balance"; echo

R=$(curl -sf -X POST "$BASE/api/transfers/$T/reverse" | ref)
echo "reversal: $R"

curl -s "$BASE/api/accounts/$A/transactions?limit=5"; echo
curl -s "$BASE/api/admin/reconciliation"; echo